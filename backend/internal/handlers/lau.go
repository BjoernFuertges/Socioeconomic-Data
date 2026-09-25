package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// GetLAURegions gibt alle LAU-Regionen zurück
// @Summary Liste aller LAU-Regionen
// @Description Gibt alle LAU-Regionen mit optionalen Filtern zurück
// @Tags LAU
// @Accept json
// @Produce json
// @Param country_code query string false "Filter nach Ländercode (z.B. DE)" example(DE)
// @Param nuts_parent_id query string false "Filter nach NUTS-Präfix (z.B. DE1 für alle Regionen unter NUTS-1)" example(DE1)
// @Param geodata_name_contains query string false "Teilstring-Suche im LAU-Namen" example(berlin)
// @Param point query string false "Punktabfrage im Format lon,lat (WGS84)" example(13.404954,52.520008)
// @Param year query int false "Filter nach Jahr" example(2024)
// @Param from_year query int false "Nur Jahre >= from_year zurückgeben" example(2020)
// @Param to_year query int false "Nur Jahre <= to_year zurückgeben" example(2024)
// @Param has_population query bool false "Filter nach Vorhandensein von Bevölkerungsdaten" example(true)
// @Param limit query int false "Anzahl der Ergebnisse (1-1000)" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.LAURegion}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau [get]
func (h *Handler) GetLAURegions(c *gin.Context) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.LAURegionFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	query := `SELECT gisco_id, year, lau_name, country_code, lau_id,
		geometry_wgs84, has_valid_geometry, lau_name_national, lau_name_latin,
		nuts_3_code, nuts3_id, nuts3_link_source, lau_nat_code, population_count, country, has_valid_population,
		missing_population_data, year_match
		FROM transformed_data.lau_integrated
		WHERE lau_name IS NOT NULL`

	args := []interface{}{}
	argPos := 1

	if filter.CountryCode != "" {
		query += fmt.Sprintf(" AND country_code = $%d", argPos)
		args = append(args, filter.CountryCode)
		argPos++
	}

	if filter.NutsParentID != "" {
		// nuts3_id ist die kanonische LAU->NUTS-3-Verlinkung (deklariert oder
		// räumlich ermittelt) - robuster als die oft NULLe nuts_3_code-Rohspalte.
		normalizedParent := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		query += fmt.Sprintf(" AND nuts3_id IS NOT NULL AND nuts3_id LIKE $%d", argPos)
		args = append(args, normalizedParent+"%")
		argPos++
	}

	if filter.GeodataNameContains != "" {
		query += fmt.Sprintf(" AND lau_name ILIKE $%d", argPos)
		args = append(args, "%"+strings.TrimSpace(filter.GeodataNameContains)+"%")
		argPos++
	}

	if strings.TrimSpace(filter.Point) != "" {
		lon, lat, err := parseLonLatPoint(filter.Point)
		if err != nil {
			respondBadRequest(c, err.Error())
			return
		}

		query += fmt.Sprintf(" AND has_valid_geometry AND ST_Contains(geometry_3035, ST_Transform(ST_SetSRID(ST_Point($%d, $%d), 4326), 3035))", argPos, argPos+1)
		args = append(args, lon, lat)
		argPos += 2
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

	if filter.HasPopulation != nil {
		if *filter.HasPopulation {
			query += " AND NOT missing_population_data"
		} else {
			query += " AND missing_population_data"
		}
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	err := h.DB.Get(&total, countQuery, args...)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"gisco_id, year",
		100,
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

	regions := []models.LAURegion{}
	err = h.DB.Select(&regions, query, args...)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    regions,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

// GetLAURegionByID gibt eine spezifische LAU-Region zurück
// @Summary LAU-Region Details
// @Description Gibt Details einer LAU-Region mit GeoJSON-Geometrie zurück
// @Tags LAU
// @Accept json
// @Produce json
// @Param gisco_id path string true "GISCO ID der LAU-Region"
// @Param year query int false "Jahr (optional, gibt neuestes Jahr zurück wenn nicht angegeben)"
// @Param include_datasets query bool false "Verfügbare Soziodata-Datensätze einbetten" example(false)
// @Success 200 {object} models.APIResponse{data=models.LAURegion}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau/{gisco_id} [get]
func (h *Handler) GetLAURegionByID(c *gin.Context) {
	giscoID := c.Param("gisco_id")
	yearStr := c.Query("year")
	includeDatasets := c.Query("include_datasets") == "true"

	query := `SELECT gisco_id, year, lau_name, country_code, lau_id,
		geometry_wgs84, has_valid_geometry, lau_name_national, lau_name_latin,
		nuts_3_code, nuts3_id, nuts3_link_source, lau_nat_code, population_count, country, has_valid_population,
		missing_population_data, year_match
		FROM transformed_data.lau_integrated
		WHERE gisco_id = $1 AND lau_name IS NOT NULL`

	args := []interface{}{giscoID}

	if yearStr != "" {
		query += " AND year = $2"
		args = append(args, yearStr)
	} else {
		query += " ORDER BY year DESC LIMIT 1"
	}

	var region models.LAURegion
	err := h.DB.Get(&region, query, args...)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			respondNotFound(c, "LAU-Region nicht gefunden")
			return
		}
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	// nuts3_id ist die kanonische Verlinkung (deklariert oder räumlich
	// ermittelt) - siehe Kommentar auf models.LAURegion.NUTS3ID.
	if includeDatasets && region.NUTS3ID.Valid {
		datasets, dsErr := h.regionNutsDatasets(region.NUTS3ID.String, 3)
		if dsErr == nil {
			region.Datasets = datasets
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    region,
	})
}

// GetLAURegionVersions gibt alle Geometrie-Perioden einer LAU-Region zurück
// @Summary Geometrie-Perioden einer LAU-Region
// @Description Gibt alle Geometrie-Perioden (valid_from/valid_to) einer LAU-Region aus der historisierten Geometrietabelle zurück (neueste zuerst). valid_to=null bedeutet noch aktuell.
// @Tags LAU
// @Accept json
// @Produce json
// @Param gisco_id path string true "GISCO ID der LAU-Region" example(DE_11000000)
// @Success 200 {object} models.APIResponse{data=models.GeoRegionVersions}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau/{gisco_id}/versions [get]
func (h *Handler) GetLAURegionVersions(c *gin.Context) {
	giscoID := strings.TrimSpace(c.Param("gisco_id"))
	if giscoID == "" {
		respondBadRequest(c, "gisco_id darf nicht leer sein")
		return
	}

	normalizedID := strings.ToUpper(giscoID)
	periods := make([]models.GeoPeriod, 0)
	query := `SELECT valid_from, valid_to
		FROM transformed_data.mart_lau_geometry_historized
		WHERE gisco_id = $1
		ORDER BY valid_from DESC`
	if err := h.DB.Select(&periods, query, normalizedID); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if len(periods) == 0 {
		respondNotFound(c, "Keine Versionsdaten für diese LAU-Region gefunden")
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: models.GeoRegionVersions{
			GeoID:   normalizedID,
			GeoType: "LAU",
			Periods: periods,
		},
	})
}
