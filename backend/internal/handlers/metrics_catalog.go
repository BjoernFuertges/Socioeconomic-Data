package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// GetDerivedMetricsCatalog returns all derived-metric definitions.
// @Summary Abgeleitete Metriken-Definitionen
// @Description Gibt alle abgeleiteten Metriken aus dem Katalog zurück (z.B. Bevölkerungsdichte,
// @Description eigenberechnete Arbeitslosenquote). Jede Metrik wird per Formel aus mehreren Datensätzen berechnet.
// @Description Mit GET /metrics/derived/:slug können die statistischen Werte abgerufen werden.
// @Description meta.note: Es gehen nur gültige Werte ein; Zähler und Nenner müssen gültig sein.
// @Tags Metriken
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]models.DerivedMetricEntry}
// @Router /metrics/derived [get]
func (h *Handler) GetDerivedMetricsCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    h.DerivedCatalog,
		Meta:    &models.MetaInfo{Note: metricsValidityNote},
	})
}

// GetDerivedMetricsCatalogEntry computes statistics for a derived metric.
// @Summary Statistiken für eine abgeleitete Metrik
// @Description Berechnet Min, Max, Mittelwert, Median, Stddev und YoY-Veränderung für eine
// @Description abgeleitete Metrik (z.B. Bevölkerungsdichte = Bevölkerung / Fläche in km²).
// @Description Sind alle beteiligten Datensätze harmonisiert, werden Zähler und Nenner aus mart_nuts3_sociodata_harmonized im
// @Description NUTS-3-Stand 'nuts_version' gelesen (Default 2024) und die Fläche aus der Geometrie desselben Stands; die Antwort nennt ihn in nuts_version.
// @Description Nur Regionen mit gültigem Zähler und Nenner gehen ein. Ausgeschlossene Werte zählen excluded_invalid_count (ungültiger Zähler/Nenner oder Nenner 0),
// @Description regions_without_geometry (keine Geometrie für den Flächennenner) und regions_without_denominator (kein Nennerwert) je Jahr; excluded_regions nennt die betroffenen Regionen insgesamt.
// @Tags Metriken
// @Accept json
// @Produce json
// @Param slug path string true "Slug der abgeleiteten Metrik" example(bevoelkerungsdichte)
// @Param year query int false "Einzelnes Jahr filtern" example(2022)
// @Param from_year query int false "Jahresbereich ab" example(2018)
// @Param to_year query int false "Jahresbereich bis" example(2022)
// @Param nuts_parent_id query string false "Auf NUTS-Präfix einschränken (z.B. DE1)" example(DE1)
// @Param geodata_id_nuts query string false "NUTS-ID einer Region für Regionalwert und Ranking (im Stand nuts_version)" example(DEF01)
// @Param nuts_version query int false "NUTS-3-Zielversion der harmonisierten Werte" Enums(2003,2006,2010,2013,2016,2021,2024) default(2024)
// @Success 200 {object} models.APIResponse{data=models.DatasetMetricsResult}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Failure 503 {object} models.APIResponse
// @Router /metrics/derived/{slug} [get]
func (h *Handler) GetDerivedMetricsCatalogEntry(c *gin.Context) {
	slug := c.Param("slug")

	entry, ok := h.DerivedCatalog.FindBySlug(slug)
	if !ok {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Abgeleitete Metrik %q nicht gefunden", slug),
		})
		return
	}

	var filter models.MetricsFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}
	nutsVersion, err := resolveMetricsNutsVersion(filter)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	src, err := derivedMetricSource(entry, nutsVersion)
	if err != nil {
		// Catalog misconfiguration (configs/metrics_catalog.yaml), no DB involved.
		respondInternalError(c, fmt.Sprintf("Fehler bei abgeleiteter Metrik %q: %v", slug, err))
		return
	}

	result, err := h.computeMetrics(c.Request.Context(), src, filter)
	if err != nil {
		respondDBError(c, fmt.Sprintf("Fehler bei abgeleiteter Metrik %q", slug), err)
		return
	}
	result.Dataset = entry.Slug
	result.Column = entry.Unit

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
		Meta:    &models.MetaInfo{Note: metricsValidityNote},
	})
}

// columnAllowed checks whether a column is allowed in a dataset's ValueColumns.
// Prevents injection of arbitrary column names into SQL queries.
func columnAllowed(cfg models.KreisDatasetConfig, col string) bool {
	for _, c := range cfg.ValueColumns {
		if c == col {
			return true
		}
	}
	return false
}

// derivedMetricSource builds the row source (geodata_id_nuts, year, value) of
// a derived metric. Currently supports type=ratio (numerator/denominator x
// Scale); denominator sources: dataset (RouteSlug + column) or the builtin
// "nuts_geometry_area_sqkm". Errors are catalog misconfigurations.
//
// If every dataset involved is available in mart_nuts3_sociodata_harmonized,
// numerator and denominator are read there for nutsVersion and the area is
// that of the same NUTS-3 version (geometry vintage nutsVersion), so all parts
// refer to the same region boundaries. Otherwise the previous, unharmonized
// staging join is used (NutsVersion of the source stays nil).
func derivedMetricSource(entry models.DerivedMetricEntry, nutsVersion int) (metricsSource, error) {
	if entry.Formula.Type != "ratio" {
		return metricsSource{}, fmt.Errorf("unbekannter Formeltyp: %q (unterstützt: ratio)", entry.Formula.Type)
	}

	// Validate numerator
	numCfg, ok := models.DatasetConfigBySlug(entry.Formula.Numerator.Dataset)
	if !ok {
		return metricsSource{}, fmt.Errorf("unbekannter Datensatz im Zähler: %q", entry.Formula.Numerator.Dataset)
	}
	if !columnAllowed(numCfg, entry.Formula.Numerator.Column) {
		return metricsSource{}, fmt.Errorf("unbekannte Spalte im Zähler: %q", entry.Formula.Numerator.Column)
	}

	// Validate denominator
	var denCfg models.KreisDatasetConfig
	areaDenominator := false
	switch {
	case entry.Formula.Denominator.Builtin == "nuts_geometry_area_sqkm":
		areaDenominator = true
	case entry.Formula.Denominator.Dataset != "":
		denCfg, ok = models.DatasetConfigBySlug(entry.Formula.Denominator.Dataset)
		if !ok {
			return metricsSource{}, fmt.Errorf("unbekannter Datensatz im Nenner: %q", entry.Formula.Denominator.Dataset)
		}
		if !columnAllowed(denCfg, entry.Formula.Denominator.Column) {
			return metricsSource{}, fmt.Errorf("unbekannte Spalte im Nenner: %q", entry.Formula.Denominator.Column)
		}
	default:
		return metricsSource{}, fmt.Errorf("keine gültige Nenner-Quelle definiert (dataset oder builtin erforderlich)")
	}

	scale := entry.Formula.Scale
	if scale == 0 {
		scale = 1.0
	}
	scaleLiteral := strconv.FormatFloat(scale, 'f', -1, 64)

	harmonized := harmonizedMetricsColumn(numCfg, entry.Formula.Numerator.Column) &&
		(areaDenominator || harmonizedMetricsColumn(denCfg, entry.Formula.Denominator.Column))
	if harmonized {
		return harmonizedDerivedMetricSource(numCfg, denCfg, areaDenominator, scaleLiteral, nutsVersion), nil
	}
	return stagingDerivedMetricSource(numCfg, entry.Formula.Numerator.Column, denCfg, entry.Formula.Denominator.Column, areaDenominator, scaleLiteral), nil
}

// derivedExcludedReasonSQL classifies a numerator row (alias n) against its
// LEFT JOINed denominator (alias dc, present when dcPresent is true):
// missing denominator row/geometry first, then validity of numerator and
// denominator, then a zero denominator (value NULL). NULL means "used".
func derivedExcludedReasonSQL(dcPresent, numValid, denValid, valueExpr string, areaDenominator bool) string {
	missing := metricsExcludedNoDenominator
	if areaDenominator {
		missing = metricsExcludedNoGeometry
	}
	return fmt.Sprintf(`CASE WHEN NOT (%s) THEN '%s'
			WHEN NOT (%s) OR NOT (%s) OR %s IS NULL THEN '%s'
		END`, dcPresent, missing, numValid, denValid, valueExpr, metricsExcludedInvalid)
}

// harmonizedDerivedMetricSource joins numerator and denominator on
// (target_nuts_id, year) within one target_version. The area comes from the
// geometry period that contains vintage nutsVersion (valid_from <= v <=
// max_vintage_year: the region exists in exactly that vintage, so the match is
// unique), i.e. the boundaries the harmonized values refer to. The joins are
// LEFT JOINs: numerator regions without denominator/geometry are kept and
// marked (excluded_reason) instead of silently disappearing; numerator and
// denominator must both have has_valid_value.
func harmonizedDerivedMetricSource(numCfg, denCfg models.KreisDatasetConfig, areaDenominator bool, scaleLiteral string, nutsVersion int) metricsSource {
	args := []interface{}{numCfg.RouteSlug, nutsVersion}
	var join, denomExpr, dcPresent, denValid string
	if areaDenominator {
		join = `LEFT JOIN (
			SELECT nuts_id, ST_Area(geometry_3035) / 1e6 AS area_sqkm
			FROM transformed_data.mart_nuts_geometry_historized
			WHERE levl_code = 3 AND has_valid_geometry
			  AND valid_from <= $2::int AND max_vintage_year >= $2::int
		) dc ON dc.nuts_id = n.target_nuts_id`
		denomExpr = `dc.area_sqkm`
		dcPresent = `dc.nuts_id IS NOT NULL`
		denValid = `true`
	} else {
		join = `LEFT JOIN transformed_data.mart_nuts3_sociodata_harmonized dc
			ON dc.target_nuts_id = n.target_nuts_id AND dc.year = n.year
			AND dc.target_version = n.target_version AND dc.dataset_slug = $3`
		denomExpr = `dc.harmonized_value::float8`
		dcPresent = `dc.target_nuts_id IS NOT NULL`
		denValid = `dc.has_valid_value`
		args = append(args, denCfg.RouteSlug)
	}
	valueExpr := fmt.Sprintf(`((n.harmonized_value::float8 / NULLIF(%s, 0)) * %s)`, denomExpr, scaleLiteral)
	v := nutsVersion
	return metricsSource{
		SQL: fmt.Sprintf(`(SELECT n.target_nuts_id AS geodata_id_nuts, n.year,
			%s AS value,
			%s AS excluded_reason
			FROM transformed_data.mart_nuts3_sociodata_harmonized n
			%s
			WHERE n.dataset_slug = $1 AND n.target_version = $2::int) src`,
			valueExpr, derivedExcludedReasonSQL(dcPresent, "n.has_valid_value", denValid, valueExpr, areaDenominator), join),
		Args:        args,
		NutsVersion: &v,
	}
}

// stagingDerivedMetricSource is the unharmonized computation on the staging
// tables (area of the geometry period that contains the data year), with the
// same LEFT JOIN/excluded_reason handling as harmonizedDerivedMetricSource.
// Columns are validated against ValueColumns by the caller.
func stagingDerivedMetricSource(numCfg models.KreisDatasetConfig, numCol string, denCfg models.KreisDatasetConfig, denCol string, areaDenominator bool, scaleLiteral string) metricsSource {
	var join, denomExpr, dcPresent, denValid string
	if areaDenominator {
		join = `LEFT JOIN (
			SELECT nuts_id, valid_from, valid_to, ST_Area(geometry_3035) / 1e6 AS area_sqkm
			FROM transformed_data.mart_nuts_geometry_historized
			WHERE has_valid_geometry
		) dc ON n.geodata_id_nuts = dc.nuts_id AND n.year >= dc.valid_from AND (dc.valid_to IS NULL OR n.year <= dc.valid_to)`
		denomExpr = `dc.area_sqkm`
		dcPresent = `dc.nuts_id IS NOT NULL`
		denValid = `true`
	} else {
		join = fmt.Sprintf(
			`LEFT JOIN %s.%s dc ON n.geodata_id_nuts = dc.geodata_id_nuts AND n.year = dc.year`,
			denCfg.Schema, denCfg.Table,
		)
		denomExpr = fmt.Sprintf(`dc."%s"`, denCol)
		dcPresent = `dc.geodata_id_nuts IS NOT NULL`
		denValid = `dc.has_valid_value`
	}
	valueExpr := fmt.Sprintf(`((n."%s" / NULLIF(%s, 0)) * %s)`, numCol, denomExpr, scaleLiteral)
	return metricsSource{
		SQL: fmt.Sprintf(`(SELECT n.geodata_id_nuts, n.year, %s AS value,
			%s AS excluded_reason
			FROM %s.%s n
			%s) src`,
			valueExpr, derivedExcludedReasonSQL(dcPresent, "n.has_valid_value", denValid, valueExpr, areaDenominator),
			numCfg.Schema, numCfg.Table, join),
	}
}
