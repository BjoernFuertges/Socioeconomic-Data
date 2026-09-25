package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// GetDerivedMetricsCatalog gibt alle abgeleiteten Metriken-Definitionen zurück.
// @Summary Abgeleitete Metriken-Definitionen
// @Description Gibt alle abgeleiteten Metriken aus dem Katalog zurück (z.B. Bevölkerungsdichte,
// @Description Ausbildungsquote). Jede Metrik wird per Formel aus mehreren Datensätzen berechnet.
// @Description Mit GET /metrics/derived/:slug können die statistischen Werte abgerufen werden.
// @Tags Metriken
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.DerivedMetricEntry}
// @Router /metrics/derived [get]
func (h *Handler) GetDerivedMetricsCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    h.DerivedCatalog,
	})
}

// GetDerivedMetricsCatalogEntry berechnet Statistiken für eine abgeleitete Metrik.
// @Summary Statistiken für eine abgeleitete Metrik
// @Description Berechnet Min, Max, Mittelwert, Median, Stddev und YoY-Veränderung für eine
// @Description abgeleitete Metrik (z.B. Bevölkerungsdichte = Bevölkerung / Fläche in km²).
// @Tags Metriken
// @Accept json
// @Produce json
// @Param slug path string true "Slug der abgeleiteten Metrik" example(bevoelkerungsdichte)
// @Param year query int false "Einzelnes Jahr filtern" example(2022)
// @Param from_year query int false "Jahresbereich ab" example(2018)
// @Param to_year query int false "Jahresbereich bis" example(2022)
// @Param nuts_parent_id query string false "Auf NUTS-Präfix einschränken (z.B. DE1)" example(DE1)
// @Param geodata_id_nuts query string false "NUTS-ID einer Region für Regionalwert und Ranking" example(DEF01)
// @Success 200 {object} models.APIResponse{data=models.DatasetMetricsResult}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /metrics/derived/{slug} [get]
func (h *Handler) GetDerivedMetricsCatalogEntry(c *gin.Context) {
	slug := c.Param("slug")

	entry, ok := h.DerivedCatalog.FindBySlug(slug)
	if !ok {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Abgeleitete Metrik %q nicht gefunden", slug),
		})
		return
	}

	var filter models.MetricsFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	result, err := h.computeDerivedMetric(entry, filter)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler bei abgeleiteter Metrik %q: %v", slug, err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
	})
}

// columnAllowed prüft ob eine Spalte in den ValueColumns eines Datasets erlaubt ist.
// Verhindert das Einschleusen beliebiger Spaltennamen in SQL-Abfragen.
func columnAllowed(cfg models.KreisDatasetConfig, col string) bool {
	for _, c := range cfg.ValueColumns {
		if c == col {
			return true
		}
	}
	return false
}

// computeDerivedMetric berechnet Statistiken für eine abgeleitete Metrik per SQL-JOIN.
// Unterstützt aktuell: type=ratio (Zähler/Nenner × Scale).
// Nenner-Quellen: dataset (RouteSlug+Spalte) oder builtin "nuts_geometry_area_sqkm".
func (h *Handler) computeDerivedMetric(entry models.DerivedMetricEntry, filter models.MetricsFilter) (models.DatasetMetricsResult, error) {
	if entry.Formula.Type != "ratio" {
		return models.DatasetMetricsResult{}, fmt.Errorf("unbekannter Formeltyp: %q (unterstützt: ratio)", entry.Formula.Type)
	}

	// Zähler validieren
	numCfg, ok := models.DatasetConfigBySlug(entry.Formula.Numerator.Dataset)
	if !ok {
		return models.DatasetMetricsResult{}, fmt.Errorf("unbekannter Datensatz im Zähler: %q", entry.Formula.Numerator.Dataset)
	}
	if !columnAllowed(numCfg, entry.Formula.Numerator.Column) {
		return models.DatasetMetricsResult{}, fmt.Errorf("unbekannte Spalte im Zähler: %q", entry.Formula.Numerator.Column)
	}
	numCol := fmt.Sprintf(`"%s"`, entry.Formula.Numerator.Column)

	// Nenner auflösen
	var denomCTE, denomJoin, denomExpr string
	switch {
	case entry.Formula.Denominator.Builtin == "nuts_geometry_area_sqkm":
		denomCTE = `area_cte AS (
		SELECT nuts_id, valid_from, valid_to, ST_Area(geometry_3035) / 1e6 AS area_sqkm
		FROM transformed_data.mart_nuts_geometry_historized
		WHERE has_valid_geometry
	), `
		denomJoin = `INNER JOIN area_cte dc ON n.geodata_id_nuts = dc.nuts_id AND n.year >= dc.valid_from AND (dc.valid_to IS NULL OR n.year <= dc.valid_to)`
		denomExpr = `dc.area_sqkm`
	case entry.Formula.Denominator.Dataset != "":
		denCfg, ok := models.DatasetConfigBySlug(entry.Formula.Denominator.Dataset)
		if !ok {
			return models.DatasetMetricsResult{}, fmt.Errorf("unbekannter Datensatz im Nenner: %q", entry.Formula.Denominator.Dataset)
		}
		if !columnAllowed(denCfg, entry.Formula.Denominator.Column) {
			return models.DatasetMetricsResult{}, fmt.Errorf("unbekannte Spalte im Nenner: %q", entry.Formula.Denominator.Column)
		}
		denomJoin = fmt.Sprintf(
			`INNER JOIN %s.%s dc ON n.geodata_id_nuts = dc.geodata_id_nuts AND n.year = dc.year AND dc.has_valid_value = true`,
			denCfg.Schema, denCfg.Table,
		)
		denomExpr = fmt.Sprintf(`dc."%s"`, entry.Formula.Denominator.Column)
	default:
		return models.DatasetMetricsResult{}, fmt.Errorf("keine gültige Nenner-Quelle definiert (dataset oder builtin erforderlich)")
	}

	scale := entry.Formula.Scale
	if scale == 0 {
		scale = 1.0
	}
	scaleLiteral := strconv.FormatFloat(scale, 'f', -1, 64)
	valueExpr := fmt.Sprintf(`(n.%s / NULLIF(%s, 0)) * %s`, numCol, denomExpr, scaleLiteral)

	// WHERE-Filter für den Zähler (mit Alias "n.")
	numWhere, numArgs, _ := buildMetricsWhere(filter, false, 1, "n")

	// Aggregations-Query mit CTE
	aggQuery := fmt.Sprintf(
		`WITH %sjoined AS (
		SELECT n.year, n.geodata_id_nuts, %s AS value
		FROM %s.%s n
		%s
		WHERE n.has_valid_value = true%s
	)
	SELECT year,
		COUNT(*)           AS region_count,
		MIN(value)         AS min_val,
		MAX(value)         AS max_val,
		AVG(value)         AS mean_val,
		STDDEV_POP(value)  AS stddev_val,
		PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY value) AS median_val
	FROM joined
	GROUP BY year
	ORDER BY year`,
		denomCTE,
		valueExpr,
		numCfg.Schema, numCfg.Table,
		denomJoin,
		numWhere,
	)

	var aggRows []metricsAggRow
	if err := h.DB.Select(&aggRows, aggQuery, numArgs...); err != nil {
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

	// Ranking-Query: nur wenn eine spezifische Region abgefragt wird
	if filter.GeodataIDNUTS != "" {
		rankWhere, rankArgs, rankArgPos := buildMetricsWhere(filter, true, 1, "n")

		rankQuery := fmt.Sprintf(
			`WITH %sjoined AS (
			SELECT n.year, n.geodata_id_nuts, %s AS value
			FROM %s.%s n
			%s
			WHERE n.has_valid_value = true%s
		), ranked AS (
			SELECT year, geodata_id_nuts, value,
				RANK() OVER (PARTITION BY year ORDER BY value) AS rank,
				COUNT(*) OVER (PARTITION BY year)              AS total
			FROM joined
		)
		SELECT year, value, rank::int AS rank, total::int AS total
		FROM ranked
		WHERE geodata_id_nuts = $%d
		ORDER BY year`,
			denomCTE,
			valueExpr,
			numCfg.Schema, numCfg.Table,
			denomJoin,
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

	// Year-over-Year-Berechnung in Go für konsekutive Jahrespaare
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
		Dataset: entry.Slug,
		Column:  entry.Unit,
		ByYear:  byYear,
	}, nil
}
