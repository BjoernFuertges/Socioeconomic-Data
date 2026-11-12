package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// metricsAggRow is used internally to scan the aggregation query. Year is
// NULL for the grand-total row (GROUPING SETS); the statistics are NULL for a
// year without any valid value.
type metricsAggRow struct {
	Year                      *int     `db:"year"`
	RegionCount               int      `db:"region_count"`
	MinVal                    *float64 `db:"min_val"`
	MaxVal                    *float64 `db:"max_val"`
	MeanVal                   *float64 `db:"mean_val"`
	StddevVal                 *float64 `db:"stddev_val"`
	MedianVal                 *float64 `db:"median_val"`
	ExcludedInvalidCount      int      `db:"excluded_invalid_count"`
	RegionsWithoutGeometry    int      `db:"regions_without_geometry"`
	RegionsWithoutDenominator int      `db:"regions_without_denominator"`
	ExcludedRegions           int      `db:"excluded_regions"`
}

// Values of excluded_reason in a metricsSource; NULL means the value is used.
const (
	metricsExcludedInvalid       = "invalid"
	metricsExcludedNoGeometry    = "no_geometry"
	metricsExcludedNoDenominator = "no_denominator"
)

// metricsValidityNote is the meta.note of the metrics endpoints.
const metricsValidityNote = "Nur gültige Werte gehen in die Statistik ein (has_valid_value der Staging-Modelle; harmonisierte Werte nur, wenn alle beteiligten Quellwerte gültig sind). " +
	"Ausgeschlossene Werte stehen in excluded_invalid_count, betroffene Regionen in excluded_regions; " +
	"bei abgeleiteten Kennzahlen müssen Zähler und Nenner gültig sein, Regionen ohne Geometrie bzw. Nennerwert stehen in regions_without_geometry bzw. regions_without_denominator."

// metricsMaxParallel bounds the concurrent DB queries of one GET /metrics
// request (one per dataset column).
const metricsMaxParallel = 4

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
// @Description Harmonisierte Kreis-Datensätze werden aus mart_nuts3_sociodata_harmonized im NUTS-3-Stand 'nuts_version' berechnet (Default 2024; die Antwort nennt ihn in nuts_version), sodass jede Region je Jahr genau einmal zählt.
// @Description Nur gültige Werte gehen ein (has_valid_value; harmonisierte Werte nur, wenn alle beteiligten Quellwerte gültig sind); ausgeschlossene Werte zählt excluded_invalid_count je Jahr und Spalte, excluded_regions die betroffenen Regionen, meta.note erklärt das.
// @Description Spalten ohne harmonisierte Entsprechung (beschaeftigungsquote_median, beschaeftigungsquote_ungewichtet, gemeinden_count von beschaeftigungsquote-nuts3) und nicht harmonisierte Datensätze (beschaeftigungsquote-gemeinden) kommen weiter ungeharmonisiert aus der Staging-Tabelle (nuts_version fehlt dann in der Antwort).
// @Tags Metriken
// @Accept json
// @Produce json
// @Param dataset query []string true "Datensatz-Slug(s), z.B. schuldnerquote (mehrfach angeben für mehrere Datensätze)" collectionFormat(multi) example(schuldnerquote)
// @Param year query int false "Einzelnes Jahr filtern" example(2022)
// @Param from_year query int false "Jahresbereich ab" example(2018)
// @Param to_year query int false "Jahresbereich bis" example(2022)
// @Param nuts_parent_id query string false "Auf NUTS-Präfix einschränken (z.B. DE1 für alle Kreise in Bayern)" example(DE1)
// @Param geodata_id_nuts query string false "NUTS-ID einer Region für Regionalwert und Ranking (im Stand nuts_version)" example(DEF01)
// @Param nuts_version query int false "NUTS-3-Zielversion der harmonisierten Werte" Enums(2003,2006,2010,2013,2016,2021,2024) default(2024)
// @Success 200 {object} models.APIResponse{data=[]models.DatasetMetricsResult}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Failure 503 {object} models.APIResponse
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
	nutsVersion, err := resolveMetricsNutsVersion(filter)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	// Resolve every source before the first DB access, so an unknown slug is a
	// 400 regardless of its position in the list.
	type pending struct {
		slug string
		col  string
		src  metricsSource
	}
	work := make([]pending, 0, len(datasets))
	for _, slug := range datasets {
		cfg, ok := models.DatasetConfigBySlug(slug)
		if !ok {
			respondBadRequest(c, fmt.Sprintf("Unbekannter Datensatz: %q", slug))
			return
		}
		for _, col := range cfg.ValueColumns {
			work = append(work, pending{slug: slug, col: col, src: datasetMetricsSource(cfg, col, nutsVersion)})
		}
	}

	// One query set per column, at most metricsMaxParallel at a time; results
	// keep the order of the request.
	results := make([]models.DatasetMetricsResult, len(work))
	failed, err := runIndexedLimited(c.Request.Context(), len(work), metricsMaxParallel, func(ctx context.Context, i int) error {
		result, err := h.computeMetrics(ctx, work[i].src, filter)
		if err != nil {
			return err
		}
		result.Dataset = work[i].slug
		result.Column = work[i].col
		results[i] = result
		return nil
	})
	if err != nil {
		op := "Fehler bei Metriken"
		if failed >= 0 {
			op = fmt.Sprintf("Fehler bei Metriken für %s/%s", work[failed].slug, work[failed].col)
		}
		respondDBError(c, op, err)
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    results,
		Meta:    &models.MetaInfo{Note: metricsValidityNote},
	})
}

// runIndexedLimited calls fn(ctx, 0..n-1) with at most limit calls running at
// once. The first error cancels the context passed to the remaining calls and
// is returned with its index; calls not yet started are skipped. If the
// parent context ends before all calls ran, its error is returned with index
// -1. Callers write results by index, so the order of the input is kept.
func runIndexedLimited(parent context.Context, n int, limit int, fn func(ctx context.Context, i int) error) (int, error) {
	if limit < 1 {
		limit = 1
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		firstIdx = -1
	)
	sem := make(chan struct{}, limit)
	for i := 0; i < n; i++ {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, i); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr, firstIdx = err, i
					cancel()
				}
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		return firstIdx, firstErr
	}
	return -1, parent.Err()
}

// resolveMetricsNutsVersion validates ?nuts_version= (default 2024), with the
// same allowed values as SozioKreisFilter.NutsVersion on /{slug}/timeseries.
func resolveMetricsNutsVersion(filter models.MetricsFilter) (int, error) {
	if filter.NutsVersion == nil {
		return defaultNutsVersion, nil
	}
	if !isSupportedNutsVersion(*filter.NutsVersion) {
		return 0, fmt.Errorf("nuts_version %d wird nicht unterstützt (erlaubt: %v)", *filter.NutsVersion, supportedNutsVersions)
	}
	return *filter.NutsVersion, nil
}

// metricsSource is the row source of a metrics computation: an SQL fragment
// (subquery with alias src) that yields the columns geodata_id_nuts, year,
// value and excluded_reason (NULL = value is used, else one of the
// metricsExcluded* constants), plus its bind args ($1..$n). Excluded rows stay
// in the source so they can be counted. All aggregation/ranking SQL runs on top
// of it, so the harmonized and the staging source share one code path.
type metricsSource struct {
	SQL  string
	Args []interface{}
	// NutsVersion is set when the values are harmonized to that NUTS-3 version.
	NutsVersion *int
}

// harmonizedMetricsColumns maps datasets with several ValueColumns to the one
// column whose values mart_nuts3_sociodata_harmonized carries as
// harmonized_value (see the source_datasets/beschaeftigungsquote-nuts3 union
// there). For single-column datasets it is that column.
var harmonizedMetricsColumns = map[string]string{
	"beschaeftigungsquote-nuts3": "beschaeftigungsquote_prozent",
}

// harmonizedMetricsColumn reports whether col of cfg is available as
// harmonized_value in mart_nuts3_sociodata_harmonized.
func harmonizedMetricsColumn(cfg models.KreisDatasetConfig, col string) bool {
	if !cfg.IsHarmonized {
		return false
	}
	if mapped, ok := harmonizedMetricsColumns[cfg.RouteSlug]; ok {
		return mapped == col
	}
	return len(cfg.ValueColumns) == 1 && cfg.ValueColumns[0] == col
}

// datasetMetricsSource returns the source for one dataset column: the
// harmonized mart filtered to nutsVersion where possible (each region counted
// once per year, comparable across years), otherwise the staging table
// (unharmonized). Either way only rows with has_valid_value and a non-NULL
// value are used; the others are marked excluded_reason = 'invalid'. The
// harmonized has_valid_value is false as soon as one contributing source
// value is invalid (see mart_nuts3_sociodata_harmonized). col must be one of
// cfg.ValueColumns (checked by the callers) - it is quoted into the SQL.
func datasetMetricsSource(cfg models.KreisDatasetConfig, col string, nutsVersion int) metricsSource {
	if harmonizedMetricsColumn(cfg, col) {
		v := nutsVersion
		return metricsSource{
			SQL: `(SELECT target_nuts_id AS geodata_id_nuts, year, harmonized_value::float8 AS value,` +
				` CASE WHEN has_valid_value AND harmonized_value IS NOT NULL THEN NULL ELSE '` + metricsExcludedInvalid + `' END AS excluded_reason` +
				` FROM transformed_data.mart_nuts3_sociodata_harmonized` +
				` WHERE dataset_slug = $1 AND target_version = $2::int) src`,
			Args:        []interface{}{cfg.RouteSlug, nutsVersion},
			NutsVersion: &v,
		}
	}
	return metricsSource{
		SQL: fmt.Sprintf(`(SELECT geodata_id_nuts, year, "%[1]s"::float8 AS value,`+
			` CASE WHEN has_valid_value AND "%[1]s" IS NOT NULL THEN NULL ELSE '%[4]s' END AS excluded_reason`+
			` FROM %[2]s.%[3]s) src`,
			col, cfg.Schema, cfg.Table, metricsExcludedInvalid),
	}
}

// computeMetrics computes the per-year statistics (and, with geodata_id_nuts,
// the region's value and rank) over a metricsSource. Dataset/Column of the
// result are left to the caller.
func (h *Handler) computeMetrics(ctx context.Context, src metricsSource, filter models.MetricsFilter) (models.DatasetMetricsResult, error) {
	aggQuery, aggArgs := metricsAggregateQuery(src, filter)

	var aggRows []metricsAggRow
	if err := h.DB.SelectContext(ctx, &aggRows, aggQuery, aggArgs...); err != nil {
		return models.DatasetMetricsResult{}, fmt.Errorf("Aggregationsabfrage fehlgeschlagen: %w", err)
	}
	result, byYear := metricsResultFromAggRows(aggRows)
	yearIndex := make(map[int]int, len(byYear))
	for i, y := range byYear {
		yearIndex[y.Year] = i
	}

	// Ranking query: only when a specific region is requested
	if filter.GeodataIDNUTS != "" {
		rankQuery, rankArgs := metricsRankQuery(src, filter)

		var rankRows []metricsRankRow
		if err := h.DB.SelectContext(ctx, &rankRows, rankQuery, rankArgs...); err != nil {
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

	addYoYChanges(byYear)

	result.NutsVersion = src.NutsVersion
	result.ByYear = byYear
	return result, nil
}

// metricsResultFromAggRows splits the aggregation rows into the per-year
// metrics (years with at least one valid value) and the totals of the
// grand-total row (year NULL). Excluded counts of years without any valid
// value only show up in the totals.
func metricsResultFromAggRows(rows []metricsAggRow) (models.DatasetMetricsResult, []models.DatasetYearMetrics) {
	var result models.DatasetMetricsResult
	byYear := make([]models.DatasetYearMetrics, 0, len(rows))
	for _, row := range rows {
		if row.Year == nil {
			result.ExcludedInvalidCount = row.ExcludedInvalidCount
			result.ExcludedRegions = row.ExcludedRegions
			result.RegionsWithoutGeometry = row.RegionsWithoutGeometry
			result.RegionsWithoutDenominator = row.RegionsWithoutDenominator
			continue
		}
		if row.RegionCount == 0 {
			continue
		}
		byYear = append(byYear, models.DatasetYearMetrics{
			Year:                      *row.Year,
			RegionCount:               row.RegionCount,
			Min:                       derefFloat(row.MinVal),
			Max:                       derefFloat(row.MaxVal),
			Mean:                      derefFloat(row.MeanVal),
			Median:                    derefFloat(row.MedianVal),
			StdDev:                    derefFloat(row.StddevVal),
			ExcludedInvalidCount:      row.ExcludedInvalidCount,
			RegionsWithoutGeometry:    row.RegionsWithoutGeometry,
			RegionsWithoutDenominator: row.RegionsWithoutDenominator,
		})
	}
	return result, byYear
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// metricsAggregateQuery builds the per-year statistics over the valid values
// of all regions of the source that match the filter, the counts of excluded
// values per reason, and (GROUPING SETS) one grand-total row with year NULL
// whose counts of distinct regions span all years. In the per-year rows the
// region counts are values (one per region and year).
func metricsAggregateQuery(src metricsSource, filter models.MetricsFilter) (string, []interface{}) {
	where, whereArgs, _ := buildMetricsWhere(filter, false, len(src.Args)+1, "")
	used := ` FILTER (WHERE excluded_reason IS NULL)`
	query := `SELECT year,` +
		` COUNT(*)` + used + ` AS region_count,` +
		` MIN(value)` + used + ` AS min_val,` +
		` MAX(value)` + used + ` AS max_val,` +
		` AVG(value)` + used + ` AS mean_val,` +
		` STDDEV_POP(value)` + used + ` AS stddev_val,` +
		` PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY value)` + used + ` AS median_val,` +
		` COUNT(*) FILTER (WHERE excluded_reason = '` + metricsExcludedInvalid + `') AS excluded_invalid_count,` +
		` COUNT(DISTINCT geodata_id_nuts) FILTER (WHERE excluded_reason = '` + metricsExcludedNoGeometry + `') AS regions_without_geometry,` +
		` COUNT(DISTINCT geodata_id_nuts) FILTER (WHERE excluded_reason = '` + metricsExcludedNoDenominator + `') AS regions_without_denominator,` +
		` COUNT(DISTINCT geodata_id_nuts) FILTER (WHERE excluded_reason IS NOT NULL) AS excluded_regions` +
		` FROM ` + src.SQL +
		` WHERE true` + where +
		` GROUP BY GROUPING SETS ((year), ())` +
		` ORDER BY year NULLS LAST`
	return query, append(append([]interface{}{}, src.Args...), whereArgs...)
}

// metricsRankQuery ranks filter.GeodataIDNUTS among all regions of the
// filtered set with a valid value (the WHERE deliberately omits
// geodata_id_nuts itself); a region whose own value is excluded gets no rank.
func metricsRankQuery(src metricsSource, filter models.MetricsFilter) (string, []interface{}) {
	where, whereArgs, argPos := buildMetricsWhere(filter, true, len(src.Args)+1, "")
	query := fmt.Sprintf(
		`WITH ranked AS (`+
			` SELECT year, geodata_id_nuts, value,`+
			` RANK() OVER (PARTITION BY year ORDER BY value) AS rank,`+
			` COUNT(*) OVER (PARTITION BY year) AS total`+
			` FROM %s WHERE excluded_reason IS NULL%s`+
			`)`+
			` SELECT year, value, rank::int AS rank, total::int AS total`+
			` FROM ranked`+
			` WHERE geodata_id_nuts = $%d`+
			` ORDER BY year`,
		src.SQL, where, argPos,
	)
	args := append(append([]interface{}{}, src.Args...), whereArgs...)
	return query, append(args, filter.GeodataIDNUTS)
}

// addYoYChanges fills the year-over-year changes for consecutive year pairs.
func addYoYChanges(byYear []models.DatasetYearMetrics) {
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
