package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// supportedLauVersions sind die LAU-Zielversionen, für die
// mart_lau_sociodata_harmonized / mart_lau_timeseries_interpolated Zeilen
// führen (siehe target_versions dort). Muss manuell synchron gehalten
// werden - analog zu supportedNutsVersions in kreis_generic.go und
// harmonizedLauRouteSlugs in cmd/gen-models/main.go.
var supportedLauVersions = []int{2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024}

// defaultLauVersion ist die Zielversion, wenn lau_version nicht gesetzt ist.
const defaultLauVersion = 2024

func isSupportedLauVersion(v int) bool {
	for _, sv := range supportedLauVersions {
		if sv == v {
			return true
		}
	}
	return false
}

// getLauTimeseriesEndpoint liefert die typstabile Timeseries-Antwort für
// harmonisierte LAU-Datensätze - das LAU-Pendant zu getKreisTimeseriesEndpoint.
func getLauTimeseriesEndpoint(h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	getLauTimeseriesData(h, c, cfg, filter, limitNull)
}

// getLauTimeseriesData liefert die lückenlose, interpolierte Zeitreihe aus
// transformed_data.mart_lau_timeseries_interpolated statt der rohen
// LAU-Tabelle (fill_gaps=true) - das LAU-Pendant zu getKreisTimeseriesData.
// Unterstützt nur die Filter, die auf mart_lau_timeseries_interpolated
// sinnvoll anwendbar sind (geodata_id_nuts als Kreis-Präfixfilter über
// nuts_parent_id, year, from_year, to_year); geodata_name_contains- und
// point-Filter erfordern die Originaldaten und werden abgelehnt.
func getLauTimeseriesData(h *Handler, c *gin.Context, cfg models.KreisDatasetConfig, filter models.SozioKreisFilter, limitNull bool) {
	if !cfg.IsHarmonizedLau {
		respondBadRequest(c, fmt.Sprintf(
			"fill_gaps wird für den Datensatz %q nicht unterstützt (nicht Teil von mart_lau_sociodata_harmonized)",
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
	if filter.GeodataIDLAU == "" && filter.NutsParentID == "" && filter.Year == nil && filter.FromYear == nil && filter.ToYear == nil {
		respondBadRequest(c, "ohne geodata_id_lau/nuts_parent_id muss year, from_year oder to_year gesetzt sein, um die Anfrage einzugrenzen")
		return
	}

	lauVersion := defaultLauVersion
	if filter.LauVersion != nil {
		if !isSupportedLauVersion(*filter.LauVersion) {
			respondBadRequest(c, fmt.Sprintf(
				"lau_version %d wird nicht unterstützt (erlaubt: %v)",
				*filter.LauVersion, supportedLauVersions,
			))
			return
		}
		lauVersion = *filter.LauVersion
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	query := `SELECT gisco_id, dataset_slug, target_version, year, value, is_interpolated, value_linear, value_peer_trend, n_peer_regions ` +
		`FROM transformed_data.mart_lau_timeseries_interpolated WHERE dataset_slug = $1 AND target_version = $2`
	args := []interface{}{cfg.RouteSlug, lauVersion}
	argPos := 3

	if filter.GeodataIDLAU != "" {
		query += fmt.Sprintf(" AND gisco_id = $%d", argPos)
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.GeodataIDLAU)))
		argPos++
	}

	if filter.NutsParentID != "" {
		// nuts_parent_id filtert hier über die kanonische LAU->NUTS-3-
		// Verlinkung aus lau_integrated (nuts3_id) statt über einen
		// gisco_id-Präfix - LAU-IDs haben keine hierarchische Struktur.
		normalizedParent := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		query += fmt.Sprintf(
			" AND gisco_id IN (SELECT gisco_id FROM transformed_data.lau_integrated WHERE nuts3_id IS NOT NULL AND nuts3_id LIKE $%d)",
			argPos,
		)
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
		"gisco_id, year",
		cfg.DefaultLimit,
		filter.Limit,
		filter.Offset,
		limitNull,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	points := make([]models.LauTimeseriesPoint, 0)
	if err := h.DB.Select(&points, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	respondList(c, points, &models.MetaInfo{
		Total:  total,
		Offset: offset,
	})
}
