package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// supportedNutsVersions sind die NUTS-3-Zielversionen, für die
// mart_nuts3_sociodata_harmonized / mart_nuts3_timeseries_interpolated
// Zeilen führen (siehe target_versions dort). Muss manuell synchron
// gehalten werden - analog zu harmonizedRouteSlugs in cmd/gen-models/main.go.
var supportedNutsVersions = []int{2016, 2021, 2024}

// defaultNutsVersion ist die Zielversion, wenn nuts_version nicht gesetzt
// ist - entspricht dem bisherigen (Vor-Multi-Version-)Verhalten.
const defaultNutsVersion = 2024

func isSupportedNutsVersion(v int) bool {
	for _, sv := range supportedNutsVersions {
		if sv == v {
			return true
		}
	}
	return false
}

// sozioKreisBaseColumns ist die SELECT-Spaltenliste gemeinsam für alle Kreis-Datensätze.
// observation_date wird explizit als text gecastet, damit sqlx es in ein string-Feld scannen kann.
const sozioKreisBaseColumns = `d.original_id_1, d.original_name, d.geodata_name, d.geodata_id_nuts, ` +
	`coalesce(` +
	`nullif(to_jsonb(d)->>'geodata_id_lau', ''), ` +
	`case when d.geodata_level = 'LAU' then nullif(cast(d.geodata_id_ars as text), '') end, ` +
	`case when d.geodata_level = 'LAU' then cast(d.original_id_1 as text) end, ''` +
	`) as geodata_id_lau, ` +
	`cast(d.geodata_id_ars as text) as geodata_id_ars, d.mapper, d.parameter, d.geodata_level, d.geodata_year, ` +
	`d.year, d.observation_date::text, d.has_valid_value`

// getKreisData ist der generische Handler-Kern für alle Kreis-Datensätze.
// T muss SozioBaseData einbetten und alle cfg.ValueColumns via db-Tags mappen.
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
	if filter.FillGaps {
		table = cfg.InterpolatedTable
		extraColumns = ", d.is_interpolated, d.value_linear, d.value_peer_trend, d.n_peer_regions"
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
		// Wenn ein konkretes Datenjahr angefragt wird, wird die Kreisgrenze
		// verwendet, die in diesem Jahr gültig war (valid_from <= year <=
		// valid_to). Ohne Jahresfilter gilt die jeweils neueste Geometrie
		// (valid_to IS NULL). Damit ist sichergestellt, dass der Punkt-Filter
		// dieselbe Geometrie-Version wie die Soziodaten selbst verwendet
		// (vergleiche geodata_year in SozioBaseData).
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
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
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
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	query = pagedQuery
	args = pagedArgs

	data := make([]T, 0)
	if err := h.DB.Select(&data, query, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

// getKreisTimeseriesEndpoint liefert die typstabile Timeseries-Antwort für
// harmonisierte Kreis-Datensätze. Der dedizierte Endpoint vermeidet, dass
// dieselbe Route je nach Query-Parameter unterschiedliche Response-Schemata
// zurückliefert.
func getKreisTimeseriesEndpoint(h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	getKreisTimeseriesData(h, c, cfg, filter, limitNull)
}

// getKreisTimeseriesData liefert die lückenlose, interpolierte Zeitreihe aus
// transformed_data.mart_nuts3_timeseries_interpolated statt der rohen
// Kreis-Tabelle (fill_gaps=true). Fehlende Jahre werden dabei mit
// is_interpolated + value_linear + value_peer_trend transparent gekennzeichnet
// statt einfach zu fehlen. Unterstützt nur die Filter, die auf
// mart_nuts3_timeseries_interpolated sinnvoll anwendbar sind
// (geodata_id_nuts, nuts_parent_id, year, from_year, to_year); geodata_name-
// und point-Filter erfordern die Originaldaten und werden abgelehnt.
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

	nutsVersion := defaultNutsVersion
	if filter.NutsVersion != nil {
		if !isSupportedNutsVersion(*filter.NutsVersion) {
			respondBadRequest(c, fmt.Sprintf(
				"nuts_version %d wird nicht unterstützt (erlaubt: %v)",
				*filter.NutsVersion, supportedNutsVersions,
			))
			return
		}
		nutsVersion = *filter.NutsVersion
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	query := `SELECT nuts3_id, dataset_slug, target_version, year, value, is_interpolated, value_linear, value_peer_trend, n_peer_regions ` +
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
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
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
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	points := make([]models.NutsTimeseriesPoint, 0)
	if err := h.DB.Select(&points, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    points,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}
