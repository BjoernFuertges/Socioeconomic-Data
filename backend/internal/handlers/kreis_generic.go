package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// Supported nuts_version values and the default (newest) come from the
// harmonized marts at startup - see target_versions.go.

// sozioKreisBaseColumns is the SELECT column list shared by all Kreis datasets.
// observation_date is explicitly cast to text so sqlx can scan it into a string field.
const sozioKreisBaseColumns = `d.original_id_1, d.original_name, d.geodata_name, d.geodata_id_nuts, ` +
	`coalesce(` +
	`nullif(to_jsonb(d)->>'geodata_id_lau', ''), ` +
	`case when d.geodata_level = 'LAU' then nullif(cast(d.geodata_id_ars as text), '') end, ` +
	`case when d.geodata_level = 'LAU' then cast(d.original_id_1 as text) end, ''` +
	`) as geodata_id_lau, ` +
	`cast(d.geodata_id_ars as text) as geodata_id_ars, d.mapper, d.parameter, d.geodata_level, d.geodata_year, ` +
	`d.year, d.observation_date::text, d.has_valid_value`

// getKreisData is the generic handler core for all Kreis datasets.
// T must embed SozioBaseData and map all cfg.ValueColumns via db tags.
func getKreisData[T any](h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	if filter.FillGaps {
		if cfg.InterpolatedTable == "" {
			respondBadRequest(c, fmt.Sprintf(
				"fill_gaps wird für den Datensatz %q nicht unterstützt",
				cfg.RouteSlug,
			))
			return
		}
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	schema := cfg.Schema
	table := cfg.Table
	extraColumns := ""
	// nutsVersion/lauVersion != nil only with fill_gaps=true - the filter below
	// then applies to the api_*_fill_gaps view, which (like
	// mart_nuts3_timeseries_interpolated / mart_lau_timeseries_interpolated)
	// carries several target versions in parallel. Without this filter,
	// fill_gaps=true would silently mix rows from all versions and ignore the
	// nuts_version/lau_version parameter of the /timeseries endpoint. A dataset
	// is always either IsHarmonized (NUTS) or IsHarmonizedLau, never both - see
	// KreisDatasetConfig.
	var nutsVersion *int
	var lauVersion *int
	if filter.FillGaps {
		table = cfg.InterpolatedTable
		if cfg.IsHarmonizedLau {
			extraColumns = ", d.is_interpolated, d.value_linear, d.value_peer_trend, d.n_peer_regions, d.lau_version"
			v := defaultLauVersion()
			if filter.LauVersion != nil {
				if !isSupportedLauVersion(*filter.LauVersion) {
					respondBadRequest(c, fmt.Sprintf(
						"lau_version %d wird nicht unterstützt (erlaubt: %v)",
						*filter.LauVersion, supportedLauVersions(),
					))
					return
				}
				v = *filter.LauVersion
			}
			lauVersion = &v
		} else {
			extraColumns = ", d.is_interpolated, d.is_estimated, d.value_linear, d.value_peer_trend, d.n_peer_regions, d.nuts_version"
			v := defaultNutsVersion()
			if filter.NutsVersion != nil {
				if !isSupportedNutsVersion(*filter.NutsVersion) {
					respondBadRequest(c, fmt.Sprintf(
						"nuts_version %d wird nicht unterstützt (erlaubt: %v)",
						*filter.NutsVersion, supportedNutsVersions(),
					))
					return
				}
				v = *filter.NutsVersion
			}
			nutsVersion = &v
		}
	}

	query := fmt.Sprintf(
		`SELECT %s, %s%s FROM %s.%s d WHERE 1=1`,
		sozioKreisBaseColumns,
		strings.Join(cfg.ValueColumns, ", "),
		extraColumns,
		schema,
		table,
	)

	args := []interface{}{}
	argPos := 1

	if nutsVersion != nil {
		query += fmt.Sprintf(" AND d.nuts_version = $%d", argPos)
		args = append(args, *nutsVersion)
		argPos++
	}

	if lauVersion != nil {
		query += fmt.Sprintf(" AND d.lau_version = $%d", argPos)
		args = append(args, *lauVersion)
		argPos++
	}

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND d.geodata_id_nuts = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.NutsParentID != "" {
		normalizedParent := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		query += fmt.Sprintf(" AND d.geodata_id_nuts LIKE $%d", argPos)
		args = append(args, normalizedParent+"%")
		argPos++
	}

	if filter.GeodataNameContains != "" {
		pattern := "%" + strings.TrimSpace(filter.GeodataNameContains) + "%"
		query += fmt.Sprintf(" AND (d.geodata_name ILIKE $%d OR d.original_name ILIKE $%d)", argPos, argPos)
		args = append(args, pattern)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND d.year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND d.year >= $%d", argPos)
		args = append(args, *filter.FromYear)
		argPos++
	}

	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND d.year <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
	}

	if strings.TrimSpace(filter.Point) != "" {
		lon, lat, err := parseLonLatPoint(filter.Point)
		if err != nil {
			respondBadRequest(c, err.Error())
			return
		}
		// If a concrete data year is requested, the Kreis boundary that was valid in
		// that year is used (valid_from <= year <= valid_to). Without a year filter
		// the latest geometry applies (valid_to IS NULL). This ensures the point
		// filter uses the same geometry version as the socio data itself (compare
		// geodata_year in SozioBaseData).
		if filter.Year != nil {
			query += fmt.Sprintf(
				` AND d.geodata_id_nuts = (`+
					`SELECT nuts_id FROM transformed_data.mart_nuts_geometry_historized `+
					`WHERE levl_code = 3 AND has_valid_geometry `+
					`AND valid_from <= $%d AND (valid_to IS NULL OR valid_to >= $%d) `+
					`AND ST_Contains(geometry_3035, ST_Transform(ST_SetSRID(ST_Point($%d, $%d), 4326), 3035)) `+
					`ORDER BY valid_from DESC LIMIT 1)`,
				argPos, argPos, argPos+1, argPos+2,
			)
			args = append(args, *filter.Year, lon, lat)
			argPos += 3
		} else {
			query += fmt.Sprintf(
				` AND d.geodata_id_nuts = (`+
					`SELECT nuts_id FROM transformed_data.mart_nuts_geometry_historized `+
					`WHERE levl_code = 3 AND has_valid_geometry AND valid_to IS NULL `+
					`AND ST_Contains(geometry_3035, ST_Transform(ST_SetSRID(ST_Point($%d, $%d), 4326), 3035)) `+
					`LIMIT 1)`,
				argPos, argPos+1,
			)
			args = append(args, lon, lat)
			argPos += 2
		}
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		respondDBError(c, "Fehler beim Zählen der Datensätze", err)
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		cfg.OrderBy,
		cfg.DefaultLimit,
		filter.Limit,
		filter.Offset,
		limitNull,
		total,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	query = pagedQuery
	args = pagedArgs

	data := make([]T, 0)
	if err := h.DB.SelectContext(c.Request.Context(), &data, query, args...); err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	respondList(c, data, &models.MetaInfo{
		Total:  total,
		Offset: offset,
	})
}

// getKreisTimeseriesEndpoint returns the type-stable time-series response for
// harmonized Kreis datasets. The dedicated endpoint avoids the same route
// returning different response schemas depending on query parameters.
func getKreisTimeseriesEndpoint(h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	getKreisTimeseriesData(h, c, cfg, filter, limitNull)
}

// getKreisTimeseriesData returns the gap-free, interpolated time series from
// transformed_data.mart_nuts3_timeseries_interpolated instead of the raw
// Kreis table (fill_gaps=true). Missing years are transparently flagged with
// is_interpolated + value_linear + value_peer_trend instead of simply being
// absent. Supports only the filters that are meaningful on
// mart_nuts3_timeseries_interpolated (geodata_id_nuts, nuts_parent_id, year,
// from_year, to_year); geodata_name and point filters need the original data
// and are rejected.
func getKreisTimeseriesData(h *Handler, c *gin.Context, cfg models.KreisDatasetConfig, filter models.SozioKreisFilter, limitNull bool) {
	if !cfg.IsHarmonized {
		respondBadRequest(c, fmt.Sprintf(
			"fill_gaps wird für den Datensatz %q nicht unterstützt (nicht Teil von mart_nuts3_sociodata_harmonized)",
			cfg.RouteSlug,
		))
		return
	}
	if strings.TrimSpace(filter.GeodataNameContains) != "" {
		respondBadRequest(c, "geodata_name_contains wird in Kombination mit fill_gaps nicht unterstützt")
		return
	}
	if strings.TrimSpace(filter.Point) != "" {
		respondBadRequest(c, "point wird in Kombination mit fill_gaps nicht unterstützt")
		return
	}
	if filter.GeodataIDNUTS == "" && filter.Year == nil && filter.FromYear == nil && filter.ToYear == nil {
		respondBadRequest(c, "ohne geodata_id_nuts muss year, from_year oder to_year gesetzt sein, um die Anfrage einzugrenzen")
		return
	}

	nutsVersion := defaultNutsVersion()
	if filter.NutsVersion != nil {
		if !isSupportedNutsVersion(*filter.NutsVersion) {
			respondBadRequest(c, fmt.Sprintf(
				"nuts_version %d wird nicht unterstützt (erlaubt: %v)",
				*filter.NutsVersion, supportedNutsVersions(),
			))
			return
		}
		nutsVersion = *filter.NutsVersion
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	query := `SELECT nuts3_id, dataset_slug, target_version, year, value, is_interpolated, is_estimated, value_linear, value_peer_trend, n_peer_regions ` +
		`FROM transformed_data.mart_nuts3_timeseries_interpolated WHERE dataset_slug = $1 AND target_version = $2`
	args := []interface{}{cfg.RouteSlug, nutsVersion}
	argPos := 3

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND nuts3_id = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.NutsParentID != "" {
		normalizedParent := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		query += fmt.Sprintf(" AND nuts3_id LIKE $%d", argPos)
		args = append(args, normalizedParent+"%")
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND year >= $%d", argPos)
		args = append(args, *filter.FromYear)
		argPos++
	}

	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND year <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		respondDBError(c, "Fehler beim Zählen der Datensätze", err)
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"nuts3_id, year",
		cfg.DefaultLimit,
		filter.Limit,
		filter.Offset,
		limitNull,
		total,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	points := make([]models.NutsTimeseriesPoint, 0)
	if err := h.DB.SelectContext(c.Request.Context(), &points, pagedQuery, pagedArgs...); err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	respondList(c, points, &models.MetaInfo{
		Total:  total,
		Offset: offset,
	})
}
