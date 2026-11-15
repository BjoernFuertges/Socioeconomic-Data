package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type datasetYearRow struct {
	DatasetSlug string `db:"dataset_slug"`
	Year        int    `db:"year"`
}

func (r datasetYearRow) slugYear() (string, int) { return r.DatasetSlug, r.Year }

// regionNutsDatasets returns the available Soziodata datasets for a NUTS ID.
// For Kreise (level 3) from mart_region_datasets (Kreis grain, a single
// indexed DB access instead of N individual queries); for Land/Bundesland/
// Regierungsbezirk (levels 0-2) from mart_nuts012_dataset_aggregate, which
// already aggregates the same Kreis-grain datasets up to these levels (see
// the model comment there). Both return the same (dataset_slug, year) row
// shape, so the grouping below applies to both.
func (h *Handler) regionNutsDatasets(ctx context.Context, nutsID string, level int) ([]models.RegionDataset, error) {
	query := `SELECT dataset_slug, year FROM transformed_data.mart_region_datasets
		 WHERE geodata_id_nuts = $1 ORDER BY dataset_slug, year`
	if level != 3 {
		query = `SELECT dataset_slug, year FROM transformed_data.mart_nuts012_dataset_aggregate
		 WHERE nuts_id = $1 ORDER BY dataset_slug, year`
	}
	var rows []datasetYearRow
	if err := h.DB.SelectContext(ctx, &rows, query, nutsID); err != nil {
		return nil, err
	}

	return buildRegionDatasets(rows2years(rows)), nil
}

// regionDatasetYears maps dataset_slug -> years.
type regionDatasetYears map[string][]int

func rows2years[T interface{ slugYear() (string, int) }](rows []T) regionDatasetYears {
	out := regionDatasetYears{}
	for _, r := range rows {
		slug, year := r.slugYear()
		out[slug] = append(out[slug], year)
	}
	return out
}

// buildRegionDatasets builds the result in the order of AllSoziodataConfigs
// (RouteSlug -> table name).
func buildRegionDatasets(yearsBySlug regionDatasetYears) []models.RegionDataset {
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
	return result
}

// regionNutsDatasetsBulk is the batched variant of regionNutsDatasets for
// Kreise (level 3): one query for all IDs instead of one per region (include_datasets
// on list endpoints). IDs without datasets are missing from the map.
func (h *Handler) regionNutsDatasetsBulk(ctx context.Context, nutsIDs []string) (map[string][]models.RegionDataset, error) {
	out := map[string][]models.RegionDataset{}
	if len(nutsIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		NutsID      string `db:"geodata_id_nuts"`
		DatasetSlug string `db:"dataset_slug"`
		Year        int    `db:"year"`
	}
	if err := h.DB.SelectContext(ctx, &rows,
		`SELECT geodata_id_nuts, dataset_slug, year FROM transformed_data.mart_region_datasets
		 WHERE geodata_id_nuts = ANY($1) ORDER BY geodata_id_nuts, dataset_slug, year`, pq.Array(nutsIDs)); err != nil {
		return nil, err
	}
	byID := map[string]regionDatasetYears{}
	for _, r := range rows {
		if byID[r.NutsID] == nil {
			byID[r.NutsID] = regionDatasetYears{}
		}
		byID[r.NutsID][r.DatasetSlug] = append(byID[r.NutsID][r.DatasetSlug], r.Year)
	}
	for id, years := range byID {
		out[id] = buildRegionDatasets(years)
	}
	return out, nil
}

// logDatasetsError records a failed (optional) include_datasets lookup; the
// main response is still served, just without the embedded datasets.
func logDatasetsError(c *gin.Context, op string, err error) {
	log.Printf("include_datasets (%s) fehlgeschlagen (%s): %v", op, c.Request.URL.Path, err)
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
