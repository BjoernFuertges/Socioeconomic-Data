package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// Handler enthält alle Handler für die API
type Handler struct {
	DB *sqlx.DB
}

// NewHandler erstellt einen neuen Handler
func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{
		DB: db,
	}
}

// HealthCheck gibt den Status der API zurück
// @Summary Health Check
// @Description Gibt den Status der API zurück
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "soziooekonomische-daten-api",
	})
}

// GetLAURegions gibt alle LAU-Regionen zurück
// @Summary Liste aller LAU-Regionen
// @Description Gibt alle LAU-Regionen mit optionalen Filtern zurück
// @Tags LAU
// @Accept json
// @Produce json
// @Param country_code query string false "Filter nach Ländercode (z.B. DE)" example(DE)
// @Param year query int false "Filter nach Jahr" example(2024)
// @Param has_population query bool false "Filter nach Vorhandensein von Bevölkerungsdaten" example(true)
// @Param limit query int false "Anzahl der Ergebnisse (1-1000)" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.LAURegion}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau [get]
func (h *Handler) GetLAURegions(c *gin.Context) {
	var filter models.LAURegionFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Ungültige Query-Parameter: %v", err),
		})
		return
	}

	// Default values
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Build query
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

	// Count total
	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	err := h.DB.Get(&total, countQuery, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err),
		})
		return
	}

	// Add pagination
	query += fmt.Sprintf(" ORDER BY gisco_id, year LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	// Query mit automatischem Struct-Mapping
	regions := []models.LAURegion{}
	err = h.DB.Select(&regions, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    regions,
		Meta: &models.MetaInfo{
			Total:  total,
			Limit:  filter.Limit,
			Offset: filter.Offset,
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

	var r models.LAURegion
	err := h.DB.Get(&r, query, args...)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "LAU-Region nicht gefunden",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    r,
	})
}

// GetUnemploymentData gibt Arbeitslosenzahlen zurück
// @Summary Arbeitslosenzahlen abrufen
// @Description Gibt Arbeitslosenzahlen mit optionalen Filtern zurück
// @Tags Unemployment
// @Accept json
// @Produce json
// @Param geodata_id query string false "Filter nach Geodaten-ID" example(DE111)
// @Param from_date query string false "Start-Datum (YYYY-MM-DD)" example(2024-01-01)
// @Param to_date query string false "End-Datum (YYYY-MM-DD)" example(2024-12-31)
// @Param time_type query string false "Zeittyp: yearly, monthly" example(monthly)
// @Param year query int false "Filter nach Jahr" example(2024)
// @Param quarter query int false "Filter nach Quartal (1-4)" example(4)
// @Param limit query int false "Anzahl der Ergebnisse (1-10000)" default(1000) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.UnemploymentData}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /unemployment [get]
func (h *Handler) GetUnemploymentData(c *gin.Context) {
	var filter models.UnemploymentFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Ungültige Query-Parameter: %v", err),
		})
		return
	}

	// Default values
	if filter.Limit == 0 {
		filter.Limit = 1000
	}

	// Build query
	query := `SELECT dataset_id, geodata_id, geodata_name, year, month, quarter,
		observation_date, time_type, arbeitslose, load_date, dataset_title,
		dataset_series, source_url, geodata_level, arbeitslose_previous_year,
		yoy_change_percent, data_quality_flag
		FROM transformed_data.mart_arbeitslosenzahlen
		WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if filter.GeodataID != "" {
		query += fmt.Sprintf(" AND geodata_id = $%d", argPos)
		args = append(args, filter.GeodataID)
		argPos++
	}

	if filter.FromDate != "" {
		query += fmt.Sprintf(" AND observation_date >= $%d", argPos)
		args = append(args, filter.FromDate)
		argPos++
	}

	if filter.ToDate != "" {
		query += fmt.Sprintf(" AND observation_date <= $%d", argPos)
		args = append(args, filter.ToDate)
		argPos++
	}

	if filter.TimeType != "" {
		query += fmt.Sprintf(" AND time_type = $%d", argPos)
		args = append(args, filter.TimeType)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	err := h.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err),
		})
		return
	}

	// Add pagination
	query += fmt.Sprintf(" ORDER BY observation_date DESC, geodata_id LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	// Query mit automatischem Struct-Mapping
	data := []models.UnemploymentData{}
	err = h.DB.Select(&data, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		},
	})
}

// GetUnemploymentByGeodataID gibt Zeitreihe für ein Gebiet zurück
// @Summary Arbeitslosenzahlen für ein Gebiet
// @Description Gibt alle Arbeitslosenzahlen für eine spezifische Geodaten-ID zurück
// @Tags Unemployment
// @Accept json
// @Produce json
// @Param geodata_id path string true "Geodaten-ID" example(DE111)
// @Param from_date query string false "Start-Datum (YYYY-MM-DD)" example(2024-01-01)
// @Param to_date query string false "End-Datum (YYYY-MM-DD)" example(2024-12-31)
// @Param time_type query string false "Zeittyp: yearly, monthly" example(monthly)
// @Param limit query int false "Anzahl der Ergebnisse" default(100) example(12)
// @Success 200 {object} models.APIResponse{data=[]models.UnemploymentData}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /unemployment/geodata/{geodata_id} [get]
func (h *Handler) GetUnemploymentByGeodataID(c *gin.Context) {
	geodataID := c.Param("geodata_id")

	query := `SELECT dataset_id, geodata_id, geodata_name, year, month, quarter,
		observation_date, time_type, arbeitslose, load_date, dataset_title,
		dataset_series, source_url, geodata_level, arbeitslose_previous_year,
		yoy_change_percent, data_quality_flag
		FROM transformed_data.mart_arbeitslosenzahlen
		WHERE geodata_id = $1`

	args := []interface{}{geodataID}
	argPos := 2

	// Optional filters
	if fromDate := c.Query("from_date"); fromDate != "" {
		query += fmt.Sprintf(" AND observation_date >= $%d", argPos)
		args = append(args, fromDate)
		argPos++
	}

	if toDate := c.Query("to_date"); toDate != "" {
		query += fmt.Sprintf(" AND observation_date <= $%d", argPos)
		args = append(args, toDate)
		argPos++
	}

	if timeType := c.Query("time_type"); timeType != "" {
		query += fmt.Sprintf(" AND time_type = $%d", argPos)
		args = append(args, timeType)
		argPos++
	}

	query += " ORDER BY observation_date ASC"

	// Query mit automatischem Struct-Mapping
	data := []models.UnemploymentData{}
	err := h.DB.Select(&data, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	if len(data) == 0 {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   "Keine Daten für diese Geodaten-ID gefunden",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
	})
}

// GetUnemploymentAggregated gibt aggregierte Statistiken zurück
// @Summary Aggregierte Arbeitslosenstatistiken
// @Description Gibt aggregierte Statistiken pro Gebiet zurück
// @Tags Unemployment
// @Accept json
// @Produce json
// @Param geodata_id query string false "Filter nach Geodaten-ID" example(DE111)
// @Param year query int false "Filter nach Jahr" example(2024)
// @Param from_date query string false "Start-Datum (YYYY-MM-DD)" example(2024-01-01)
// @Param to_date query string false "End-Datum (YYYY-MM-DD)" example(2024-12-31)
// @Param limit query int false "Anzahl der Ergebnisse" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.UnemploymentAggregation}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /unemployment/aggregated [get]
func (h *Handler) GetUnemploymentAggregated(c *gin.Context) {
	query := `SELECT 
		geodata_id,
		geodata_name,
		COUNT(*) as total_records,
		AVG(arbeitslose) as avg_arbeitslose,
		MIN(arbeitslose) as min_arbeitslose,
		MAX(arbeitslose) as max_arbeitslose,
		MAX(observation_date) as latest_observation,
		MIN(observation_date) as earliest_observation
		FROM transformed_data.mart_arbeitslosenzahlen
		WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if fromDate := c.Query("from_date"); fromDate != "" {
		query += fmt.Sprintf(" AND observation_date >= $%d", argPos)
		args = append(args, fromDate)
		argPos++
	}

	if toDate := c.Query("to_date"); toDate != "" {
		query += fmt.Sprintf(" AND observation_date <= $%d", argPos)
		args = append(args, toDate)
		argPos++
	}

	query += " GROUP BY geodata_id, geodata_name ORDER BY geodata_id"

	// Pagination
	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	// Query mit automatischem Struct-Mapping
	aggregations := []models.UnemploymentAggregation{}
	err := h.DB.Select(&aggregations, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    aggregations,
		Meta: &models.MetaInfo{
			Limit:  limit,
			Offset: offset,
		},
	})
}

// GetBevoelkerungData gibt Bevölkerungsdaten zurück
// @Summary Bevölkerungsdaten abrufen
// @Description Gibt Bevölkerungsdaten je Kreis mit optionalen Filtern zurück
// @Tags Bevölkerung
// @Accept json
// @Produce json
// @Param geodata_id_nuts query string false "Filter nach NUTS-3-Code" example(DEF01)
// @Param year query int false "Filter nach Jahr" example(2020)
// @Param limit query int false "Anzahl der Ergebnisse (1-10000)" default(1000) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.BevoelkerungData}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /bevoelkerung [get]
func (h *Handler) GetBevoelkerungData(c *gin.Context) {
	var filter models.BevoelkerungFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Ungültige Query-Parameter: %v", err),
		})
		return
	}

	if filter.Limit == 0 {
		filter.Limit = 1000
	}

	query := `SELECT original_id_1, original_name, geodata_name, geodata_id_nuts,
		geodata_id_ars, mapper, parameter, geodata_level, geodata_year,
		year, bevoelkerung, observation_date::text, has_valid_value
		FROM transformed_data.stg_bevoelkerung_kreise
		WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND geodata_id_nuts = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err),
		})
		return
	}

	query += fmt.Sprintf(" ORDER BY geodata_id_nuts, year LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	data := []models.BevoelkerungData{}
	if err := h.DB.Select(&data, query, args...); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		},
	})
}

// GetSchuldnerquoteData gibt Schuldnerquoten zurück
// @Summary Schuldnerquoten abrufen
// @Description Gibt Schuldnerquoten je Kreis mit optionalen Filtern zurück
// @Tags Schuldnerquote
// @Accept json
// @Produce json
// @Param geodata_id_nuts query string false "Filter nach NUTS-3-Code" example(DEF01)
// @Param year query int false "Filter nach Jahr" example(2020)
// @Param limit query int false "Anzahl der Ergebnisse (1-10000)" default(1000) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.SchuldnerquoteData}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /schuldnerquote [get]
func (h *Handler) GetSchuldnerquoteData(c *gin.Context) {
	var filter models.SchuldnerquoteFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Ungültige Query-Parameter: %v", err),
		})
		return
	}

	if filter.Limit == 0 {
		filter.Limit = 1000
	}

	query := `SELECT original_id_1, original_name, geodata_name, geodata_id_nuts,
		geodata_id_ars, mapper, parameter, geodata_level, geodata_year,
		year, schuldnerquote_prozent, observation_date::text, has_valid_value
		FROM transformed_data.stg_schuldnerquote_kreise
		WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND geodata_id_nuts = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err),
		})
		return
	}

	query += fmt.Sprintf(" ORDER BY geodata_id_nuts, year LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	data := []models.SchuldnerquoteData{}
	if err := h.DB.Select(&data, query, args...); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		},
	})
}
