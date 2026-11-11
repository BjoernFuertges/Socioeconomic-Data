package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// metricsAggRow is used internally to scan the aggregation query.
type metricsAggRow struct {
	Year        int     `db:"year"`
	RegionCount int     `db:"region_count"`
	MinVal      float64 `db:"min_val"`
	MaxVal      float64 `db:"max_val"`
	MeanVal     float64 `db:"mean_val"`
	StddevVal   float64 `db:"stddev_val"`
	MedianVal   float64 `db:"median_val"`
}

// metricsRankRow is used internally to scan the ranking query.
type metricsRankRow struct {
	Year  int     `db:"year"`
	Value float64 `db:"value"`
	Rank  int     `db:"rank"`
	Total int     `db:"total"`
}

// GetMetrics computes statistical metrics for one or more Soziodata datasets.
// @Summary Statistische Metriken für Soziodata-Datensätze
// @Description Berechnet Min, Max, Mittelwert, Median und Standardabweichung je Jahr für einen oder mehrere Datensätze.
// @Description Mehrere 'dataset'-Parameter kombinieren unterschiedliche Datensätze in einer Antwort.
// @Description Mit 'geodata_id_nuts' werden zusätzlich der Regionalwert und das Ranking der Region zurückgegeben.
// @Description Für aufeinanderfolgende Jahre wird die prozentuale Veränderung (YoY) berechnet.
// @Tags Metriken
// @Accept json
// @Produce json
// @Param dataset query []string true "Datensatz-Slug(s), z.B. schuldnerquote (mehrfach angeben für mehrere Datensätze)" collectionFormat(multi) example(schuldnerquote)
// @Param year query int false "Einzelnes Jahr filtern" example(2022)
// @Param from_year query int false "Jahresbereich ab" example(2018)
// @Param to_year query int false "Jahresbereich bis" example(2022)
// @Param nuts_parent_id query string false "Auf NUTS-Präfix einschränken (z.B. DE1 für alle Kreise in Bayern)" example(DE1)
// @Param geodata_id_nuts query string false "NUTS-ID einer Region für Regionalwert und Ranking" example(DEF01)
// @Success 200 {object} models.APIResponse{data=[]models.DatasetMetricsResult}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /metrics [get]
func (h *Handler) GetMetrics(c *gin.Context) {
	datasets := c.QueryArray("dataset")
	if len(datasets) == 0 {
		respondBadRequest(c, "Mindestens ein 'dataset'-Parameter ist erforderlich")
		return
	}
	if len(datasets) > 10 {
		respondBadRequest(c, "Maximal 10 Datensätze pro Anfrage erlaubt")
		return
	}

	var filter models.MetricsFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	results := make([]models.DatasetMetricsResult, 0, len(datasets))

	for _, slug := range datasets {
		cfg, ok := models.DatasetConfigBySlug(slug)
		if !ok {
			respondBadRequest(c, fmt.Sprintf("Unbekannter Datensatz: %q", slug))
			return
		}

		for _, col := range cfg.ValueColumns {
			result, err := h.computeColumnMetrics(cfg, col, filter)
			if err != nil {
				respondInternalError(c, fmt.Sprintf("Fehler bei Metriken für %s/%s: %v", slug, col, err))
				return
			}
			results = append(results, result)
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    results,
	})
}

// computeColumnMetrics computes the metrics for one dataset column.
func (h *Handler) computeColumnMetrics(cfg models.KreisDatasetConfig, col string, filter models.MetricsFilter) (models.DatasetMetricsResult, error) {
	quotedCol := fmt.Sprintf(`"%s"`, col)

	// Aggregation query: per-year statistics over all matching regions
	aggWhere, aggArgs, _ := buildMetricsWhere(filter, false, 1, "")
	aggQuery := fmt.Sprintf(
		`SELECT year,`+
			` COUNT(*) AS region_count,`+
			` MIN(%s) AS min_val,`+
			` MAX(%s) AS max_val,`+
			` AVG(%s) AS mean_val,`+
			` STDDEV_POP(%s) AS stddev_val,`+
			` PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY %s) AS median_val`+
			` FROM %s.%s`+
			` WHERE has_valid_value = true%s`+
			` GROUP BY year`+
			` ORDER BY year`,
		quotedCol, quotedCol, quotedCol, quotedCol, quotedCol,
		cfg.Schema, cfg.Table,
		aggWhere,
	)

	var aggRows []metricsAggRow
	if err := h.DB.Select(&aggRows, aggQuery, aggArgs...); err != nil {
		return models.DatasetMetricsResult{}, fmt.Errorf("Aggregationsabfrage fehlgeschlagen: %w", err)
	}

	byYear := make([]models.DatasetYearMetrics, len(aggRows))
	yearIndex := make(map[int]int, len(aggRows))
	for i, row := range aggRows {
		byYear[i] = models.DatasetYearMetrics{
			Year:        row.Year,
			RegionCount: row.RegionCount,
			Min:         row.MinVal,
			Max:         row.MaxVal,
			Mean:        row.MeanVal,
			Median:      row.MedianVal,
			StdDev:      row.StddevVal,
		}
		yearIndex[row.Year] = i
	}

	// Ranking query: only when a specific region is requested
	if filter.GeodataIDNUTS != "" {
		// WHERE without geodata_id_nuts, so the ranking spans all regions of the filtered set
		rankWhere, rankArgs, rankArgPos := buildMetricsWhere(filter, true, 1, "")
		rankQuery := fmt.Sprintf(
			`WITH ranked AS (`+
				` SELECT year, geodata_id_nuts, %s AS value,`+
				` RANK() OVER (PARTITION BY year ORDER BY %s) AS rank,`+
				` COUNT(*) OVER (PARTITION BY year) AS total`+
				` FROM %s.%s WHERE has_valid_value = true%s`+
				`)`+
				` SELECT year, value, rank::int AS rank, total::int AS total`+
				` FROM ranked`+
				` WHERE geodata_id_nuts = $%d`+
				` ORDER BY year`,
			quotedCol, quotedCol,
			cfg.Schema, cfg.Table,
			rankWhere,
			rankArgPos,
		)
		rankArgs = append(rankArgs, filter.GeodataIDNUTS)

		var rankRows []metricsRankRow
		if err := h.DB.Select(&rankRows, rankQuery, rankArgs...); err != nil {
			return models.DatasetMetricsResult{}, fmt.Errorf("Ranking-Abfrage fehlgeschlagen: %w", err)
		}

		for _, rr := range rankRows {
			if idx, ok := yearIndex[rr.Year]; ok {
				val := rr.Value
				r := rr.Rank
				t := rr.Total
				byYear[idx].RegionalValue = &val
				byYear[idx].RegionalRank = &r
				byYear[idx].RegionalRankTotal = &t
			}
		}
	}

	// Year-over-year calculation in Go for consecutive year pairs
	for i := 1; i < len(byYear); i++ {
		prev := byYear[i-1]
		curr := &byYear[i]
		if curr.Year != prev.Year+1 {
			continue
		}
		if prev.Mean != 0 {
			pct := (curr.Mean - prev.Mean) / prev.Mean * 100
			curr.YoYMeanPct = &pct
		}
		if prev.Median != 0 {
			pct := (curr.Median - prev.Median) / prev.Median * 100
			curr.YoYMedianPct = &pct
		}
	}

	return models.DatasetMetricsResult{
		Dataset: cfg.RouteSlug,
		Column:  col,
		ByYear:  byYear,
	}, nil
}

// buildMetricsWhere builds the WHERE clause for metrics queries.
// excludeNutsID: if true, geodata_id_nuts is not added as a filter
// (for ranking queries that must rank across all regions of a set).
// tableAlias: optional table-alias prefix for column names (e.g. "n" -> "n.geodata_id_nuts").
// Leave empty for simple queries without an alias.
func buildMetricsWhere(filter models.MetricsFilter, excludeNutsID bool, argPos int, tableAlias string) (string, []interface{}, int) {
	prefix := ""
	if tableAlias != "" {
		prefix = tableAlias + "."
	}

	where := ""
	args := make([]interface{}, 0, 4)

	if !excludeNutsID && filter.GeodataIDNUTS != "" {
		where += fmt.Sprintf(" AND %sgeodata_id_nuts = $%d", prefix, argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.NutsParentID != "" {
		normalized := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		where += fmt.Sprintf(" AND %sgeodata_id_nuts LIKE $%d", prefix, argPos)
		args = append(args, normalized+"%")
		argPos++
	}

	if filter.Year != nil {
		where += fmt.Sprintf(" AND %syear = $%d", prefix, argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	if filter.FromYear != nil {
		where += fmt.Sprintf(" AND %syear >= $%d", prefix, argPos)
		args = append(args, *filter.FromYear)
		argPos++
	}

	if filter.ToYear != nil {
		where += fmt.Sprintf(" AND %syear <= $%d", prefix, argPos)
		args = append(args, *filter.ToYear)
		argPos++
	}

	return where, args, argPos
}
