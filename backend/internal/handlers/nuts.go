package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

type nutsLevelDetails struct {
	NormalizedID string
	Number       int
	Code         string
	Table        string
}

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

// nutsLevelDetailsFromID normalisiert eine NUTS-ID und liefert explizite Level-Informationen.
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

// GetAllNutsRegions gibt alle NUTS-Regionen zurück (optional gefiltert nach Level)
// @Summary Liste aller NUTS-Regionen
// @Description Gibt alle NUTS-Regionen zurück. Mit ?level=0..3 auf ein bestimmtes Level einschränken. Ohne ?level werden alle Levels zusammengefasst.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param level query int false "NUTS-Level (0–3)" Enums(0,1,2,3) example(3)
// @Param limit query int false "Anzahl der Ergebnisse" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.NutsRegionSummary}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts [get]
func (h *Handler) GetAllNutsRegions(c *gin.Context) {
	levelStr := c.Query("level")
	limitNull := normalizeNullableLimitQuery(c)
	limit, offset, err := resolvePagination(c, 100, 0, 0, limitNull)
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
			return `SELECT DISTINCT ON (nuts_id) nuts_id, region_name, geodata_id_ars FROM ` + table + ` ORDER BY nuts_id`
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

	for _, lvl := range levels {
		levelDetails, err := nutsLevelDetailsFromLevel(lvl)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		var cnt int
		if err := h.DB.QueryRow(buildQuery(levelDetails.Table, true)).Scan(&cnt); err != nil {
			respondInternalError(c, fmt.Sprintf("Datenbankfehler (Level %d): %v", lvl, err))
			return
		}
		total += cnt

		rows := []dbRow{}
		query := buildQuery(levelDetails.Table, false)
		paginationQuery, paginationArgs := applyPaginationClause(query, 1, limit, offset)
		if err := h.DB.Select(&rows, paginationQuery, paginationArgs...); err != nil {
			respondInternalError(c, fmt.Sprintf("Datenbankfehler (Level %d): %v", lvl, err))
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

	const selectCols = `nuts_id, region_name, year, geodata_id_ars, bevoelkerung, schuldnerquote_prozent, arbeitslose, yoy_change_percent`
	fromClause := ` FROM ` + levelDetails.Table + ` WHERE nuts_id = $1`

	if allStr == "true" {
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

		metadata := nutsMetadata(levelDetails, rows[0].RegionName, rows[0].GeodataIDArs)
		response := models.NutsRegionTimeSeries{
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
		return
	}

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

	metadata := nutsMetadata(levelDetails, row.RegionName, row.GeodataIDArs)
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

// GetNutsRegionVersions gibt alle verfügbaren Versionsnummern (Jahre) für eine NUTS-Region zurück
// @Summary Versionsliste für eine NUTS-Region
// @Description Gibt alle verfügbaren Jahre einer NUTS-Region zurück (neueste zuerst)
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

	query := `SELECT DISTINCT year FROM ` + levelDetails.Table + ` WHERE nuts_id = $1 ORDER BY year DESC`
	versions := make([]int, 0)
	if err := h.DB.Select(&versions, query, levelDetails.NormalizedID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datenbankfehler: %v", err),
		})
		return
	}

	if len(versions) == 0 {
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
			Versions:      versions,
		},
	})
}
