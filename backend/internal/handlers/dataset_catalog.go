package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

const datasetCatalogColumns = `dataset_slug, dataset_uri, name, description, description_long, unit,
	provenance, license, license_url, algorithm, number_type, geodata_level, geodata_year,
	min_year, max_year, total_records, updated_at, license_tracked_since::text as license_tracked_since,
	license_backfilled`

// GetDatasetCatalog gibt den Metadaten-Katalog aller Datensätze zurück.
// @Summary Datensatz-Katalog (Herkunft, Lizenz, Beschreibung)
// @Description Liefert pro Datensatz Herkunft, Lizenz, Beschreibung, Einheit und Abdeckungszeitraum
// @Description sowie eine persistente URI. Lizenz ist "unbekannt", solange die tatsächlichen Nutzungsbedingungen der Quelle noch nicht recherchiert wurden.
// @Tags Datensatz-Katalog
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.DatasetCatalogEintrag}
// @Failure 500 {object} models.APIResponse
// @Router /datasets [get]
func (h *Handler) GetDatasetCatalog(c *gin.Context) {
	query := fmt.Sprintf(
		`SELECT %s FROM transformed_data.mart_dataset_catalog ORDER BY dataset_slug`,
		datasetCatalogColumns,
	)

	catalog := make([]models.DatasetCatalogEintrag, 0)
	if err := h.DB.Select(&catalog, query); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler beim Datensatz-Katalog: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    catalog,
		Meta: &models.MetaInfo{
			Total: len(catalog),
		},
	})
}

// GetDatasetCatalogEntry gibt den Metadaten-Katalogeintrag eines einzelnen Datensatzes zurück.
// @Summary Katalogeintrag eines einzelnen Datensatzes
// @Description Liefert Herkunft, Lizenz, Beschreibung, Einheit und Abdeckungszeitraum für einen Datensatz-Slug.
// @Tags Datensatz-Katalog
// @Accept json
// @Produce json
// @Param slug path string true "Datensatz-Slug (z.B. bevoelkerung)" example(bevoelkerung)
// @Success 200 {object} models.APIResponse{data=models.DatasetCatalogEintrag}
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /datasets/{slug} [get]
func (h *Handler) GetDatasetCatalogEntry(c *gin.Context) {
	slug := c.Param("slug")

	query := fmt.Sprintf(
		`SELECT %s FROM transformed_data.mart_dataset_catalog WHERE dataset_slug = $1`,
		datasetCatalogColumns,
	)

	var entry models.DatasetCatalogEintrag
	if err := h.DB.Get(&entry, query, slug); err != nil {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Datensatz %q nicht im Katalog gefunden", slug),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    entry,
	})
}
