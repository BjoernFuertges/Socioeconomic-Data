package handlers

import (
	"fmt"
	"net/http"
	"strings"

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

	query := `SELECT arbeitslosenzahlen_pk, geodata_id, geodata_name, geodata_id_ars,
		geodata_level, geodata_year, year, observation_date, arbeitslose,
		arbeitslose_previous_year, yoy_change_percent
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

type nutsLevelDetails struct {
	NormalizedID string
	Number       int
	Code         string
	Table        string
}

// nutsLevelDetailsFromID normalisiert eine NUTS-ID und liefert explizite Level-Informationen.
func nutsLevelDetailsFromID(nutsID string) (nutsLevelDetails, error) {
	normalizedID := strings.ToUpper(strings.TrimSpace(nutsID))

	switch len(normalizedID) {
	case 2:
		return nutsLevelDetails{NormalizedID: normalizedID, Number: 0, Code: "NUTS-0", Table: "transformed_data.mart_nuts0_summary"}, nil
	case 3:
		return nutsLevelDetails{NormalizedID: normalizedID, Number: 1, Code: "NUTS-1", Table: "transformed_data.mart_nuts1_summary"}, nil
	case 4:
		return nutsLevelDetails{NormalizedID: normalizedID, Number: 2, Code: "NUTS-2", Table: "transformed_data.mart_nuts2_summary"}, nil
	case 5:
		return nutsLevelDetails{NormalizedID: normalizedID, Number: 3, Code: "NUTS-3", Table: "transformed_data.mart_nuts3_summary"}, nil
	default:
		return nutsLevelDetails{}, fmt.Errorf("ungültige NUTS-ID: erwartet 2–5 Zeichen (NUTS-0 bis NUTS-3), erhalten %d", len(normalizedID))
	}
}

func nutsMetadata(details nutsLevelDetails, regionName string) *models.NutsRegionMetadata {
	return &models.NutsRegionMetadata{
		NutsID:        details.NormalizedID,
		RegionName:    regionName,
		NutsLevel:     details.Number,
		NutsLevelCode: details.Code,
	}
}

func normalizeNutsResponsePart(part string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "", "full":
		return "full", nil
	case "metadata", "geo", "geodata":
		return "metadata", nil
	case "socioeconomic", "socio", "stats":
		return "socioeconomic", nil
	default:
		return "", fmt.Errorf("ungültiger Query-Parameter part: erlaubt sind full, metadata oder socioeconomic")
	}
}

// GetNutsRegionData gibt alle Indikatoren für eine NUTS-Region zurück (Level 0–3)
// @Summary Alle Indikatoren für eine NUTS-Region (Level 0–3)
// @Description Gibt Bevölkerung, Schuldnerquote und Arbeitslosenzahlen für eine NUTS-Region zurück. Das NUTS-Level wird automatisch anhand der Code-Länge erkannt: 2 Zeichen = NUTS-0 (Land), 3 = NUTS-1 (Bundesland), 4 = NUTS-2 (Regierungsbezirk), 5 = NUTS-3 (Kreis). Standard: neuestes verfügbares Jahr. Mit ?all=true: Zeitreihe aller Jahre. Mit ?year=YYYY: bestimmtes Jahr. Mit ?part=metadata werden nur Geo-Metadaten zurückgegeben, mit ?part=socioeconomic nur die sozioökonomischen Daten. Für NUTS-Level 0–2 enthält region_name mangels Namenstabelle den NUTS-Code selbst.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param nuts_id path string true "NUTS-Code der Region (Level 0–3)" example(DE111)
// @Param year query int false "Jahr (gibt neuestes Jahr zurück wenn nicht angegeben)" example(2022)
// @Param all query bool false "Alle verfügbaren Jahre zurückgeben" example(false)
// @Param part query string false "Antwortausschnitt: full, metadata oder socioeconomic" Enums(full,metadata,socioeconomic) default(full)
// @Success 200 {object} models.APIResponse{data=models.NutsRegionResponse}
// @Success 200 {object} models.APIResponse{data=models.NutsRegionTimeSeries}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id} [get]
func (h *Handler) GetNutsRegionData(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	yearStr := c.Query("year")
	allStr := c.Query("all")
	partStr := c.Query("part")

	if strings.TrimSpace(nutsID) == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "nuts_id darf nicht leer sein",
		})
		return
	}

	part, err := normalizeNutsResponsePart(partStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	levelDetails, err := nutsLevelDetailsFromID(nutsID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	const selectCols = `nuts_id, region_name, year, bevoelkerung, schuldnerquote_prozent, arbeitslose, yoy_change_percent`
	// table enthält ausschließlich hardkodierte Strings aus nutsLevelDetailsFromID() – kein SQL-Injection-Risiko.
	fromClause := ` FROM ` + levelDetails.Table + ` WHERE nuts_id = $1`

	if allStr == "true" {
		// Zeitreihe: alle Jahre für diese Region
		query := `SELECT ` + selectCols + fromClause + ` ORDER BY year DESC`
		rows := []models.NutsRegionData{}
		if err := h.DB.Select(&rows, query, levelDetails.NormalizedID); err != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   fmt.Sprintf("Datenbankfehler: %v", err),
			})
			return
		}
		if len(rows) == 0 {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Keine Daten für diese NUTS-Region gefunden",
			})
			return
		}

		socioeconomic := make([]models.NutsSocioeconomicData, 0, len(rows))
		for _, record := range rows {
			socioeconomic = append(socioeconomic, models.NutsSocioeconomicData{
				Year:                  record.Year,
				Bevoelkerung:          record.Bevoelkerung,
				SchuldnerquoteProzent: record.SchuldnerquoteProzent,
				Arbeitslose:           record.Arbeitslose,
				YoYChangePercent:      record.YoYChangePercent,
			})
		}

		metadata := nutsMetadata(levelDetails, rows[0].RegionName)
		ts := models.NutsRegionTimeSeries{
			Metadata:      metadata,
			Socioeconomic: socioeconomic,
		}

		if part == "metadata" {
			ts.Socioeconomic = nil
		}
		if part == "socioeconomic" {
			ts.Metadata = nil
		}

		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: ts})
		return
	}

	// Snapshot: ein Jahr (angegeben oder neuestes)
	query := `SELECT ` + selectCols + fromClause
	args := []interface{}{levelDetails.NormalizedID}
	if yearStr != "" {
		query += ` AND year = $2`
		args = append(args, yearStr)
	} else {
		query += ` ORDER BY year DESC LIMIT 1`
	}

	var row models.NutsRegionData
	if err := h.DB.Get(&row, query, args...); err != nil {
		if err.Error() == "sql: no rows in result set" {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Keine Daten für diese NUTS-Region gefunden",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	metadata := nutsMetadata(levelDetails, row.RegionName)
	socioeconomic := &models.NutsSocioeconomicData{
		Year:                  row.Year,
		Bevoelkerung:          row.Bevoelkerung,
		SchuldnerquoteProzent: row.SchuldnerquoteProzent,
		Arbeitslose:           row.Arbeitslose,
		YoYChangePercent:      row.YoYChangePercent,
	}
	response := models.NutsRegionResponse{
		Metadata:      metadata,
		Socioeconomic: socioeconomic,
	}

	if part == "metadata" {
		response.Socioeconomic = nil
	}
	if part == "socioeconomic" {
		response.Metadata = nil
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: response})
}

// sozioKreisBaseColumns ist die SELECT-Spaltenliste gemeinsam für alle Kreis-Datensätze.
// observation_date wird explizit als text gecastet, damit sqlx es in ein string-Feld scannen kann.
const sozioKreisBaseColumns = `d.original_id_1, d.original_name, d.geodata_name, d.geodata_id_nuts, ` +
	`coalesce(` +
	`nullif(to_jsonb(d)->>'geodata_id_lau', ''), ` +
	`case when d.geodata_level = 'LAU' then nullif(d.geodata_id_ars, '') end, ` +
	`case when d.geodata_level = 'LAU' then d.original_id_1 end, ''` +
	`) as geodata_id_lau, ` +
	`d.geodata_id_ars, d.mapper, d.parameter, d.geodata_level, d.geodata_year, ` +
	`d.year, d.observation_date::text, d.has_valid_value`

// getKreisData ist der generische Handler-Kern für alle Kreis-Datensätze.
// T muss SozioBaseData einbetten und alle cfg.ValueColumns via db-Tags mappen.
func getKreisData[T any](h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Ungültige Query-Parameter: %v", err),
		})
		return
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	query := fmt.Sprintf(
		`SELECT %s, %s FROM %s.%s d WHERE 1=1`,
		sozioKreisBaseColumns,
		strings.Join(cfg.ValueColumns, ", "),
		cfg.Schema,
		cfg.Table,
	)

	args := []interface{}{}
	argPos := 1

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND d.geodata_id_nuts = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND d.year = $%d", argPos)
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

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", cfg.OrderBy, argPos, argPos+1)
	args = append(args, filter.Limit, filter.Offset)

	data := make([]T, 0)
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
