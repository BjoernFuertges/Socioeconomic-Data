package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// GetUnemploymentData gibt Arbeitslosenzahlen zurück
// @Summary Arbeitslosenzahlen abrufen
// @Description Gibt Arbeitslosenzahlen mit optionalen Filtern zurück
// @Tags Unemployment
// @Accept json
// @Produce json
// @Param geodata_id query string false "Filter nach Geodaten-ID" example(DE111)
// @Param from_date query string false "Start-Datum (YYYY-MM-DD)" example(2024-01-01)
// @Param to_date query string false "End-Datum (YYYY-MM-DD)" example(2024-12-31)
// @Param year query int false "Filter nach Jahr" example(2024)
// @Param limit query int false "Anzahl der Ergebnisse (1-10000)" default(1000) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.UnemploymentData}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /unemployment [get]
func (h *Handler) GetUnemploymentData(c *gin.Context) {
	var filter models.UnemploymentFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	if filter.Limit == 0 {
		filter.Limit = 1000
	}

	query := `SELECT arbeitslosenzahlen_pk, geodata_id, geodata_name, geodata_id_ars,
		geodata_level, geodata_year, year, observation_date, arbeitslose,
		arbeitslose_previous_year, yoy_change_percent
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

	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	err := h.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	query += fmt.Sprintf(" ORDER BY observation_date DESC, geodata_id LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	data := []models.UnemploymentData{}
	err = h.DB.Select(&data, query, args...)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
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

	query := `SELECT arbeitslosenzahlen_pk, geodata_id, geodata_name, geodata_id_ars,
		geodata_level, geodata_year, year, observation_date, arbeitslose,
		arbeitslose_previous_year, yoy_change_percent
		FROM transformed_data.mart_arbeitslosenzahlen
		WHERE geodata_id = $1`

	args := []interface{}{geodataID}
	argPos := 2

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

	query += " ORDER BY observation_date ASC"

	data := []models.UnemploymentData{}
	err := h.DB.Select(&data, query, args...)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	if len(data) == 0 {
		respondNotFound(c, "Keine Daten für diese Geodaten-ID gefunden")
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

	limit, offset := parseLimitOffset(c, 100)

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	aggregations := []models.UnemploymentAggregation{}
	err := h.DB.Select(&aggregations, query, args...)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
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
