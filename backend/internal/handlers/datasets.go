package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// regionNutsDatasets returns the available Soziodata datasets for a NUTS ID.
// For Kreise (level 3) from mart_region_datasets (Kreis grain, a single
// indexed DB access instead of N individual queries); for Land/Bundesland/
// Regierungsbezirk (levels 0-2) from mart_nuts012_dataset_aggregate, which
// already aggregates the same Kreis-grain datasets up to these levels (see
// the model comment there). Both return the same (dataset_slug, year) row
// shape, so the grouping below applies to both.
func (h *Handler) regionNutsDatasets(ctx context.Context, nutsID string, level int) ([]models.RegionDataset, error) {
	type row struct {
		DatasetSlug string `db:"dataset_slug"`
		Year        int    `db:"year"`
	}
	query := `SELECT dataset_slug, year FROM transformed_data.mart_region_datasets
		 WHERE geodata_id_nuts = $1 ORDER BY dataset_slug, year`
	if level != 3 {
		query = `SELECT dataset_slug, year FROM transformed_data.mart_nuts012_dataset_aggregate
		 WHERE nuts_id = $1 ORDER BY dataset_slug, year`
	}
	var rows []row
	if err := h.DB.SelectContext(ctx, &rows, query, nutsID); err != nil {
		return nil, err
	}

	// Group years per slug
	yearsBySlug := make(map[string][]int, len(rows))
	for _, r := range rows {
		yearsBySlug[r.DatasetSlug] = append(yearsBySlug[r.DatasetSlug], r.Year)
	}

	// Build the result in the order of AllSoziodataConfigs (RouteSlug -> table name)
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

// GetNutsDatasets returns the available Soziodata datasets for a NUTS region.
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

	datasets, err := h.regionNutsDatasets(c.Request.Context(), levelDetails.NormalizedID, levelDetails.Number)
	if err != nil {
		respondDBError(c, "Datenbankfehler", err)
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

// GetLAUDatasets returns the available Soziodata datasets for a LAU region.
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

	// nuts3_id is the canonical LAU->NUTS-3 link (declared source column, else
	// spatially derived fallback - see int_lau_nuts3_spatial_link.sql /
	// lau_integrated.sql), more robust than the often-NULL raw nuts_3_code
	// column.
	var nuts3Code string
	err := h.DB.GetContext(c.Request.Context(), &nuts3Code,
		`SELECT nuts3_id FROM transformed_data.lau_integrated WHERE gisco_id = $1 AND nuts3_id IS NOT NULL AND lau_name IS NOT NULL LIMIT 1`,
		giscoID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(c, "LAU-Region nicht gefunden oder kein NUTS-3-Code vorhanden")
			return
		}
		respondDBError(c, "Datenbankfehler", err)
		return
	}

	datasets, err := h.regionNutsDatasets(c.Request.Context(), nuts3Code, 3)
	if err != nil {
		respondDBError(c, "Datenbankfehler", err)
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
