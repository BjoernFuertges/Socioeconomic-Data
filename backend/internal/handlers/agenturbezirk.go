package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// GetAgenturbezirke gibt alle Agenturbezirke der Bundesagentur für Arbeit zurück
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

// GetAgenturbezirkByID gibt einen einzelnen Agenturbezirk zurück
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
		if err.Error() == "sql: no rows in result set" {
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

// GetAgenturbezirkArbeitslosigkeit gibt die monatliche Arbeitslosenzahlen-Zeitreihe eines Agenturbezirks zurück
// @Summary Monatliche Arbeitslosenzahlen eines Agenturbezirks
// @Description Gibt die monatliche Zeitreihe (Arbeitslose, Unterbeschäftigung) für einen Agenturbezirk zurück (AfA Lange Zeitreihen, ab Januar 2007)
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturbezirkArbeitslosigkeitPoint}
// @Failure 404 {object} models.APIResponse
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

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    points,
	})
}

// GetAgenturbezirkeLatest gibt alle Agenturbezirke mit Geometrie und dem
// jeweils neuesten Berichtsmonat zurück - für die Kartendarstellung
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

// GetAgenturbezirkePeriods gibt alle Berichtsmonate zurück, für die
// Agenturbezirk-Arbeitslosenzahlen vorliegen - für die Jahr/Monat-Auswahl in
// der Karte (analog zur Jahres-/Versionsauswahl bei NUTS/LAU).
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

// GetAgenturbezirkeByPeriod gibt alle Agenturbezirke mit Geometrie und dem
// Wert eines bestimmten Berichtsmonats zurück - für die Kartendarstellung
// mit wählbarem Zeitpunkt (analog zur Jahresauswahl bei NUTS/LAU). Bezirke
// ohne Wert für diesen Monat fehlen in der Antwort.
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

// GetAgenturbezirkNuts3 gibt die n:m-Zuordnung eines Agenturbezirks zu
// überlappenden NUTS-3-Regionen zurück, mit Flächenanteil in beide
// Richtungen - verweist auf den ergänzenden Kreis-Level-Datensatz
// "Arbeitslose" (INKAR/BBSR).
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

// GetAgenturbezirkDatasetAggregateTimeseries gibt eine auf Agenturbezirk-Ebene
// flächenanteilsgewichtet hochaggregierte Zeitreihe eines Kreis-grain-Datensatzes zurück
// @Summary Auf Agenturbezirk-Ebene aggregierte Datensatz-Zeitreihe
// @Description Aggregiert einen Kreis-grain-Soziodata-Datensatz (siehe GET /nuts/{nuts_id}/datasets) über den Agentur<->NUTS-3-Flächenanteils-Crosswalk auf Agenturbezirk-Ebene hoch (Summe bzw. flächengewichteter Durchschnitt je nach Datensatz, siehe mart_agentur_dataset_aggregate). Analog zu GET /nuts/{nuts_id}/dataset-aggregate/{slug} für NUTS-0/1/2, aber flächen- statt bevölkerungsgewichtet.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param ba_schl path string true "Amtlicher BA-Schlüssel des Agenturbezirks" example(962)
// @Param slug path string true "Route-Slug des Datensatzes" example(bevoelkerung)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregatePoint}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
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
	query := `SELECT ba_schl, dataset_slug, year, value, source_region_count, true AS is_computed
		FROM transformed_data.mart_agentur_dataset_aggregate
		WHERE ba_schl = $1 AND dataset_slug = $2
		ORDER BY year DESC`
	if err := h.DB.Select(&points, query, baSchl, cfg.RouteSlug); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: points})
}

// GetAgenturbezirkDatasetAggregateYears listet die auf Agenturbezirk-Ebene aggregierten Datensätze mit ihren Jahren
// @Summary Verfügbare aggregierte Datensätze/Jahre (Agenturbezirke)
// @Description Gibt alle (Datensatz-Slug, Jahr)-Kombinationen zurück, für die mart_agentur_dataset_aggregate Werte enthält - Grundlage der Datensatz-/Jahresauswahl in der Karte.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregateYear}
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

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: rows, Meta: &models.MetaInfo{Total: len(rows)}})
}

// GetAgenturbezirkeDatasetAggregateByYear gibt für ein Jahr die aggregierten Werte aller Agenturbezirke zurück
// @Summary Aggregierter Datensatz für alle Agenturbezirke (ein Jahr)
// @Description Liefert für einen Kreis-grain-Datensatz und ein Jahr den auf Agenturbezirk-Ebene flächenanteilsgewichtet aggregierten Wert je Agenturbezirk (siehe mart_agentur_dataset_aggregate) - für die Kartendarstellung. Geometrien kommen weiterhin aus GET /agenturbezirke/period.
// @Tags Agenturbezirke
// @Accept json
// @Produce json
// @Param slug path string true "Route-Slug des Datensatzes" example(bevoelkerung)
// @Param year query int true "Jahr" example(2022)
// @Success 200 {object} models.APIResponse{data=[]models.AgenturDatasetAggregatePoint}
// @Failure 400 {object} models.APIResponse
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
	query := `SELECT ba_schl, dataset_slug, year, value, source_region_count, true AS is_computed
		FROM transformed_data.mart_agentur_dataset_aggregate
		WHERE dataset_slug = $1 AND year = $2
		ORDER BY ba_schl`
	if err := h.DB.Select(&points, query, cfg.RouteSlug, filter.Year); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: points, Meta: &models.MetaInfo{Total: len(points)}})
}
