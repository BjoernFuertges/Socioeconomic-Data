package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
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
// @Description ?datasets= filtert auf einzelne Slugs (kommagetrennt, leer = alle) - gedacht für den
// @Description gleichzeitigen Metadaten-Export neben einem Daten-Download
// @Description Format per Accept-Header (application/json oder text/csv) oder ?format=csv|json; ?download=1 setzt Content-Disposition.
// @Tags Datensatz-Katalog
// @Accept json
// @Produce json,text/csv
// @Param datasets query string false "Kommagetrennte Datensatz-Slugs (leer = alle)" example(bevoelkerung,arbeitslose)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen"
// @Success 200 {object} models.APIResponse{data=[]models.DatasetCatalogEintrag}
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /datasets [get]
func (h *Handler) GetDatasetCatalog(c *gin.Context) {
	slugs := parseDatasetList(c.Query("datasets"))
	query, args := datasetCatalogQuery(slugs)

	catalog := make([]models.DatasetCatalogEintrag, 0)
	if err := h.DB.Select(&catalog, query, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler beim Datensatz-Katalog: %v", err))
		return
	}

	respondList(c, catalog, &models.MetaInfo{Total: len(catalog)})
}

// datasetCatalogQuery baut die Katalog-Abfrage; ohne slugs (leer) kein
// WHERE-Filter, sonst dataset_slug = ANY(...).
func datasetCatalogQuery(slugs []string) (string, []interface{}) {
	query := fmt.Sprintf(`SELECT %s FROM transformed_data.mart_dataset_catalog`, datasetCatalogColumns)
	args := []interface{}{}
	if len(slugs) > 0 {
		args = append(args, pq.Array(slugs))
		query += " WHERE dataset_slug = ANY($1)"
	}
	query += " ORDER BY dataset_slug"
	return query, args
}

// GetDatasetCatalogEntry gibt den Metadaten-Katalogeintrag eines einzelnen Datensatzes zurück.
// @Summary Katalogeintrag eines einzelnen Datensatzes
// @Description Liefert Herkunft, Lizenz, Beschreibung, Einheit und Abdeckungszeitraum für einen Datensatz-Slug.
// @Description Format per Accept-Header (application/json oder text/csv) oder ?format=csv|json; ?download=1 setzt Content-Disposition.
// @Tags Datensatz-Katalog
// @Accept json
// @Produce json,text/csv
// @Param slug path string true "Datensatz-Slug (z.B. bevoelkerung)" example(bevoelkerung)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen"
// @Success 200 {object} models.APIResponse{data=models.DatasetCatalogEintrag}
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
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
		respondNotFound(c, fmt.Sprintf("Datensatz %q nicht im Katalog gefunden", slug))
		return
	}

	// CSV erwartet eine Slice von Structs (eine Zeile pro Element) - hier immer genau eine.
	respondNegotiated(c, entry, []models.DatasetCatalogEintrag{entry}, nil)
}
