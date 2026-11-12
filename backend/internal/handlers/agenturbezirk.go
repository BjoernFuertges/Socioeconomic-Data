package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// GetAgenturbezirke returns all Agenturbezirke of the Bundesagentur für Arbeit.
// @Summary Liste aller Agenturbezirke
// @Description Gibt alle Agenturbezirke mit Geometrie zurück. Agenturbezirke sind eine eigenständige Gebietsgliederung, nicht deckungsgleich mit NUTS-3/LAU/ARS.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkSummary}
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke [get]
func (h *Handler) GetAgenturbezirke(c *gin.Context) {
	query := `SELECT DISTINCT ON (ba_schl)
		ba_schl, name, geometry_wgs84, has_valid_geometry,
		valid_from::text as valid_from, valid_to::text as valid_to
		FROM transformed_data.mart_agenturbezirk_sociodata
		ORDER BY ba_schl`

	agenturbezirke := []models.AgenturbezirkSummary{}
	if err := h.DB.Select(&agenturbezirke, query); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    agenturbezirke,
		Meta: &models.MetaInfo{
			Total: len(agenturbezirke),
		},
	})
}

// GetAgenturbezirkByID returns a single Agenturbezirk.
// @Summary Agenturbezirk-Details
// @Description Gibt Details eines Agenturbezirks mit Geometrie zurück
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Success 200 {object} models.APIResponse{data=models.AgenturbezirkSummary}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/{ba_schl} [get]
func (h *Handler) GetAgenturbezirkByID(c *gin.Context) {
	baSchl := strings.TrimSpace(c.Param("ba_schl"))

	query := `SELECT DISTINCT ON (ba_schl)
		ba_schl, name, geometry_wgs84, has_valid_geometry,
		valid_from::text as valid_from, valid_to::text as valid_to
		FROM transformed_data.mart_agenturbezirk_sociodata
		WHERE ba_schl = $1`

	var agenturbezirk models.AgenturbezirkSummary
	err := h.DB.Get(&agenturbezirk, query, baSchl)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(c, "Agenturbezirk nicht gefunden")
			return
		}
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    agenturbezirk,
	})
}

// GetAgenturbezirkArbeitslosigkeit returns the monthly Arbeitslose time series of an Agenturbezirk.
// @Summary Monatliche Arbeitslosenzahlen eines Agenturbezirks
// @Description Gibt die monatliche Zeitreihe (Arbeitslose, Unterbeschäftigung) für einen Agenturbezirk zurück (AfA Lange Zeitreihen, ab Januar 2007)
// @Tags Agenturbezirke
// @Accept json
// @Produce json,text/csv
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkArbeitslosigkeitPoint}
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/{ba_schl}/arbeitslosigkeit [get]
func (h *Handler) GetAgenturbezirkArbeitslosigkeit(c *gin.Context) {
	baSchl := strings.TrimSpace(c.Param("ba_schl"))

	var exists bool
	if err := h.DB.Get(&exists, `SELECT EXISTS(SELECT 1 FROM transformed_data.mart_agenturbezirk_sociodata WHERE ba_schl = $1)`, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if !exists {
		respondNotFound(c, "Agenturbezirk nicht gefunden")
		return
	}

	query := `SELECT year, month, observation_date::text as observation_date, arbeitslose, unterbeschaeftigung
		FROM transformed_data.mart_agenturbezirk_sociodata
		WHERE ba_schl = $1 AND year IS NOT NULL
		ORDER BY observation_date`

	points := []models.AgenturbezirkArbeitslosigkeitPoint{}
	if err := h.DB.Select(&points, query, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	respondList(c, points, nil)
}

// GetAgenturbezirkeLatest returns all Agenturbezirke with geometry and their
// latest reporting month, for map rendering.
// @Summary Agenturbezirke mit neuestem Berichtsmonat (für die Karte)
// @Description Gibt jeden Agenturbezirk mit Geometrie und dem jeweils neuesten verfügbaren Monatswert (Arbeitslose, Unterbeschäftigung) zurück.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkLatestValue}
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/latest [get]
func (h *Handler) GetAgenturbezirkeLatest(c *gin.Context) {
	query := `SELECT
		ba_schl, name, geometry_wgs84, has_valid_geometry,
		year, month, observation_date::text as observation_date,
		arbeitslose, unterbeschaeftigung
		FROM transformed_data.mart_agenturbezirk_latest
		ORDER BY ba_schl`

	latest := []models.AgenturbezirkLatestValue{}
	if err := h.DB.Select(&latest, query); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    latest,
		Meta: &models.MetaInfo{
			Total: len(latest),
		},
	})
}

// GetAgenturbezirkePeriods returns all reporting months with Agenturbezirk
// Arbeitslose data, for the year/month picker on the map (analogous to the
// year/version picker for NUTS/LAU).
// @Summary Verfügbare Berichtsmonate der Agenturbezirke
// @Description Gibt alle distinkten (Jahr, Monat)-Kombinationen zurück, für die Arbeitslosenzahlen vorliegen (neueste zuerst).
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkPeriod}
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/periods [get]
func (h *Handler) GetAgenturbezirkePeriods(c *gin.Context) {
	query := `SELECT DISTINCT year, month, observation_date::text as observation_date
		FROM transformed_data.mart_agenturbezirk_sociodata
		WHERE year IS NOT NULL
		ORDER BY year DESC, month DESC`

	periods := []models.AgenturbezirkPeriod{}
	if err := h.DB.Select(&periods, query); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    periods,
		Meta: &models.MetaInfo{
			Total: len(periods),
		},
	})
}

// GetAgenturbezirkeByPeriod returns all Agenturbezirke with geometry and the
// value of a given reporting month, for map rendering at a selectable point in
// time. Districts without a value for that month are absent from the response.
// @Summary Agenturbezirke zu einem gewählten Berichtsmonat (für die Karte)
// @Description Gibt jeden Agenturbezirk mit Geometrie und dem Monatswert (Arbeitslose, Unterbeschäftigung) für das angegebene Jahr/Monat zurück.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param year query int true "Jahr" example(2026)
// @Param month query int true "Monat (1-12)" example(8)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkLatestValue}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/period [get]
func (h *Handler) GetAgenturbezirkeByPeriod(c *gin.Context) {
	var filter struct {
		Year  int `form:"year" binding:"required"`
		Month int `form:"month" binding:"required,min=1,max=12"`
	}
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	query := `SELECT
		ba_schl, name, geometry_wgs84, has_valid_geometry,
		year, month, observation_date::text as observation_date,
		arbeitslose, unterbeschaeftigung
		FROM transformed_data.mart_agenturbezirk_sociodata
		WHERE year = $1 AND month = $2
		ORDER BY ba_schl`

	values := []models.AgenturbezirkLatestValue{}
	if err := h.DB.Select(&values, query, filter.Year, filter.Month); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    values,
		Meta: &models.MetaInfo{
			Total: len(values),
		},
	})
}

// GetAgenturbezirkNuts3 returns the n:m mapping of an Agenturbezirk to
// overlapping NUTS-3 regions, with area share in both directions. Points to the
// complementary Kreis-level dataset "Arbeitslose" (INKAR/BBSR).
// @Summary NUTS-3-Zuordnung eines Agenturbezirks (Flächenanteile)
// @Description Gibt die überlappenden NUTS-3-Regionen eines Agenturbezirks zurück, mit Flächenanteil in beide Richtungen. Agenturbezirke bestehen i.d.R. aus einem oder mehreren ganzen Kreisen (Ausnahme Berlin: 3 Agenturbezirke teilen sich einen Kreis) - eine räumliche Schätzung, keine amtliche Zuordnung.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturNuts3Share}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/{ba_schl}/nuts3 [get]
func (h *Handler) GetAgenturbezirkNuts3(c *gin.Context) {
	baSchl := strings.TrimSpace(c.Param("ba_schl"))

	var exists bool
	if err := h.DB.Get(&exists, `SELECT EXISTS(SELECT 1 FROM transformed_data.mart_agenturbezirk_sociodata WHERE ba_schl = $1)`, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if !exists {
		respondNotFound(c, "Agenturbezirk nicht gefunden")
		return
	}

	query := `SELECT
		nuts_id, region_name, area_share_of_agentur, area_share_of_nuts3
		FROM transformed_data.mart_agentur_nuts3_crosswalk
		WHERE ba_schl = $1
		ORDER BY area_share_of_agentur DESC`

	shares := []models.AgenturNuts3Share{}
	if err := h.DB.Select(&shares, query, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    shares,
		Meta: &models.MetaInfo{
			Total: len(shares),
		},
	})
}

// GetAgenturbezirkNutsHierarchy returns where an Agenturbezirk sits in the
// NUTS hierarchy.
// @Summary NUTS-Hierarchie eines Agenturbezirks (Kreis/Regierungsbezirk/Bundesland)
// @Description Gibt alle überlappenden NUTS-1/2/3-Regionen eines Agenturbezirks zurück. is_fully_contained markiert Regionen, die den Agenturbezirk vollständig enthalten (Flächenanteil >= 99 %); is_parent die kleinste davon (übergeordnete Region). Reicht ein Agenturbezirk über Bundeslandgrenzen hinaus, ist keine Zeile is_parent. Räumliche Schätzung, keine amtliche Zuordnung.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturNutsMembership}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/{ba_schl}/nuts-hierarchy [get]
func (h *Handler) GetAgenturbezirkNutsHierarchy(c *gin.Context) {
	baSchl := strings.TrimSpace(c.Param("ba_schl"))

	var exists bool
	if err := h.DB.Get(&exists, `SELECT EXISTS(SELECT 1 FROM transformed_data.mart_agenturbezirk_latest WHERE ba_schl = $1)`, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if !exists {
		respondNotFound(c, "Agenturbezirk nicht gefunden")
		return
	}

	query := `SELECT nuts_id, nuts_level, region_name, area_share_of_agentur, is_fully_contained, is_parent
		FROM transformed_data.mart_agentur_nuts_membership
		WHERE ba_schl = $1
		ORDER BY nuts_level DESC, area_share_of_agentur DESC`

	rows := []models.AgenturNutsMembership{}
	if err := h.DB.Select(&rows, query, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    rows,
		Meta:    &models.MetaInfo{Total: len(rows)},
	})
}

// GetNutsAgenturbezirke returns the Agenturbezirke that overlap a NUTS region
// (levels 1-3).
// @Summary Agenturbezirke einer NUTS-Region
// @Description Gibt die Agenturbezirke zurück, die eine NUTS-1/2/3-Region überlappen. is_fully_contained=true: der Agenturbezirk liegt vollständig in der Region (untergeordneter Agenturbezirk); false: nur teilweise Überlappung. Die meisten Kreise haben keinen vollständig enthaltenen Agenturbezirk, weil sie nur Teil eines größeren Agenturbezirks sind. NUTS-0 wird nicht unterstützt. Räumliche Schätzung, keine amtliche Zuordnung.
// @Tags NUTS
// @Accept json
// @Produce json
// @Param nuts_id path string true "NUTS-Code der Region (Level 1–3)" example(DE30)
// @Success 200 {object} models.APIResponse{data=[]models.NutsAgenturbezirk}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id}/agenturbezirke [get]
func (h *Handler) GetNutsAgenturbezirke(c *gin.Context) {
	details, err := nutsLevelDetailsFromID(c.Param("nuts_id"))
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if details.Number < 1 {
		respondBadRequest(c, "NUTS-0 wird nicht unterstützt: Agenturbezirke sind ab NUTS-1 zugeordnet")
		return
	}

	query := `SELECT m.ba_schl, l.name, m.area_share_of_agentur, m.is_fully_contained
		FROM transformed_data.mart_agentur_nuts_membership m
		LEFT JOIN transformed_data.mart_agenturbezirk_latest l ON l.ba_schl = m.ba_schl
		WHERE m.nuts_id = $1
		ORDER BY m.is_fully_contained DESC, m.area_share_of_agentur DESC, l.name`

	rows := []models.NutsAgenturbezirk{}
	if err := h.DB.Select(&rows, query, details.NormalizedID); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    rows,
		Meta:    &models.MetaInfo{Total: len(rows)},
	})
}

// GetAgenturbezirkDatasetAggregateTimeseries returns a Kreis-grain dataset's
// time series aggregated up to Agenturbezirk level.
// @Summary Auf Agenturbezirk-Ebene aggregierte Datensatz-Zeitreihe
// @Description Aggregiert einen Kreis-grain-Soziodata-Datensatz (siehe GET /nuts/{nuts_id}/datasets) über den Agentur<->NUTS-3-Flächenanteils-Crosswalk auf Agenturbezirk-Ebene hoch (Summe bzw. bevölkerungsgewichteter Durchschnitt je nach Datensatz, siehe mart_agentur_dataset_aggregate). Analog zu GET /nuts/{nuts_id}/dataset-aggregate/{slug} für NUTS-0/1/2. LIMITATIONEN: Summen (Zählgrößen) sind Summen der Kreis-Werte; Quoten und Durchschnitte sind ein bevölkerungsgewichteter Durchschnitt der Kreis-Werte (Werte je Erwerbstätigen: erwerbstätigengewichtet; Rückfall auf Flächengewichte, wenn die Gewichte fehlen) - eine Näherung, keine exakte Neuberechnung, da Zähler/Nenner nicht vorliegen und die Gesamtbevölkerung nur Ersatz für die eigentliche Bezugsgröße ist. Mediane lassen sich nicht aus Kreis-Medianen zusammensetzen (nur Schätzung). Berlin: ein Kreis verteilt sich auf drei Agenturbezirke, Summen werden flächenanteilig aufgeteilt. Pro Punkt kennzeichnen aggregation_method und value_kind das Verfahren.
// @Tags Agenturbezirke
// @Accept json
// @Produce json,text/csv
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Param slug path string true "Route-Slug des Datensatzes" example(bevoelkerung)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregatePoint}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/{ba_schl}/dataset-aggregate/{slug} [get]
func (h *Handler) GetAgenturbezirkDatasetAggregateTimeseries(c *gin.Context) {
	baSchl := strings.TrimSpace(c.Param("ba_schl"))

	var exists bool
	if err := h.DB.Get(&exists, `SELECT EXISTS(SELECT 1 FROM transformed_data.mart_agenturbezirk_sociodata WHERE ba_schl = $1)`, baSchl); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if !exists {
		respondNotFound(c, "Agenturbezirk nicht gefunden")
		return
	}

	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	cfg, ok := models.DatasetConfigBySlug(slug)
	if !ok || !cfg.IsHarmonized {
		respondBadRequest(c, fmt.Sprintf("unbekannter oder nicht aggregierbarer Datensatz-Slug %q", slug))
		return
	}

	points := []models.AgenturDatasetAggregatePoint{}
	query := `SELECT ba_schl, dataset_slug, year, value, source_region_count, true AS is_computed, aggregation_method, value_kind
		FROM transformed_data.mart_agentur_dataset_aggregate
		WHERE ba_schl = $1 AND dataset_slug = $2
		ORDER BY year DESC`
	if err := h.DB.Select(&points, query, baSchl, cfg.RouteSlug); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	respondList(c, points, nil)
}

// GetAgenturbezirkDatasetAggregateYears lists the datasets aggregated at Agenturbezirk level, with their years.
// @Summary Verfügbare aggregierte Datensätze/Jahre (Agenturbezirke)
// @Description Gibt alle (Datensatz-Slug, Jahr)-Kombinationen zurück, für die mart_agentur_dataset_aggregate Werte enthält - Grundlage der Datensatz-/Jahresauswahl in der Karte.
// @Tags Agenturbezirke
// @Accept json
// @Produce json,text/csv
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregateYear}
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/dataset-aggregate [get]
func (h *Handler) GetAgenturbezirkDatasetAggregateYears(c *gin.Context) {
	rows := []models.AgenturDatasetAggregateYear{}
	query := `SELECT DISTINCT dataset_slug, year
		FROM transformed_data.mart_agentur_dataset_aggregate
		ORDER BY dataset_slug, year DESC`
	if err := h.DB.Select(&rows, query); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	respondList(c, rows, &models.MetaInfo{Total: len(rows)})
}

// GetAgenturbezirkeDatasetAggregateByYear returns the aggregated values of all Agenturbezirke for one year.
// @Summary Aggregierter Datensatz für alle Agenturbezirke (ein Jahr)
// @Description Liefert für einen Kreis-grain-Datensatz und ein Jahr den auf Agenturbezirk-Ebene aggregierten Wert je Agenturbezirk (siehe mart_agentur_dataset_aggregate) - für die Kartendarstellung. Geometrien kommen weiterhin aus GET /agenturbezirke/period. LIMITATIONEN: Summen (Zählgrößen) sind Summen der Kreis-Werte; Quoten und Durchschnitte sind ein bevölkerungsgewichteter Durchschnitt der Kreis-Werte (Werte je Erwerbstätigen: erwerbstätigengewichtet; Rückfall auf Flächengewichte, wenn die Gewichte fehlen) - eine Näherung, keine exakte Neuberechnung, da Zähler/Nenner nicht vorliegen und die Gesamtbevölkerung nur Ersatz für die eigentliche Bezugsgröße ist. Mediane lassen sich nicht aus Kreis-Medianen zusammensetzen (nur Schätzung). Berlin: ein Kreis verteilt sich auf drei Agenturbezirke, Summen werden flächenanteilig aufgeteilt. Pro Punkt kennzeichnen aggregation_method und value_kind das Verfahren.
// @Tags Agenturbezirke
// @Accept json
// @Produce json,text/csv
// @Param slug path string true "Route-Slug des Datensatzes" example(bevoelkerung)
// @Param year query int true "Jahr" example(2022)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen" example(false)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregatePoint}
// @Failure 400 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /agenturbezirke/dataset-aggregate/{slug} [get]
func (h *Handler) GetAgenturbezirkeDatasetAggregateByYear(c *gin.Context) {
	var filter struct {
		Year int `form:"year" binding:"required"`
	}
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	cfg, ok := models.DatasetConfigBySlug(slug)
	if !ok || !cfg.IsHarmonized {
		respondBadRequest(c, fmt.Sprintf("unbekannter oder nicht aggregierbarer Datensatz-Slug %q", slug))
		return
	}

	points := []models.AgenturDatasetAggregatePoint{}
	query := `SELECT ba_schl, dataset_slug, year, value, source_region_count, true AS is_computed, aggregation_method, value_kind
		FROM transformed_data.mart_agentur_dataset_aggregate
		WHERE dataset_slug = $1 AND year = $2
		ORDER BY ba_schl`
	if err := h.DB.Select(&points, query, cfg.RouteSlug, filter.Year); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	respondList(c, points, &models.MetaInfo{Total: len(points)})
}
