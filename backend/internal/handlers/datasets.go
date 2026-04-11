package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// regionNutsDatasets gibt die verfügbaren Soziodata-Datensätze für eine NUTS-ID zurück.
// Nutzt mart_region_datasets für einen einzelnen indizierten DB-Zugriff statt N Einzelabfragen.
func (h *Handler) regionNutsDatasets(nutsID string) ([]models.RegionDataset, error) {
	type row struct {
		DatasetSlug string `db:"dataset_slug"`
		Year        int    `db:"year"`
	}
	var rows []row
	if err := h.DB.Select(&rows,
		`SELECT dataset_slug, year FROM transformed_data.mart_region_datasets
		 WHERE geodata_id_nuts = $1 ORDER BY dataset_slug, year`,
		nutsID,
	); err != nil {
		return nil, err
	}

	// Jahre je Slug gruppieren
	yearsBySlug := make(map[string][]int, len(rows))
	for _, r := range rows {
		yearsBySlug[r.DatasetSlug] = append(yearsBySlug[r.DatasetSlug], r.Year)
	}

	// Ergebnis in der Reihenfolge von AllSoziodataConfigs aufbauen (RouteSlug → Table-Name)
	result := make([]models.RegionDataset, 0, len(yearsBySlug))
	for _, cfg := range models.AllSoziodataConfigs() {
		years, ok := yearsBySlug[cfg.RouteSlug]
		if !ok {
			continue
		}
		result = append(result, models.RegionDataset{
			Dataset:        cfg.Table,
			RouteSlug:      cfg.RouteSlug,
			AvailableYears: years,
		})
	}
	return result, nil
}

// GetNutsDatasets gibt verfügbare Soziodata-Datensätze für eine NUTS-Region zurück.
// @Summary Verfügbare Datensätze für eine NUTS-Region
// @Description Gibt alle Soziodata-Datensätze zurück, für die Daten zu dieser NUTS-Region vorhanden sind
// @Tags NUTS
// @Accept json
// @Produce json
// @Param nuts_id path string true "NUTS-Code der Region" example(DEF01)
// @Success 200 {object} models.APIResponse{data=models.RegionDatasets}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /nuts/{nuts_id}/datasets [get]
func (h *Handler) GetNutsDatasets(c *gin.Context) {
	nutsID := c.Param("nuts_id")
	levelDetails, err := nutsLevelDetailsFromID(nutsID)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	datasets, err := h.regionNutsDatasets(levelDetails.NormalizedID)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: models.RegionDatasets{
			GeoID:    levelDetails.NormalizedID,
			GeoType:  "NUTS",
			Datasets: datasets,
		},
	})
}

// GetLAUDatasets gibt verfügbare Soziodata-Datensätze für eine LAU-Region zurück.
// @Summary Verfügbare Datensätze für eine LAU-Region
// @Description Gibt alle Soziodata-Datensätze zurück, für die Daten zu dieser LAU-Region vorhanden sind (via NUTS-3-Code)
// @Tags LAU
// @Accept json
// @Produce json
// @Param gisco_id path string true "GISCO-ID der LAU-Region" example(DE111000)
// @Success 200 {object} models.APIResponse{data=models.RegionDatasets}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /lau/{gisco_id}/datasets [get]
func (h *Handler) GetLAUDatasets(c *gin.Context) {
	giscoID := strings.ToUpper(strings.TrimSpace(c.Param("gisco_id")))

	var nuts3Code string
	err := h.DB.Get(&nuts3Code,
		`SELECT nuts_3_code FROM transformed_data.lau_integrated WHERE gisco_id = $1 AND nuts_3_code IS NOT NULL LIMIT 1`,
		giscoID,
	)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			respondNotFound(c, "LAU-Region nicht gefunden oder kein NUTS-3-Code vorhanden")
			return
		}
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	datasets, err := h.regionNutsDatasets(nuts3Code)
	if err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: models.RegionDatasets{
			GeoID:    giscoID,
			GeoType:  "LAU",
			Datasets: datasets,
		},
	})
}
