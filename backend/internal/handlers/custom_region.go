package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

const (
	customRegionMaxBodyBytes = 5 << 20 // 5 MiB GeoJSON
	customRegionMaxPoints    = 200000
	customRegionMaxDatasets  = 30
	customRegionDefaultShare = 0.01
	// From this area match (intersection / union) the polygon counts as exactly
	// one existing unit. Not 1.0, because reprojection (4326 -> 3035) and
	// coordinate rounding in the GeoJSON introduce tiny deviations.
	customRegionExactMatchIoU = 0.999
)

// CustomRegionAggregateRequest describes a dataset query for a freely defined
// region (GeoJSON). The region is never stored.
type CustomRegionAggregateRequest struct {
	Geometry json.RawMessage `json:"geometry" swaggertype:"object"`
	Datasets []string        `json:"datasets" example:"bevoelkerung,schuldnerquote"`
	FromYear *int            `json:"from_year,omitempty" example:"2015"`
	ToYear   *int            `json:"to_year,omitempty" example:"2024"`
	MinShare *float64        `json:"min_share,omitempty" example:"0.01"`
}

// CustomRegionAggregateRow is one row of the aggregated time series.
type CustomRegionAggregateRow struct {
	DatasetSlug       string  `json:"dataset_slug" example:"schuldnerquote"`
	Indicator         string  `json:"indicator" example:"schuldnerquote"`
	LevelUsed         string  `json:"level_used" example:"nuts3"`
	Year              int     `json:"year" example:"2023"`
	Month             *int    `json:"month,omitempty" example:"6"`
	Value             float64 `json:"value" example:"7.4"`
	ValueKind         string  `json:"value_kind" example:"rate"`
	AggregationMethod string  `json:"aggregation_method" example:"weighted_avg_population"`
	SourceRegionCount int     `json:"source_region_count" example:"3"`
	Coverage          float64 `json:"coverage" example:"0.98"`
	// MatchedUnit is set when the polygon matches exactly one existing unit of
	// the level used (nuts_id, gisco_id or ba_schl); the values are then that
	// unit's values, without contributions from neighbouring units.
	MatchedUnit *string `json:"matched_unit,omitempty" example:"DE300"`
	// IsCrosswalked: at least one contributing unit value was converted across a
	// version boundary (not a direct value).
	IsCrosswalked bool `json:"is_crosswalked" example:"false"`
}

type customRegionMeta struct {
	DatasetSlug string  `db:"dataset_slug" json:"dataset_slug" example:"schuldnerquote"`
	Indicator   string  `db:"indicator" json:"indicator" example:"schuldnerquote"`
	Level       string  `db:"level" json:"level" example:"nuts3"`
	MeasureType string  `db:"measure_type" json:"measure_type" example:"weighted_avg"`
	Kind        string  `db:"kind" json:"kind" example:"rate"`
	FinerSlug   *string `db:"finer_slug" json:"finer_slug"`
}

// customRegionAggRow is an intermediate SQL row (per year/month) from which
// computeCustomRegionValue derives the final value.
type customRegionAggRow struct {
	Year              int             `db:"year"`
	Month             sql.NullInt64   `db:"month"`
	SourceRegionCount int             `db:"source_region_count"`
	SumValue          sql.NullFloat64 `db:"sum_value"`
	AreaNum           sql.NullFloat64 `db:"area_num"`
	AreaDen           sql.NullFloat64 `db:"area_den"`
	PlainAvg          sql.NullFloat64 `db:"plain_avg"`
	PopNum            sql.NullFloat64 `db:"pop_num"`
	PopDen            sql.NullFloat64 `db:"pop_den"`
	PopComplete       bool            `db:"pop_complete"`
	EmpNum            sql.NullFloat64 `db:"emp_num"`
	EmpDen            sql.NullFloat64 `db:"emp_den"`
	EmpComplete       bool            `db:"emp_complete"`
	Coverage          float64         `db:"coverage"`
	IsCrosswalked     bool            `db:"is_crosswalked"`
}

// customRegionUnitsSQL returns the unit geometries per level (EPSG:3035,
// reference vintage 2024, as in mart_agentur_nuts3_crosswalk). Fixed fragments
// only, never user input.
var customRegionUnitsSQL = map[string]string{
	"nuts3": `SELECT nuts_id AS unit_id, geometry_3035 AS g
		FROM transformed_data.mart_nuts_geometry_historized
		WHERE levl_code = 3 AND country_code = 'DE' AND has_valid_geometry
		  AND valid_from <= 2024 AND (valid_to IS NULL OR valid_to >= 2024)`,
	"lau": `SELECT gisco_id AS unit_id, geometry_3035 AS g
		FROM transformed_data.mart_lau_geometry_historized
		WHERE has_valid_geometry AND country_code = 'DE'
		  AND valid_from <= 2024 AND max_vintage_year >= 2024`,
	"agentur": `SELECT ba_schl AS unit_id, ST_Transform(geometry_wgs84, 3035) AS g
		FROM transformed_data.stg_agenturbezirk_geometry
		WHERE has_valid_geometry`,
}

const customRegionPolyCTE = `poly AS (
	SELECT ST_Multi(ST_CollectionExtract(
		ST_UnaryUnion(ST_MakeValid(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326), 3035))), 3)) AS g
)`

// customRegionExactMatchSQL finds the unit of a level whose area is (nearly)
// identical to the polygon: intersection area / union area >= $2.
const customRegionExactMatchSQL = `
WITH ` + customRegionPolyCTE + `,
units AS (%s)
SELECT u.unit_id::text
FROM units u
JOIN poly p ON ST_Intersects(u.g, p.g)
WHERE ST_Area(ST_Intersection(u.g, p.g))
	/ NULLIF(ST_Area(u.g) + ST_Area(p.g) - ST_Area(ST_Intersection(u.g, p.g)), 0) >= $2::float8
ORDER BY ST_Area(ST_Intersection(u.g, p.g)) DESC
LIMIT 1`

// $8 (optional): unit to which the aggregation is restricted when the polygon
// exactly matches it (see customRegionExactMatchSQL).
const customRegionAggregateSQL = `
WITH ` + customRegionPolyCTE + `,
units AS (%s),
overlap AS (
	SELECT u.unit_id,
		ST_Area(ST_Intersection(u.g, p.g)) / NULLIF(ST_Area(u.g), 0) AS share_of_unit,
		ST_Area(ST_Intersection(u.g, p.g)) / NULLIF(ST_Area(p.g), 0) AS share_of_poly
	FROM units u
	JOIN poly p ON ST_Intersects(u.g, p.g)
),
kept AS (
	SELECT * FROM overlap
	WHERE ($8::text IS NOT NULL AND unit_id::text = $8::text)
	   OR ($8::text IS NULL AND (share_of_unit >= $2::float8 OR share_of_poly >= $2::float8))
),
vals AS (
	SELECT k.share_of_unit, k.share_of_poly, v.year, v.month, v.value, v.population, v.employees, v.is_crosswalked
	FROM kept k
	JOIN transformed_data.mart_custom_region_unit_values v
		ON  v.level = $3 AND v.unit_id = k.unit_id
		AND v.dataset_slug = $4 AND v.indicator = $5
		AND ($6::int IS NULL OR v.year >= $6::int)
		AND ($7::int IS NULL OR v.year <= $7::int)
)
SELECT
	year, month,
	count(*)::int                                          AS source_region_count,
	sum(value * share_of_unit)                             AS sum_value,
	sum(value * share_of_poly)                             AS area_num,
	sum(share_of_poly)                                     AS area_den,
	avg(value)                                             AS plain_avg,
	sum(value * population * share_of_unit)                AS pop_num,
	sum(population * share_of_unit)                        AS pop_den,
	(count(population) = count(*)
		AND coalesce(sum(population * share_of_unit), 0) > 0) AS pop_complete,
	sum(value * employees * share_of_unit)                 AS emp_num,
	sum(employees * share_of_unit)                         AS emp_den,
	(count(employees) = count(*)
		AND coalesce(sum(employees * share_of_unit), 0) > 0) AS emp_complete,
	least(1.0, sum(share_of_poly))::float8                 AS coverage,
	bool_or(is_crosswalked)                                AS is_crosswalked
FROM vals
GROUP BY year, month
ORDER BY year, month`

// normalizeCustomRegionRequest validates the request and returns the GeoJSON
// geometry object, the (deduplicated) dataset slugs and min_share.
func normalizeCustomRegionRequest(req *CustomRegionAggregateRequest) (geometryJSON string, datasets []string, minShare float64, err error) {
	geometryJSON, err = extractGeoJSONGeometry(req.Geometry)
	if err != nil {
		return "", nil, 0, err
	}

	var geom struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(geometryJSON), &geom); err != nil {
		return "", nil, 0, fmt.Errorf("ungültiges GeoJSON: %v", err)
	}
	if geom.Type != "Polygon" && geom.Type != "MultiPolygon" {
		return "", nil, 0, fmt.Errorf("geometry muss ein Polygon oder MultiPolygon sein, erhalten %q", geom.Type)
	}

	seen := map[string]bool{}
	for _, raw := range req.Datasets {
		slug := strings.TrimSpace(raw)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		datasets = append(datasets, slug)
	}
	if len(datasets) == 0 {
		return "", nil, 0, fmt.Errorf("datasets ist erforderlich (mindestens ein Datensatz-Slug)")
	}
	if len(datasets) > customRegionMaxDatasets {
		return "", nil, 0, fmt.Errorf("maximal %d Datensätze pro Anfrage erlaubt", customRegionMaxDatasets)
	}

	if req.FromYear != nil && req.ToYear != nil && *req.FromYear > *req.ToYear {
		return "", nil, 0, fmt.Errorf("from_year darf nicht größer als to_year sein")
	}

	minShare = customRegionDefaultShare
	if req.MinShare != nil {
		minShare = *req.MinShare
		if minShare < 0 || minShare >= 1 {
			return "", nil, 0, fmt.Errorf("min_share muss im Bereich [0, 1) liegen")
		}
	}
	return geometryJSON, datasets, minShare, nil
}

// customRegionLevelChain orders the candidates of a requested indicator from
// finest to coarsest: the finer variant (finer_slug) first, then the
// indicator itself.
func customRegionLevelChain(requested customRegionMeta, bySlug map[string][]customRegionMeta) []customRegionMeta {
	chain := []customRegionMeta{}
	if requested.FinerSlug != nil {
		for _, m := range bySlug[*requested.FinerSlug] {
			if m.Indicator == *requested.FinerSlug {
				chain = append(chain, m)
			}
		}
	}
	return append(chain, requested)
}

// computeCustomRegionValue derives the final value of a row. Formulas as in
// mart_agentur_dataset_aggregate.sql: 'sum' distributes the unit value by area
// share; 'weighted_avg' weights by the population of the overlapping part if
// available, else by the area share of the region, else unweighted;
// 'weighted_avg_employees' (values per employed person) weights by the number
// of employed persons of the overlapping part, with the same fallbacks.
// Medians are only approximations (see value_kind).
func computeCustomRegionValue(measureType string, r customRegionAggRow) (value float64, method string, ok bool) {
	if measureType != "weighted_avg" && measureType != "weighted_avg_employees" {
		if !r.SumValue.Valid {
			return 0, "", false
		}
		return r.SumValue.Float64, "sum", true
	}
	switch {
	case measureType == "weighted_avg_employees" && r.EmpComplete && r.EmpDen.Valid && r.EmpDen.Float64 > 0 && r.EmpNum.Valid:
		return r.EmpNum.Float64 / r.EmpDen.Float64, "weighted_avg_employees", true
	case measureType == "weighted_avg" && r.PopComplete && r.PopDen.Valid && r.PopDen.Float64 > 0 && r.PopNum.Valid:
		return r.PopNum.Float64 / r.PopDen.Float64, "weighted_avg_population", true
	case r.AreaDen.Valid && r.AreaDen.Float64 > 0 && r.AreaNum.Valid:
		return r.AreaNum.Float64 / r.AreaDen.Float64, "weighted_avg_area", true
	case r.PlainAvg.Valid:
		return r.PlainAvg.Float64, "avg", true
	}
	return 0, "", false
}

func customRegionRows(m customRegionMeta, agg []customRegionAggRow, matchedUnit *string) []CustomRegionAggregateRow {
	out := make([]CustomRegionAggregateRow, 0, len(agg))
	for _, r := range agg {
		value, method, ok := computeCustomRegionValue(m.MeasureType, r)
		if !ok {
			continue
		}
		row := CustomRegionAggregateRow{
			DatasetSlug:       m.DatasetSlug,
			Indicator:         m.Indicator,
			LevelUsed:         m.Level,
			Year:              r.Year,
			Value:             value,
			ValueKind:         m.Kind,
			AggregationMethod: method,
			SourceRegionCount: r.SourceRegionCount,
			Coverage:          r.Coverage,
			MatchedUnit:       matchedUnit,
			IsCrosswalked:     r.IsCrosswalked,
		}
		if r.Month.Valid {
			month := int(r.Month.Int64)
			row.Month = &month
		}
		out = append(out, row)
	}
	return out
}

// ListCustomRegionDatasets lists the datasets that can be queried for custom regions
// @Summary Für Custom-Regionen abfragbare Datensätze
// @Description Gibt je Datensatz/Indikator die Quellebene (lau, nuts3, agentur), die Aggregationsart (sum, weighted_avg, weighted_avg_employees) und die Kennzeichnung der Näherung (kind) zurück. Namen und Lizenz: GET /datasets/{slug}.
// @Tags Geo
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]handlers.customRegionMeta}
// @Failure 500 {object} models.APIResponse
// @Router /custom-regions/datasets [get]
func (h *Handler) ListCustomRegionDatasets(c *gin.Context) {
	metas, err := h.loadCustomRegionMetas()
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	respondList(c, metas, &models.MetaInfo{Total: len(metas)})
}

func (h *Handler) loadCustomRegionMetas() ([]customRegionMeta, error) {
	metas := []customRegionMeta{}
	err := h.DB.Select(&metas, `
		SELECT dataset_slug, indicator, level, measure_type, kind, finer_slug
		FROM transformed_data.mart_dataset_aggregation_meta
		ORDER BY dataset_slug, indicator`)
	return metas, err
}

// QueryCustomRegionDatasetAggregate aggregates datasets over a freely defined region
// @Summary Datensätze für eine Custom-Region (GeoJSON)
// @Description Verschneidet das übergebene Polygon mit den Geometrien der Quelldaten und aggregiert die Werte. Je Datensatz wird die feinste verfügbare Ebene genutzt (LAU vor NUTS-3; Agenturbezirks-Daten liegen nur auf Agenturbezirks-Ebene vor). Zählgrößen (sum) werden flächenanteilig verteilt, Quoten/Durchschnitte (weighted_avg) bevorzugt bevölkerungsgewichtet gemittelt, Werte je Erwerbstätigen (weighted_avg_employees) erwerbstätigengewichtet. Näherungswerte (z. B. Mediane, Quoten) sind über value_kind und aggregation_method gekennzeichnet. Die Region wird nicht gespeichert. NUTS-3/LAU-Bezug: Version 2024.
// @Tags Geo
// @Accept json
// @Produce json
// @Param request body handlers.CustomRegionAggregateRequest true "GeoJSON-Region und Datensätze"
// @Success 200 {object} models.APIResponse{data=[]handlers.CustomRegionAggregateRow}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /custom-regions/dataset-aggregate [post]
func (h *Handler) QueryCustomRegionDatasetAggregate(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, customRegionMaxBodyBytes)

	var req CustomRegionAggregateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültiger Request-Body: %v", err))
		return
	}

	geometryJSON, datasets, minShare, err := normalizeCustomRegionRequest(&req)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	// Check the polygon once (valid, non-empty, size limit). Parse/transform
	// errors (e.g. coordinates outside WGS84) are user errors.
	var check struct {
		Points int  `db:"points"`
		Empty  bool `db:"is_empty"`
	}
	err = h.DB.Get(&check, `
		WITH poly AS (
			SELECT ST_CollectionExtract(
				ST_MakeValid(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326), 3035)), 3) AS g
		)
		SELECT ST_NPoints(g) AS points, ST_IsEmpty(g) AS is_empty FROM poly`, geometryJSON)
	if err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Geometrie (erwartet GeoJSON in WGS84/EPSG:4326, [lon, lat]): %v", err))
		return
	}
	if check.Empty {
		respondBadRequest(c, "Die Geometrie enthält keine Fläche")
		return
	}
	if check.Points > customRegionMaxPoints {
		respondBadRequest(c, fmt.Sprintf("Geometrie hat zu viele Stützpunkte (%d, maximal %d)", check.Points, customRegionMaxPoints))
		return
	}

	metas, err := h.loadCustomRegionMetas()
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	bySlug := map[string][]customRegionMeta{}
	for _, m := range metas {
		bySlug[m.DatasetSlug] = append(bySlug[m.DatasetSlug], m)
	}
	for _, slug := range datasets {
		if len(bySlug[slug]) == 0 {
			respondBadRequest(c, fmt.Sprintf("Unbekannter Datensatz %q für Custom-Region-Abfragen", slug))
			return
		}
	}

	// If the polygon exactly matches a unit of a level (e.g. a whole Kreis), only
	// that unit's values count, not edge slivers of its neighbours. Determined at
	// most once per level.
	matches := map[string]*string{}
	matchFor := func(level string) (*string, error) {
		if m, ok := matches[level]; ok {
			return m, nil
		}
		var ids []string
		err := h.DB.Select(&ids, fmt.Sprintf(customRegionExactMatchSQL, customRegionUnitsSQL[level]),
			geometryJSON, customRegionExactMatchIoU)
		if err != nil {
			return nil, err
		}
		var m *string
		if len(ids) > 0 {
			m = &ids[0]
		}
		matches[level] = m
		return m, nil
	}

	result := []CustomRegionAggregateRow{}
	for _, slug := range datasets {
		for _, requested := range bySlug[slug] {
			for _, candidate := range customRegionLevelChain(requested, bySlug) {
				matched, err := matchFor(candidate.Level)
				if err != nil {
					respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
					return
				}
				var agg []customRegionAggRow
				query := fmt.Sprintf(customRegionAggregateSQL, customRegionUnitsSQL[candidate.Level])
				err = h.DB.Select(&agg, query, geometryJSON, minShare, candidate.Level,
					candidate.DatasetSlug, candidate.Indicator, req.FromYear, req.ToYear, matched)
				if err != nil {
					respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
					return
				}
				rows := customRegionRows(candidate, agg, matched)
				if len(rows) > 0 {
					result = append(result, rows...)
					break
				}
			}
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.DatasetSlug != b.DatasetSlug {
			return a.DatasetSlug < b.DatasetSlug
		}
		if a.Indicator != b.Indicator {
			return a.Indicator < b.Indicator
		}
		if a.Year != b.Year {
			return a.Year < b.Year
		}
		return derefInt(a.Month) < derefInt(b.Month)
	})

	respondList(c, result, &models.MetaInfo{Total: len(result)})
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
