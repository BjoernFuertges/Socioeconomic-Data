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
