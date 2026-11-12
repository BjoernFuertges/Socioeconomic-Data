package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

const datasetCatalogColumns = `dataset_slug, dataset_uri, name, description, description_long, unit,
	provenance, license, license_url, algorithm, number_type, geodata_level, geodata_year,
	min_year, max_year, total_records, updated_at, license_tracked_since::text as license_tracked_since,
	license_backfilled`

// GetDatasetCatalog returns the metadata catalog of all datasets.
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
	if err := h.DB.SelectContext(c.Request.Context(), &catalog, query, args...); err != nil {
		respondDBError(c, "Datenbankfehler beim Datensatz-Katalog", err)
		return
	}

	respondList(c, catalog, &models.MetaInfo{Total: len(catalog)})
}

// datasetCatalogQuery builds the catalog query; with empty slugs there is no
// WHERE filter, otherwise dataset_slug = ANY(...).
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

// GetDatasetCatalogEntry returns the metadata catalog entry of a single dataset.
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
	if err := h.DB.GetContext(c.Request.Context(), &entry, query, slug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(c, fmt.Sprintf("Datensatz %q nicht im Katalog gefunden", slug))
			return
		}
		respondDBError(c, "Datenbankfehler beim Datensatz-Katalog", err)
		return
	}

	// CSV expects a slice of structs (one row per element) - here always exactly one.
	respondNegotiated(c, entry, []models.DatasetCatalogEintrag{entry}, nil)
}

// GetDatasetSource returns the source data of a dataset.
// @Summary Quelldaten eines Datensatzes (gemappt, unverändert)
// @Description Liefert die Werte des Datensatzes so, wie sie aus dem geo-mapper-Teilprojekt kommen: unverändert, nur die
// @Description Regionsnamen sind auf Geodaten-IDs gemappt (original_*, geodata_id_*, mapper, parameter). Keine NUTS-Harmonisierung,
// @Description Interpolation oder Aggregation - dafür die Datensatz-Endpunkte verwenden. Herkunft und Lizenz: GET /datasets/{slug}.
// @Description Nicht verfügbar für abgeleitete Slugs (beschaeftigungsquote-nuts3, lau_*, ba_*) und die Agenturbezirks-Arbeitslosenzahlen (siehe /agenturbezirke).
// @Description Format per Accept-Header (application/json oder text/csv) oder ?format=csv|json; ?download=1 setzt Content-Disposition.
// @Tags Datensatz-Katalog
// @Accept json
// @Produce json,text/csv
// @Param slug path string true "Datensatz-Slug (z.B. bevoelkerung)" example(bevoelkerung)
// @Param format query string false "Überschreibt den Accept-Header" Enums(json, csv)
// @Param download query bool false "Content-Disposition: attachment setzen"
// @Success 200 {object} models.APIResponse{data=[]models.DatasetQuellwert}
// @Failure 404 {object} models.APIResponse
// @Failure 406 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /datasets/{slug}/source [get]
func (h *Handler) GetDatasetSource(c *gin.Context) {
	slug := strings.ToLower(c.Param("slug"))

	rows := make([]models.DatasetQuellwert, 0)
	err := h.DB.SelectContext(c.Request.Context(), &rows, datasetSourceQuery, slug)
	if err != nil {
		respondDBError(c, "Datenbankfehler bei den Quelldaten", err)
		return
	}
	if len(rows) == 0 {
		respondNotFound(c, fmt.Sprintf("Keine Quelldaten für Datensatz %q gefunden", slug))
		return
	}

	respondList(c, rows, &models.MetaInfo{Total: len(rows)})
}

const datasetSourceQuery = `SELECT dataset_slug, original_id_1, original_name, geodata_name,
	geodata_id_nuts, geodata_id_lau, geodata_id_ars, mapper, parameter,
	geodata_level, geodata_year, year, value::float8 AS value
	FROM transformed_data.mart_dataset_source
	WHERE dataset_slug = $1
	ORDER BY original_id_1, year`
