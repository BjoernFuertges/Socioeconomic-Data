package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// Slugs of the raw Agentur values (mart_agenturbezirk_sociodata) and of the
// LAU->NUTS-3 aggregates (mart_nuts3_lau_aggregate) in the combined export.
// They are not Kreis datasets; "ba_" separates the BA raw values from the
// same-named Kreis dataset "arbeitslose" aggregated to Agenturbezirke.
const (
	slugBAArbeitslose         = "ba_arbeitslose"
	slugBAUnterbeschaeftigung = "ba_unterbeschaeftigung"
	slugLauPopulationSum      = "lau_population_sum"
	slugLauBeschQuoteAvg      = "lau_beschaeftigungsquote_weighted_avg"
)

type regionDataFilter struct {
	Datasets string `form:"datasets"`
	Scope    string `form:"scope"`
	FromYear *int   `form:"from_year"`
	ToYear   *int   `form:"to_year"`
}

// regionScope describes which kinds of values are delivered.
type regionScope struct {
	Raw        bool
	Aggregated bool
}

func parseRegionScope(raw string) (regionScope, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "both" {
		return regionScope{Raw: true, Aggregated: true}, nil
	}

	var scope regionScope
	for _, part := range strings.Split(raw, ",") {
		switch strings.TrimSpace(part) {
		case "raw":
			scope.Raw = true
		case "aggregated":
			scope.Aggregated = true
		case "both":
			scope.Raw, scope.Aggregated = true, true
		default:
			return regionScope{}, fmt.Errorf("scope muss raw, aggregated oder both sein (kommagetrennt), erhalten %q", part)
		}
	}
	return scope, nil
}

// parseDatasetList splits ?datasets=a,b; empty = all available.
func parseDatasetList(raw string) []string {
	var slugs []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		slug := strings.ToLower(strings.TrimSpace(part))
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		slugs = append(slugs, slug)
	}
	return slugs
}

// validateRegionDatasets rejects slugs that are neither a known Kreis
// dataset nor one of the region-specific special slugs.
func validateRegionDatasets(slugs []string) error {
	special := map[string]bool{
		slugBAArbeitslose: true, slugBAUnterbeschaeftigung: true,
		slugLauPopulationSum: true, slugLauBeschQuoteAvg: true,
	}
	for _, slug := range slugs {
		if special[slug] {
			continue
		}
		if _, ok := models.DatasetConfigBySlug(slug); !ok {
			return fmt.Errorf("unbekannter Datensatz-Slug %q", slug)
		}
	}
	return nil
}

// pickSlugs returns the applicable slugs from slugs (filter: allowed). Without
// a selection (slugs empty) => nil and all=true, i.e. "all available".
func pickSlugs(slugs []string, allowed func(string) bool) (picked []string, all bool) {
	if len(slugs) == 0 {
		return nil, true
	}
	for _, slug := range slugs {
		if allowed(slug) {
			picked = append(picked, slug)
		}
	}
	return picked, false
}

func isNutsHarmonizedSlug(slug string) bool {
	cfg, ok := models.DatasetConfigBySlug(slug)
	return ok && cfg.IsHarmonized
}

func isLauHarmonizedSlug(slug string) bool {
	cfg, ok := models.DatasetConfigBySlug(slug)
	return ok && cfg.IsHarmonizedLau
}

// addYearRange appends from_year/to_year conditions and returns the new
// query and the next argument position.
func addYearRange(query string, args []interface{}, yearCol string, filter regionDataFilter) (string, []interface{}) {
	if filter.FromYear != nil {
		args = append(args, *filter.FromYear)
		query += fmt.Sprintf(" AND %s >= $%d", yearCol, len(args))
	}
	if filter.ToYear != nil {
		args = append(args, *filter.ToYear)
		query += fmt.Sprintf(" AND %s <= $%d", yearCol, len(args))
	}
	return query, args
}

// GetRegionData returns raw and/or aggregated values of a region as a
// long-format table
// @Summary Kombinierter Daten-Export einer Region (roh und/oder aggregiert)
// @Description Stellt die gewünschten Datensätze einer Region (NUTS, LAU oder Agenturbezirk) aus den bestehenden Marts zusammen und liefert sie in einem einheitlichen Long-Format (region_type, region_id, dataset_slug, year, month, value, is_computed, is_interpolated, aggregation_method, value_kind, source_region_count). scope=raw liefert die Ursprungswerte der Region (NUTS-3: interpolierte Kreis-Zeitreihe; LAU: interpolierte Gemeinde-Zeitreihe; Agentur: monatliche BA-Werte ba_arbeitslose/ba_unterbeschaeftigung), scope=aggregated die berechneten Werte (NUTS-0/1/2 und Agenturbezirke: aus Kreisen aggregiert; NUTS-3: aus Gemeinden aggregiert). NUTS-0/1/2 haben keine Rohwerte, LAU keine aggregierten Werte. Die NUTS-3-/LAU-Rohwerte stehen immer im neuesten verfügbaren NUTS-/LAU-Stand (default von GET /versions). Format per Accept-Header (application/json oder text/csv) oder ?format=csv|json; ?download=1 setzt Content-Disposition.
// @Tags Regionen
// @Accept json
// @Produce json,text/csv
// @Param region_type path string true "Regionstyp" Enums(nuts, lau, agentur)
// @Param region_id path string true "nuts_id, gisco_id bzw. ba_schl" example(DE300)
// @Param datasets query string false "Kommagetrennte Datensatz-Slugs (leer = alle verfügbaren)" example(bevoelkerung,arbeitslose)
// @Param scope query string false "raw, aggregated oder both (kommagetrennt)" Enums(raw, aggregated, both) default(both)
// @Param from_year query int false "Frühestes Jahr"
// @Param to_year query int false "Spätestes Jahr"
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen"
// @Success 200 {object} models.APIResponse{data=[]models.RegionDataRow}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /regions/{region_type}/{region_id}/data [get]
func (h *Handler) GetRegionData(c *gin.Context) {
	regionType := strings.ToLower(strings.TrimSpace(c.Param("region_type")))
	regionID := strings.TrimSpace(c.Param("region_id"))
	if regionID == "" {
		respondBadRequest(c, "region_id darf nicht leer sein")
		return
	}

	var filter regionDataFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}
	scope, err := parseRegionScope(filter.Scope)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	slugs := parseDatasetList(filter.Datasets)
	if err := validateRegionDatasets(slugs); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	rows := []models.RegionDataRow{}
	switch regionType {
	case "nuts":
		rows, err = h.regionDataNuts(c.Request.Context(), regionID, scope, slugs, filter)
	case "lau":
		rows, err = h.regionDataLau(c.Request.Context(), regionID, scope, slugs, filter)
	case "agentur":
		rows, err = h.regionDataAgentur(c.Request.Context(), regionID, scope, slugs, filter)
	default:
		respondBadRequest(c, "region_type muss nuts, lau oder agentur sein")
		return
	}
	if err != nil {
		switch e := err.(type) {
		case regionInputError:
			respondBadRequest(c, e.Error())
		case regionNotFoundError:
			respondNotFound(c, e.Error())
		default:
			respondDBError(c, "Datenbankfehler", err)
		}
		return
	}

	if rows == nil {
		rows = []models.RegionDataRow{}
	}
	respondList(c, rows, &models.MetaInfo{Total: len(rows)})
}

type regionInputError struct{ msg string }

func (e regionInputError) Error() string { return e.msg }

type regionNotFoundError struct{ msg string }

func (e regionNotFoundError) Error() string { return e.msg }

func (h *Handler) regionDataNuts(ctx context.Context, regionID string, scope regionScope, slugs []string, filter regionDataFilter) ([]models.RegionDataRow, error) {
	details, err := nutsLevelDetailsFromID(regionID)
	if err != nil {
		return nil, regionInputError{err.Error()}
	}
	id := details.NormalizedID
	rows := []models.RegionDataRow{}

	if details.Number == 3 {
		if scope.Raw {
			picked, all := pickSlugs(slugs, isNutsHarmonizedSlug)
			if all || len(picked) > 0 {
				query := `SELECT 'nuts' AS region_type, nuts3_id AS region_id, dataset_slug, year, NULL::int AS month, value,
					false AS is_computed, is_interpolated, NULL::text AS aggregation_method, NULL::text AS value_kind, NULL::int AS source_region_count
					FROM transformed_data.mart_nuts3_timeseries_interpolated
					WHERE nuts3_id = $1 AND target_version = $2`
				args := []interface{}{id, defaultNutsVersion()}
				if !all {
					args = append(args, pq.Array(picked))
					query += fmt.Sprintf(" AND dataset_slug = ANY($%d)", len(args))
				}
				query, args = addYearRange(query, args, "year", filter)
				query += " ORDER BY dataset_slug, year"

				var part []models.RegionDataRow
				if err := h.DB.SelectContext(ctx, &part, query, args...); err != nil {
					return nil, err
				}
				rows = append(rows, part...)
			}
		}
		if scope.Aggregated {
			part, err := h.lauAggregateRows(ctx, id, slugs, filter)
			if err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
		return rows, nil
	}

	// NUTS-0/1/2: aggregated values only (from Kreise), no raw values.
	if !scope.Aggregated {
		return rows, nil
	}
	picked, all := pickSlugs(slugs, isNutsHarmonizedSlug)
	if !all && len(picked) == 0 {
		return rows, nil
	}
	query := `SELECT 'nuts' AS region_type, nuts_id AS region_id, dataset_slug, year, NULL::int AS month, value,
		true AS is_computed, false AS is_interpolated, NULL::text AS aggregation_method, NULL::text AS value_kind, source_region_count, expected_region_count, is_complete
		FROM transformed_data.mart_nuts012_dataset_aggregate
		WHERE nuts_id = $1`
	args := []interface{}{id}
	if !all {
		args = append(args, pq.Array(picked))
		query += fmt.Sprintf(" AND dataset_slug = ANY($%d)", len(args))
	}
	query, args = addYearRange(query, args, "year", filter)
	query += " ORDER BY dataset_slug, year"
	if err := h.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// lauAggregateRows converts the two columns of mart_nuts3_lau_aggregate into
// long format (one row per indicator per year).
func (h *Handler) lauAggregateRows(ctx context.Context, nuts3ID string, slugs []string, filter regionDataFilter) ([]models.RegionDataRow, error) {
	wantPop, wantQuote := len(slugs) == 0, len(slugs) == 0
	for _, slug := range slugs {
		switch slug {
		case slugLauPopulationSum:
			wantPop = true
		case slugLauBeschQuoteAvg:
			wantQuote = true
		}
	}
	if !wantPop && !wantQuote {
		return nil, nil
	}

	query := `SELECT year, lau_population_sum, lau_region_count, lau_region_count_with_population, lau_beschaeftigungsquote_weighted_avg, lau_beschaeftigungsquote_region_count
		FROM transformed_data.mart_nuts3_lau_aggregate WHERE nuts3_id = $1`
	args := []interface{}{nuts3ID}
	query, args = addYearRange(query, args, "year", filter)
	query += " ORDER BY year"

	var series []models.NutsLauAggregatePoint
	if err := h.DB.SelectContext(ctx, &series, query, args...); err != nil {
		return nil, err
	}

	rows := make([]models.RegionDataRow, 0, len(series)*2)
	newRow := func(slug string, p models.NutsLauAggregatePoint, value *float64, method string, count int) models.RegionDataRow {
		return models.RegionDataRow{
			RegionType: "nuts", RegionID: nuts3ID, DatasetSlug: slug, Year: p.Year, Value: value,
			IsComputed: true, AggregationMethod: &method, SourceRegionCount: &count,
		}
	}
	for _, p := range series {
		if wantPop {
			rows = append(rows, newRow(slugLauPopulationSum, p, p.LauPopulationSum, "sum", p.LauRegionCountWithPopulation))
		}
		if wantQuote {
			rows = append(rows, newRow(slugLauBeschQuoteAvg, p, p.LauBeschaeftigungsquoteWeightedAvg, "weighted_avg_population", p.LauBeschaeftigungsquoteRegionCount))
		}
	}
	return rows, nil
}

func (h *Handler) regionDataLau(ctx context.Context, regionID string, scope regionScope, slugs []string, filter regionDataFilter) ([]models.RegionDataRow, error) {
	rows := []models.RegionDataRow{}
	// LAU has no aggregated values - raw values only.
	if !scope.Raw {
		return rows, nil
	}
	id := strings.ToUpper(regionID)
	picked, all := pickSlugs(slugs, isLauHarmonizedSlug)
	if !all && len(picked) == 0 {
		return rows, nil
	}

	query := `SELECT 'lau' AS region_type, gisco_id AS region_id, dataset_slug, year, NULL::int AS month, value,
		false AS is_computed, is_interpolated, NULL::text AS aggregation_method, NULL::text AS value_kind, NULL::int AS source_region_count
		FROM transformed_data.mart_lau_timeseries_interpolated
		WHERE gisco_id = $1 AND target_version = $2`
	args := []interface{}{id, defaultLauVersion()}
	if !all {
		args = append(args, pq.Array(picked))
		query += fmt.Sprintf(" AND dataset_slug = ANY($%d)", len(args))
	}
	query, args = addYearRange(query, args, "year", filter)
	query += " ORDER BY dataset_slug, year"
	if err := h.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *Handler) regionDataAgentur(ctx context.Context, baSchl string, scope regionScope, slugs []string, filter regionDataFilter) ([]models.RegionDataRow, error) {
	var exists bool
	if err := h.DB.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM transformed_data.mart_agenturbezirk_sociodata WHERE ba_schl = $1)`, baSchl); err != nil {
		return nil, err
	}
	if !exists {
		return nil, regionNotFoundError{"Agenturbezirk nicht gefunden"}
	}

	rows := []models.RegionDataRow{}

	if scope.Raw {
		wantAL, wantUB := len(slugs) == 0, len(slugs) == 0
		for _, slug := range slugs {
			switch slug {
			case slugBAArbeitslose:
				wantAL = true
			case slugBAUnterbeschaeftigung:
				wantUB = true
			}
		}
		if wantAL || wantUB {
			query := `SELECT year, month, arbeitslose, unterbeschaeftigung
				FROM transformed_data.mart_agenturbezirk_sociodata
				WHERE ba_schl = $1 AND year IS NOT NULL`
			args := []interface{}{baSchl}
			query, args = addYearRange(query, args, "year", filter)
			query += " ORDER BY observation_date"

			var points []models.AgenturbezirkArbeitslosigkeitPoint
			if err := h.DB.SelectContext(ctx, &points, query, args...); err != nil {
				return nil, err
			}
			for _, p := range points {
				month := p.Month
				add := func(slug string, v sql.NullFloat64) {
					row := models.RegionDataRow{RegionType: "agentur", RegionID: baSchl, DatasetSlug: slug, Year: p.Year, Month: &month}
					if v.Valid {
						value := v.Float64
						row.Value = &value
					}
					rows = append(rows, row)
				}
				if wantAL {
					add(slugBAArbeitslose, p.Arbeitslose)
				}
				if wantUB {
					add(slugBAUnterbeschaeftigung, p.Unterbeschaeftigung)
				}
			}
		}
	}

	if scope.Aggregated {
		picked, all := pickSlugs(slugs, isNutsHarmonizedSlug)
		if all || len(picked) > 0 {
			query := `SELECT 'agentur' AS region_type, ba_schl AS region_id, dataset_slug, year, NULL::int AS month, value,
				true AS is_computed, false AS is_interpolated, aggregation_method, value_kind, source_region_count, expected_region_count, is_complete
				FROM transformed_data.mart_agentur_dataset_aggregate
				WHERE ba_schl = $1`
			args := []interface{}{baSchl}
			if !all {
				args = append(args, pq.Array(picked))
				query += fmt.Sprintf(" AND dataset_slug = ANY($%d)", len(args))
			}
			query, args = addYearRange(query, args, "year", filter)
			query += " ORDER BY dataset_slug, year"

			var part []models.RegionDataRow
			if err := h.DB.SelectContext(ctx, &part, query, args...); err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
	}
	return rows, nil
}
