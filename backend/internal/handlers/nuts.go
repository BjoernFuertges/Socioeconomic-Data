package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

type nutsLevelDetails struct {
	NormalizedID string
	Number       int
	Code         string
	Table        string
}

// nutsMaxLimit caps ?limit= on GET /nuts, in line with the 10000 of the
// Kreis dataset endpoints (limit=null fetches everything).
const nutsMaxLimit = 10000

func nutsLevelDetailsFromLevel(level int) (nutsLevelDetails, error) {
	switch level {
	case 0:
		return nutsLevelDetails{Number: 0, Code: "NUTS-0", Table: "transformed_data.mart_nuts0_summary"}, nil
	case 1:
		return nutsLevelDetails{Number: 1, Code: "NUTS-1", Table: "transformed_data.mart_nuts1_summary"}, nil
	case 2:
		return nutsLevelDetails{Number: 2, Code: "NUTS-2", Table: "transformed_data.mart_nuts2_summary"}, nil
	case 3:
		return nutsLevelDetails{Number: 3, Code: "NUTS-3", Table: "transformed_data.mart_nuts3_summary"}, nil
	default:
		return nutsLevelDetails{}, fmt.Errorf("ungültiges NUTS-Level: erlaubt sind 0–3, erhalten %d", level)
	}
}

// nutsLevelDetailsFromID normalizes a NUTS ID and returns explicit level information.
func nutsLevelDetailsFromID(nutsID string) (nutsLevelDetails, error) {
	normalizedID := strings.ToUpper(strings.TrimSpace(nutsID))

	level := len(normalizedID) - 2
	if level < 0 || level > 3 {
		return nutsLevelDetails{}, fmt.Errorf("ungültige NUTS-ID: erwartet 2–5 Zeichen (NUTS-0 bis NUTS-3), erhalten %d", len(normalizedID))
	}

	details, err := nutsLevelDetailsFromLevel(level)
	if err != nil {
		return nutsLevelDetails{}, err
	}
	details.NormalizedID = normalizedID
	return details, nil
}

func nutsMetadata(details nutsLevelDetails, regionName string, ars *string) *models.NutsRegionMetadata {
	return &models.NutsRegionMetadata{
		NutsID:        details.NormalizedID,
		RegionName:    regionName,
		NutsLevel:     details.Number,
		NutsLevelCode: details.Code,
		ARS:           ars,
	}
}

func (h *Handler) lookupNutsGeometry(ctx context.Context, nutsID string, geometryYear *int) (json.RawMessage, bool, error) {
	var row struct {
		GeometryWGS84    json.RawMessage `db:"geometry_wgs84"`
		HasValidGeometry bool            `db:"has_valid_geometry"`
	}

	query := `SELECT geometry_wgs84, has_valid_geometry
		FROM transformed_data.mart_nuts_geometry_historized
		WHERE nuts_id = $1`
	args := []interface{}{nutsID}

	// In the historized table a year's geometry is unambiguous: "exact" and
	// "latest-before" coincide on the period covering that year.
	if geometryYear != nil {
		query += ` AND valid_from <= $2 AND (valid_to IS NULL OR valid_to >= $2)`
		args = append(args, *geometryYear)
	}

	query += `
		ORDER BY valid_from DESC
		LIMIT 1`

	if err := h.DB.GetContext(ctx, &row, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return row.GeometryWGS84, row.HasValidGeometry, nil
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

// GetAllNutsRegions returns all NUTS regions (optionally filtered by level)
// @Summary Liste aller NUTS-Regionen
// @Description Gibt alle NUTS-Regionen zurück. Mit ?level=0..3 auf ein bestimmtes Level einschränken. Ohne ?level werden alle Levels zusammengefasst.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param level query int false "NUTS-Level (0–3)" Enums(0,1,2,3) example(3)
// @Param limit query int false "Anzahl der Ergebnisse je Level (1-10000, oder null für alle)" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.NutsRegionSummary}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts [get]
func (h *Handler) GetAllNutsRegions(c *gin.Context) {
	levelStr := c.Query("level")
	limitNull := normalizeNullableLimitQuery(c)
	// gin caches the query string on first access (c.Query("level") above), so
	// c.Query("limit") still sees "null" after normalizeNullableLimitQuery
	// removed it from the URL - only parse it as a number without limit=null.
	var requestedLimit *int
	var err error
	if !limitNull {
		requestedLimit, err = parseOptionalIntQuery(c, "limit")
		if err != nil {
			respondBadRequest(c, err.Error())
			return
		}
	}
	requestedOffset, err := parseOptionalIntQuery(c, "offset")
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if requestedLimit != nil && *requestedLimit > nutsMaxLimit {
		respondBadRequest(c, fmt.Sprintf("limit darf höchstens %d sein (für alle Zeilen limit=null)", nutsMaxLimit))
		return
	}
	// Validated here, before any DB access; the limit=null cap is applied by
	// pagination.resolve once total is counted below.
	pagination, err := parsePagination(c, 100, derefInt(requestedLimit), derefInt(requestedOffset), limitNull)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	type dbRow struct {
		NutsID     string  `db:"nuts_id"`
		RegionName string  `db:"region_name"`
		ARS        *string `db:"geodata_id_ars"`
	}

	buildQuery := func(table string, forCount bool) string {
		if forCount {
			return `SELECT COUNT(DISTINCT nuts_id) FROM ` + table
		}
		if strings.Contains(table, "nuts3") {
			return `SELECT DISTINCT ON (nuts_id) nuts_id, region_name, CAST(geodata_id_ars AS text) AS geodata_id_ars FROM ` + table + ` ORDER BY nuts_id`
		}
		return `SELECT DISTINCT ON (nuts_id) nuts_id, region_name, null::text AS geodata_id_ars FROM ` + table + ` ORDER BY nuts_id`
	}

	var levels []int
	if levelStr != "" {
		var lvl int
		if _, err := fmt.Sscanf(levelStr, "%d", &lvl); err != nil || lvl < 0 || lvl > 3 {
			c.JSON(http.StatusBadRequest, models.APIResponse{
				Success: false,
				Error:   "level muss eine Zahl zwischen 0 und 3 sein",
			})
			return
		}
		levels = []int{lvl}
	} else {
		levels = []int{0, 1, 2, 3}
	}

	result := make([]models.NutsRegionSummary, 0)
	total := 0
	levelDetailsList := make([]nutsLevelDetails, 0, len(levels))

	for _, lvl := range levels {
		levelDetails, err := nutsLevelDetailsFromLevel(lvl)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}
		levelDetailsList = append(levelDetailsList, levelDetails)

		var cnt int
		if err := h.DB.QueryRowContext(c.Request.Context(), buildQuery(levelDetails.Table, true)).Scan(&cnt); err != nil {
			respondDBError(c, fmt.Sprintf("Datenbankfehler (Level %d)", lvl), err)
			return
		}
		total += cnt
	}

	// Pagination applies per level, so total-offset is an upper bound of the
	// rows a limit=null request returns. With chunk_size the response is paged
	// and the cap does not apply (resolve).
	limit, offset, err := pagination.resolve(total)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	for _, levelDetails := range levelDetailsList {
		lvl := levelDetails.Number
		rows := []dbRow{}
		query := buildQuery(levelDetails.Table, false)
		paginationQuery, paginationArgs := applyPaginationClause(query, 1, limit, offset)
		if err := h.DB.SelectContext(c.Request.Context(), &rows, paginationQuery, paginationArgs...); err != nil {
			respondDBError(c, fmt.Sprintf("Datenbankfehler (Level %d)", lvl), err)
			return
		}

		for _, row := range rows {
			result = append(result, models.NutsRegionSummary{
				NutsID:        row.NutsID,
				RegionName:    row.RegionName,
				NutsLevel:     lvl,
				NutsLevelCode: levelDetails.Code,
				ARS:           row.ARS,
			})
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

// GetNutsRegionData returns all indicators for a NUTS region (levels 0-3)
// @Summary Alle Indikatoren für eine NUTS-Region (Level 0–3)
// @Description Gibt Bevölkerung, Schuldnerquote und Arbeitslosenzahlen für eine NUTS-Region zurück. Das NUTS-Level wird automatisch anhand der Code-Länge erkannt: 2 Zeichen = NUTS-0 (Land), 3 = NUTS-1 (Bundesland), 4 = NUTS-2 (Regierungsbezirk), 5 = NUTS-3 (Kreis). Standard: neuestes verfügbares Jahr. Mit ?all=true: Zeitreihe aller Jahre. Mit ?year=YYYY: bestimmtes Jahr. Mit ?part=metadata werden nur Geo-Metadaten zurückgegeben, mit ?part=socioeconomic nur die sozioökonomischen Daten.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param nuts_id path string true "NUTS-Code der Region (Level 0–3)" example(DE111)
// @Param year query int false "Jahr (gibt neuestes Jahr zurück wenn nicht angegeben)" example(2022)
// @Param all query bool false "Alle verfügbaren Jahre zurückgeben" example(false)
// @Param part query string false "Antwortausschnitt: full, metadata oder socioeconomic" Enums(full,metadata,socioeconomic) default(full)
// @Param geometry_strategy query string false "Geometrieauswahl: latest oder exact" Enums(latest,exact) default(latest)
// @Param geometry_year query int false "Zieljahr für Geometrie (bei exact erforderlich)" example(2024)
// @Param include_datasets query bool false "Verfügbare Soziodata-Datensätze einbetten" example(false)
// @Success 200 {object} models.APIResponse{data=models.NutsRegionResponse}
// @Success 200 {object} models.APIResponse{data=models.NutsRegionTimeSeries}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id} [get]
func (h *Handler) GetNutsRegionData(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	year, err := parseOptionalIntQuery(c, "year")
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	allStr := c.Query("all")
	partStr := c.Query("part")
	includeDatasets := c.Query("include_datasets") == "true"

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

	geometryStrategy, err := normalizeGeometryStrategy(c.Query("geometry_strategy"))
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var geometryYear *int
	if geometryYearStr := strings.TrimSpace(c.Query("geometry_year")); geometryYearStr != "" {
		parsed, parseErr := strconv.Atoi(geometryYearStr)
		if parseErr != nil {
			respondBadRequest(c, "geometry_year muss eine gültige Jahreszahl sein")
			return
		}
		geometryYear = &parsed
	}
	if geometryStrategy == "exact" && geometryYear == nil {
		respondBadRequest(c, "geometry_year ist erforderlich, wenn geometry_strategy=exact gesetzt ist")
		return
	}

	const selectCols = `nuts_id, region_name, year, CAST(geodata_id_ars AS text) AS geodata_id_ars, bevoelkerung, schuldnerquote_prozent, arbeitslose, yoy_change_percent`
	fromClause := ` FROM ` + levelDetails.Table + ` WHERE nuts_id = $1`

	if allStr == "true" {
		query := `SELECT ` + selectCols + fromClause + ` ORDER BY year DESC`
		rows := []models.NutsRegionData{}
		if err := h.DB.SelectContext(c.Request.Context(), &rows, query, levelDetails.NormalizedID); err != nil {
			respondDBError(c, "Datenbankfehler", err)
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

		metadata := nutsMetadata(levelDetails, rows[0].RegionName, rows[0].GeodataIDArs)
		geometryWGS84, hasValidGeometry, err := h.lookupNutsGeometry(c.Request.Context(), levelDetails.NormalizedID, geometryYear)
		if err != nil {
			respondDBError(c, "Datenbankfehler", err)
			return
		}
		metadata.GeometryWGS84 = geometryWGS84
		metadata.HasValidGeometry = hasValidGeometry
		response := models.NutsRegionTimeSeries{
			Metadata:      metadata,
			Socioeconomic: socioeconomic,
		}

		if includeDatasets {
			if datasets, dsErr := h.regionNutsDatasets(c.Request.Context(), levelDetails.NormalizedID, levelDetails.Number); dsErr == nil {
				response.Datasets = datasets
			}
		}

		if part == "metadata" {
			response.Socioeconomic = nil
		}
		if part == "socioeconomic" {
			response.Metadata = nil
		}

		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: response})
		return
	}

	query := `SELECT ` + selectCols + fromClause
	args := []interface{}{levelDetails.NormalizedID}
	if year != nil {
		query += ` AND year = $2`
		args = append(args, *year)
	} else {
		query += ` ORDER BY year DESC LIMIT 1`
	}

	var row models.NutsRegionData
	if err := h.DB.GetContext(c.Request.Context(), &row, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Keine Daten für diese NUTS-Region gefunden",
			})
			return
		}
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	metadata := nutsMetadata(levelDetails, row.RegionName, row.GeodataIDArs)
	geometryWGS84, hasValidGeometry, err := h.lookupNutsGeometry(c.Request.Context(), levelDetails.NormalizedID, geometryYear)
	if err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}
	metadata.GeometryWGS84 = geometryWGS84
	metadata.HasValidGeometry = hasValidGeometry
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

	if includeDatasets {
		if datasets, dsErr := h.regionNutsDatasets(c.Request.Context(), levelDetails.NormalizedID, levelDetails.Number); dsErr == nil {
			response.Datasets = datasets
		}
	}

	if part == "metadata" {
		response.Socioeconomic = nil
	}
	if part == "socioeconomic" {
		response.Metadata = nil
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: response})
}

// GetNutsRegionVersions returns all geometry periods of a NUTS region
// @Summary Geometrie-Perioden einer NUTS-Region
// @Description Gibt alle Geometrie-Perioden (valid_from/valid_to) einer NUTS-Region aus der historisierten Geometrietabelle zurück (neueste zuerst). valid_to=null bedeutet noch aktuell.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param nuts_id path string true "NUTS-Code der Region (Level 0–3)" example(DE111)
// @Success 200 {object} models.APIResponse{data=models.NutsRegionVersions}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id}/versions [get]
func (h *Handler) GetNutsRegionVersions(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	if strings.TrimSpace(nutsID) == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "nuts_id darf nicht leer sein",
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

	periods := make([]models.GeoPeriod, 0)
	query := `SELECT valid_from, valid_to
		FROM transformed_data.mart_nuts_geometry_historized
		WHERE nuts_id = $1
		ORDER BY valid_from DESC`
	if err := h.DB.SelectContext(c.Request.Context(), &periods, query, levelDetails.NormalizedID); err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	if len(periods) == 0 {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   "Keine Versionsdaten für diese NUTS-Region gefunden",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: models.NutsRegionVersions{
			NutsID:        levelDetails.NormalizedID,
			NutsLevel:     levelDetails.Number,
			NutsLevelCode: levelDetails.Code,
			Periods:       periods,
		},
	})
}

// GetNutsLauAggregate returns the LAU values (Gemeinden) of a Kreis region aggregated to NUTS-3 level
// @Summary Auf NUTS-3-Ebene aggregierte LAU-Werte (Gemeinden)
// @Description Gibt eine Zeitreihe der auf diesen Kreis hochaggregierten Gemeinde-Werte zurück (Bevölkerungssumme, Anzahl zugeordneter Gemeinden, bevölkerungsgewichtete Beschäftigungsquote) - als Ergänzung/Cross-Check zu den nativen Kreis-Werten aus GET /nuts/{nuts_id}. Nur für NUTS-3-IDs (5 Zeichen) gültig.
// @Tags NUTS
// @Accept json
// @Produce json,text/csv
// @Param nuts_id path string true "NUTS-3-Code des Kreises" example(DE300)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=models.NutsLauAggregateResponse}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id}/lau-aggregate [get]
func (h *Handler) GetNutsLauAggregate(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	if strings.TrimSpace(nutsID) == "" {
		respondBadRequest(c, "nuts_id darf nicht leer sein")
		return
	}

	levelDetails, err := nutsLevelDetailsFromID(nutsID)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if levelDetails.Number != 3 {
		respondBadRequest(c, "lau-aggregate ist nur für NUTS-3-Regionen (Kreise) verfügbar")
		return
	}

	series := []models.NutsLauAggregatePoint{}
	query := `SELECT year, lau_population_sum, lau_region_count, lau_region_count_with_population, lau_beschaeftigungsquote_weighted_avg, lau_beschaeftigungsquote_region_count
		FROM transformed_data.mart_nuts3_lau_aggregate
		WHERE nuts3_id = $1
		ORDER BY year DESC`
	if err := h.DB.SelectContext(c.Request.Context(), &series, query, levelDetails.NormalizedID); err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	if len(series) == 0 {
		respondNotFound(c, "Keine LAU-Aggregatdaten für diese NUTS-3-Region gefunden")
		return
	}

	respondNegotiated(c, models.NutsLauAggregateResponse{
		NutsID: levelDetails.NormalizedID,
		Series: series,
	}, series, nil)
}

// GetNutsDatasetAggregateTimeseries returns a Kreis-grain dataset's time series aggregated to NUTS-0/1/2 level
// @Summary Auf NUTS-0/1/2-Ebene aggregierte Datensatz-Zeitreihe
// @Description Aggregiert einen Kreis-grain-Soziodata-Datensatz (siehe GET /nuts/{nuts_id}/datasets) auf Land-/Bundesland-/Regierungsbezirksebene hoch (Summe bzw. bevölkerungsgewichteter Durchschnitt je nach Datensatz, bei Werten je Erwerbstätigen erwerbstätigengewichtet, siehe mart_nuts012_dataset_aggregate). Pendant zu GET /{slug}/timeseries für NUTS-3, das für Level 0-2 keine Kreis-Zeilen findet. Nur für NUTS-0/1/2-IDs (2-4 Zeichen) gültig; für Kreise (NUTS-3) GET /{slug}/timeseries verwenden.
// @Tags NUTS
// @Accept json
// @Produce json,text/csv
// @Param nuts_id path string true "NUTS-0/1/2-Code der Region" example(DE1)
// @Param slug path string true "Route-Slug des Datensatzes (siehe GET /nuts/{nuts_id}/datasets)" example(bevoelkerung)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=[]models.NutsDatasetAggregatePoint}
// @Failure 400 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id}/dataset-aggregate/{slug} [get]
func (h *Handler) GetNutsDatasetAggregateTimeseries(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	if strings.TrimSpace(nutsID) == "" {
		respondBadRequest(c, "nuts_id darf nicht leer sein")
		return
	}

	levelDetails, err := nutsLevelDetailsFromID(nutsID)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if levelDetails.Number == 3 {
		respondBadRequest(c, "dataset-aggregate ist nur für NUTS-0/1/2-Regionen verfügbar - für Kreise (NUTS-3) GET /{slug}/timeseries verwenden")
		return
	}

	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	cfg, ok := models.DatasetConfigBySlug(slug)
	if !ok || !cfg.IsHarmonized {
		respondBadRequest(c, fmt.Sprintf("unbekannter oder nicht aggregierbarer Datensatz-Slug %q", slug))
		return
	}

	points := []models.NutsDatasetAggregatePoint{}
	query := `SELECT nuts_id, dataset_slug, year, value, source_region_count, true AS is_computed
		FROM transformed_data.mart_nuts012_dataset_aggregate
		WHERE nuts_id = $1 AND dataset_slug = $2
		ORDER BY year DESC`
	if err := h.DB.SelectContext(c.Request.Context(), &points, query, levelDetails.NormalizedID, cfg.RouteSlug); err != nil {
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	respondList(c, points, nil)
}
