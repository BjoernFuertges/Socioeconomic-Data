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
		nuts_3_code, lau_nat_code, population_count, country, has_valid_population,
		missing_population_data, year_match
		FROM transformed_data.lau_integrated
		WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if filter.CountryCode != "" {
		query += fmt.Sprintf(" AND country_code = $%d", argPos)
		args = append(args, filter.CountryCode)
		argPos++
	}

	if filter.NutsParentID != "" {
		normalizedParent := strings.ToUpper(strings.TrimSpace(filter.NutsParentID))
		query += fmt.Sprintf(" AND nuts_3_code IS NOT NULL AND nuts_3_code LIKE $%d", argPos)
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
// @Success 200 {object} models.APIResponse{data=models.LAURegion}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau/{gisco_id} [get]
func (h *Handler) GetLAURegionByID(c *gin.Context) {
	giscoID := c.Param("gisco_id")
	yearStr := c.Query("year")

	query := `SELECT gisco_id, year, lau_name, country_code, lau_id, 
		geometry_wgs84, has_valid_geometry, lau_name_national, lau_name_latin,
		nuts_3_code, lau_nat_code, population_count, country, has_valid_population,
		missing_population_data, year_match
		FROM transformed_data.lau_integrated
		WHERE gisco_id = $1`

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

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    region,
	})
}
