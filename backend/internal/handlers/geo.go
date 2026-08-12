package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

func normalizeGeometryStrategy(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "latest":
		return "latest", nil
	case "exact":
		return "exact", nil
	default:
		return "", fmt.Errorf("ungültiger geometry_strategy: erlaubt sind latest oder exact")
	}
}

func normalizeGeoType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return "auto", nil
	case "nuts":
		return "nuts", nil
	case "lau":
		return "lau", nil
	default:
		return "", fmt.Errorf("ungültiger geo_type: erlaubt sind auto, nuts oder lau")
	}
}

func detectGeoType(geoID string) string {
	trimmed := strings.TrimSpace(geoID)
	if _, err := nutsLevelDetailsFromID(trimmed); err == nil {
		return "nuts"
	}
	return "lau"
}

func normalizeSpatialOperation(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "intersects":
		return "intersects", nil
	case "contains":
		return "contains", nil
	default:
		return "", fmt.Errorf("ungültige operation: erlaubt sind intersects oder contains")
	}
}

func extractGeoJSONGeometry(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("geometry ist erforderlich")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("ungültiges GeoJSON: %v", err)
	}

	typeRaw, ok := payload["type"].(string)
	if !ok {
		return "", fmt.Errorf("GeoJSON muss ein Feld 'type' enthalten")
	}

	typeName := strings.ToLower(strings.TrimSpace(typeRaw))
	switch typeName {
	case "feature":
		geom, ok := payload["geometry"]
		if !ok || geom == nil {
			return "", fmt.Errorf("GeoJSON Feature enthält keine geometry")
		}
		b, err := json.Marshal(geom)
		if err != nil {
			return "", fmt.Errorf("GeoJSON geometry konnte nicht serialisiert werden: %v", err)
		}
		return string(b), nil
	case "featurecollection":
		features, ok := payload["features"].([]interface{})
		if !ok || len(features) == 0 {
			return "", fmt.Errorf("GeoJSON FeatureCollection enthält keine Features")
		}
		if len(features) != 1 {
			return "", fmt.Errorf("FeatureCollection mit mehreren Features wird nicht unterstützt")
		}
		feature, ok := features[0].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("ungültiges Feature in FeatureCollection")
		}
		geom, ok := feature["geometry"]
		if !ok || geom == nil {
			return "", fmt.Errorf("Feature enthält keine geometry")
		}
		b, err := json.Marshal(geom)
		if err != nil {
			return "", fmt.Errorf("GeoJSON geometry konnte nicht serialisiert werden: %v", err)
		}
		return string(b), nil
	default:
		b, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("GeoJSON konnte nicht serialisiert werden: %v", err)
		}
		return string(b), nil
	}
}

func spatialPredicateSQL(operation string, geomColumn string, geoJSONParam int) string {
	if operation == "contains" {
		return fmt.Sprintf("ST_Contains(%s, ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326), 3035))", geomColumn, geoJSONParam)
	}
	return fmt.Sprintf("ST_Intersects(%s, ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326), 3035))", geomColumn, geoJSONParam)
}

// GetGeoRegions gibt eine einheitliche Liste von NUTS- oder LAU-Regionen zurück
// @Summary Liste allgemeiner Geo-Regionen
// @Description Gibt eine einheitliche Liste für NUTS- oder LAU-Regionen zurück. Standard ist NUTS. Mit geo_type=lau werden LAU-Regionen geliefert.
// @Tags Geo
// @Accept json
// @Produce json
// @Param geo_type query string false "Regionstyp: nuts oder lau" Enums(nuts,lau) default(nuts)
// @Param level query int false "NUTS-Level (nur für geo_type=nuts, 0-3)" Enums(0,1,2,3) example(3)
// @Param country_code query string false "Ländercode (nur für geo_type=lau)" example(DE)
// @Param nuts_parent_id query string false "NUTS-Präfix-Filter (z.B. DE1)" example(DE1)
// @Param geodata_name_contains query string false "Teilstring-Suche nach Regionenamen" example(berlin)
// @Param point query string false "Punktabfrage im Format lon,lat (nur für geo_type=lau)" example(13.404954,52.520008)
// @Param include_geometry query bool false "Geometrie einschließen, wenn verfügbar" example(false)
// @Param geometry_strategy query string false "Geometrieauswahl (nur für geo_type=nuts): latest oder exact" Enums(latest,exact) default(latest)
// @Param geometry_year query int false "Zieljahr für Geometrie (nur für geo_type=nuts; bei exact erforderlich)" example(2024)
// @Param year query int false "Jahr (optional)" example(2024)
// @Param limit query int false "Anzahl der Ergebnisse (1-1000)" default(100) example(100)
// @Param offset query int false "Offset für Pagination" default(0) example(0)
// @Success 200 {object} models.APIResponse{data=[]models.GeoRegionSummary}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /geo [get]
func (h *Handler) GetGeoRegions(c *gin.Context) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.GeoRegionFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	geoType, err := normalizeGeoType(filter.GeoType)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if geoType == "auto" {
		geoType = "nuts"
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	geometryStrategy, err := normalizeGeometryStrategy(filter.GeometryStrategy)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	filter.GeometryStrategy = geometryStrategy
	if filter.IncludeGeometry && filter.GeometryStrategy == "exact" && filter.GeometryYear == nil {
		respondBadRequest(c, "geometry_year ist erforderlich, wenn geometry_strategy=exact gesetzt ist")
		return
	}

	if geoType == "lau" {
		h.getGeoRegionsLAU(c, filter, limitNull)
		return
	}

	h.getGeoRegionsNUTS(c, filter, limitNull)
}

func (h *Handler) getGeoRegionsLAU(c *gin.Context, filter models.GeoRegionFilter, limitNull bool) {
	geometrySelect := `'null'::json AS geometry_wgs84, null::boolean AS has_valid_geometry`
	if filter.IncludeGeometry {
		geometrySelect = `geometry_wgs84, has_valid_geometry`
	}

	query := `SELECT DISTINCT ON (gisco_id)
		gisco_id AS geo_id,
		'LAU' AS geo_type,
		lau_name AS region_name,
		country_code,
		` + geometrySelect + `,
		year
	FROM transformed_data.lau_integrated
	WHERE lau_name IS NOT NULL`

	args := []interface{}{}
	argPos := 1

	if filter.CountryCode != "" {
		query += fmt.Sprintf(" AND country_code = $%d", argPos)
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.CountryCode)))
		argPos++
	}
	if filter.NutsParentID != "" {
		query += fmt.Sprintf(" AND nuts_3_code IS NOT NULL AND nuts_3_code LIKE $%d", argPos)
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.NutsParentID))+"%")
		argPos++
	}
	if filter.GeodataNameContains != "" {
		query += fmt.Sprintf(" AND lau_name ILIKE $%d", argPos)
		args = append(args, "%"+strings.TrimSpace(filter.GeodataNameContains)+"%")
		argPos++
	}
	if strings.TrimSpace(filter.Point) != "" {
		lon, lat, err := parseLonLatPoint(filter.Point)
		if err != nil {
			respondBadRequest(c, err.Error())
			return
		}
		query += fmt.Sprintf(" AND has_valid_geometry AND ST_Contains(geometry_3035, ST_Transform(ST_SetSRID(ST_Point($%d, $%d), 4326), 3035))", argPos, argPos+1)
		args = append(args, lon, lat)
		argPos += 2
	}
	yearFilterApplied := false
	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
		yearFilterApplied = true
	} else if filter.IncludeGeometry && filter.GeometryYear != nil {
		// Wenn kein Datenjahr gesetzt ist, aber eine explizite Geometrieversion
		// angefordert wird, wird geometry_year als Jahresfilter auf lau_integrated
		// verwendet (year-Spalte entspricht dort dem Vintage-Jahr der Geometrie).
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.GeometryYear)
		argPos++
		yearFilterApplied = true
	}
	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND year >= $%d", argPos)
		args = append(args, *filter.FromYear)
		argPos++
		yearFilterApplied = true
	}
	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND year <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
		yearFilterApplied = true
	}
	if !yearFilterApplied {
		// Ohne jeglichen Jahresfilter nur den aktuellsten Vintage-Jahrgang
		// berücksichtigen. lau_integrated hat (anders als die valid_from/
		// valid_to-Perioden in mart_lau_geometry_historized) keinen Konzept
		// von "noch aktuell" - DISTINCT ON (gisco_id) ... ORDER BY year DESC
		// würde sonst auch stillgelegte/fusionierte gisco_ids mit ihrer
		// letzten bekannten (alten) Zeile durchlassen, doppelt neben ihrem
		// Nachfolgecode (analog zum NUTS-valid_to-Bug).
		query += " AND year = (SELECT MAX(year) FROM transformed_data.lau_integrated)"
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.Get(&total, countQuery, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"gisco_id, year DESC",
		100,
		filter.Limit,
		filter.Offset,
		limitNull,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	type row struct {
		GeoID            string          `db:"geo_id"`
		GeoType          string          `db:"geo_type"`
		RegionName       string          `db:"region_name"`
		Country          string          `db:"country_code"`
		GeometryWGS84    json.RawMessage `db:"geometry_wgs84"`
		HasValidGeometry *bool           `db:"has_valid_geometry"`
		Year             int             `db:"year"`
	}

	rows := make([]row, 0)
	if err := h.DB.Select(&rows, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	data := make([]models.GeoRegionSummary, 0, len(rows))
	for _, r := range rows {
		country := r.Country
		data = append(data, models.GeoRegionSummary{
			GeoID:            r.GeoID,
			GeoType:          r.GeoType,
			RegionName:       r.RegionName,
			CountryCode:      &country,
			GeometryWGS84:    r.GeometryWGS84,
			HasValidGeometry: r.HasValidGeometry,
			Year:             r.Year,
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

func (h *Handler) getGeoRegionsNUTS(c *gin.Context, filter models.GeoRegionFilter, limitNull bool) {
	argPos := 1
	args := make([]interface{}, 0)

	geometrySelect := `'null'::json AS geometry_wgs84, null::boolean AS has_valid_geometry`
	if filter.IncludeGeometry {
		geometrySelect = `geometry_wgs84, has_valid_geometry`
	}

	query := `SELECT DISTINCT ON (nuts_id)
		nuts_id AS geo_id,
		'NUTS' AS geo_type,
		region_name,
		levl_code AS nuts_level,
		CASE levl_code
			WHEN 0 THEN 'NUTS-0'
			WHEN 1 THEN 'NUTS-1'
			WHEN 2 THEN 'NUTS-2'
			ELSE 'NUTS-3'
		END AS nuts_level_code,
		` + geometrySelect + `,
		max_vintage_year AS year
	FROM transformed_data.mart_nuts_geometry_historized
	WHERE 1=1`

	if filter.Level != nil {
		query += fmt.Sprintf(" AND levl_code = $%d", argPos)
		args = append(args, *filter.Level)
		argPos++
	}
	if filter.IncludeGeometry {
		// In der historisierten Tabelle deckt genau eine Periode ein Jahr ab;
		// "exact" und "latest-before" fallen daher zusammen.
		// geometry_year wird nur angewendet, wenn year nicht gesetzt ist –
		// andernfalls würden zwei überschneidungsfreie Perioden-Filter zu 0
		// Ergebnissen führen, wenn geometry_year und year auf verschiedene
		// Perioden zeigen.
		if filter.GeometryYear != nil && filter.Year == nil {
			query += fmt.Sprintf(" AND valid_from <= $%d AND (valid_to IS NULL OR valid_to >= $%d)", argPos, argPos)
			args = append(args, *filter.GeometryYear)
			argPos++
		}
	}
	if filter.NutsParentID != "" {
		query += fmt.Sprintf(" AND nuts_id LIKE $%d", argPos)
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.NutsParentID))+"%")
		argPos++
	}
	if filter.GeodataNameContains != "" {
		query += fmt.Sprintf(" AND region_name ILIKE $%d", argPos)
		args = append(args, "%"+strings.TrimSpace(filter.GeodataNameContains)+"%")
		argPos++
	}
	if filter.Year != nil {
		query += fmt.Sprintf(" AND valid_from <= $%d AND (valid_to IS NULL OR valid_to >= $%d)", argPos, argPos)
		args = append(args, *filter.Year)
		argPos++
	}
	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND (valid_to IS NULL OR valid_to >= $%d)", argPos)
		args = append(args, *filter.FromYear)
		argPos++
	}
	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND valid_from <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.Get(&total, countQuery, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"nuts_id, year DESC",
		100,
		filter.Limit,
		filter.Offset,
		limitNull,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	type row struct {
		GeoID            string          `db:"geo_id"`
		GeoType          string          `db:"geo_type"`
		RegionName       string          `db:"region_name"`
		NutsLevel        int             `db:"nuts_level"`
		NutsLevelCode    string          `db:"nuts_level_code"`
		GeometryWGS84    json.RawMessage `db:"geometry_wgs84"`
		HasValidGeometry *bool           `db:"has_valid_geometry"`
		Year             int             `db:"year"`
	}

	rows := make([]row, 0)
	if err := h.DB.Select(&rows, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	data := make([]models.GeoRegionSummary, 0, len(rows))
	for _, r := range rows {
		level := r.NutsLevel
		code := r.NutsLevelCode
		// Wenn ein konkretes Jahr abgefragt wurde, wird es direkt zurückgegeben
		// (analog zu LAU, das den tatsächlichen Jahreswert aus der DB liefert).
		// Ohne Jahresfilter kommt max_vintage_year: das neueste Vintage-Jahr
		// der gültigen Periode – ebenfalls konsistent mit LAUs letztem verfügbaren Jahr.
		year := r.Year
		if filter.Year != nil {
			year = *filter.Year
		}
		data = append(data, models.GeoRegionSummary{
			GeoID:            r.GeoID,
			GeoType:          r.GeoType,
			RegionName:       r.RegionName,
			NutsLevel:        &level,
			NutsLevelCode:    &code,
			GeometryWGS84:    r.GeometryWGS84,
			HasValidGeometry: r.HasValidGeometry,
			Year:             year,
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

// GetGeoRegionByID gibt eine einzelne Geo-Region (NUTS oder LAU) zurück
// @Summary Allgemeine Geo-Region nach ID
// @Description Gibt Details für eine Geo-Region zurück. Mit geo_type=auto wird der Typ aus der ID erkannt (NUTS: 2-5 Zeichen, sonst LAU).
// @Tags Geo
// @Accept json
// @Produce json
// @Param geo_id path string true "Geo-ID (NUTS oder GISCO-ID)" example(DE111)
// @Param geo_type query string false "Regionstyp: auto, nuts oder lau" Enums(auto,nuts,lau) default(auto)
// @Param year query int false "Jahr (optional)" example(2024)
// @Param all query bool false "Nur für NUTS: alle verfügbaren Jahre zurückgeben" example(false)
// @Param part query string false "Nur für NUTS: full, metadata oder socioeconomic" Enums(full,metadata,socioeconomic) default(full)
// @Param geometry_strategy query string false "Nur für NUTS: Geometrieauswahl latest|exact" Enums(latest,exact) default(latest)
// @Param geometry_year query int false "Nur für NUTS: Zieljahr der Geometrie (bei exact erforderlich)" example(2024)
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /geo/{geo_id} [get]
func (h *Handler) GetGeoRegionByID(c *gin.Context) {
	geoID := strings.TrimSpace(c.Param("geo_id"))
	if geoID == "" {
		respondBadRequest(c, "geo_id darf nicht leer sein")
		return
	}

	geoType, err := normalizeGeoType(c.Query("geo_type"))
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if geoType == "auto" {
		geoType = detectGeoType(geoID)
	}

	if geoType == "nuts" {
		c.Params = append(c.Params, gin.Param{Key: "nuts_id", Value: geoID})
		h.GetNutsRegionData(c)
		return
	}

	c.Params = append(c.Params, gin.Param{Key: "gisco_id", Value: geoID})
	h.GetLAURegionByID(c)
}

// GetGeoRegionVersions gibt alle Geometrie-Perioden für eine Geo-Region zurück
// @Summary Geometrie-Perioden einer Geo-Region
// @Description Gibt alle Geometrie-Perioden (valid_from/valid_to) für eine Geo-Region zurück.
// Jede Periode repräsentiert eine Geometrie-Version; valid_to=null bedeutet noch aktuell.
// Mit geo_type=auto wird der Typ aus der ID erkannt.
// @Tags Geo
// @Accept json
// @Produce json
// @Param geo_id path string true "Geo-ID (NUTS oder GISCO-ID)" example(DE111)
// @Param geo_type query string false "Regionstyp: auto, nuts oder lau" Enums(auto,nuts,lau) default(auto)
// @Success 200 {object} models.APIResponse{data=models.GeoRegionVersions}
// @Failure 400 {object} models.APIResponse
// @Failure 404 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /geo/{geo_id}/versions [get]
func (h *Handler) GetGeoRegionVersions(c *gin.Context) {
	geoID := strings.TrimSpace(c.Param("geo_id"))
	if geoID == "" {
		respondBadRequest(c, "geo_id darf nicht leer sein")
		return
	}

	geoType, err := normalizeGeoType(c.Query("geo_type"))
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if geoType == "auto" {
		geoType = detectGeoType(geoID)
	}

	if geoType == "nuts" {
		details, err := nutsLevelDetailsFromID(geoID)
		if err != nil {
			respondBadRequest(c, err.Error())
			return
		}

		periods := make([]models.GeoPeriod, 0)
		query := `SELECT valid_from, valid_to
			FROM transformed_data.mart_nuts_geometry_historized
			WHERE nuts_id = $1
			ORDER BY valid_from DESC`
		if err := h.DB.Select(&periods, query, details.NormalizedID); err != nil {
			respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
			return
		}
		if len(periods) == 0 {
			respondNotFound(c, "Keine Versionsdaten für diese Geo-Region gefunden")
			return
		}

		level := details.Number
		levelCode := details.Code
		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Data: models.GeoRegionVersions{
				GeoID:         details.NormalizedID,
				GeoType:       "NUTS",
				NutsLevel:     &level,
				NutsLevelCode: &levelCode,
				Periods:       periods,
			},
		})
		return
	}

	normalizedGeoID := strings.ToUpper(geoID)
	periods := make([]models.GeoPeriod, 0)
	query := `SELECT valid_from, valid_to
		FROM transformed_data.mart_lau_geometry_historized
		WHERE gisco_id = $1
		ORDER BY valid_from DESC`
	if err := h.DB.Select(&periods, query, normalizedGeoID); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}
	if len(periods) == 0 {
		respondNotFound(c, "Keine Versionsdaten für diese Geo-Region gefunden")
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: models.GeoRegionVersions{
			GeoID:   normalizedGeoID,
			GeoType: "LAU",
			Periods: periods,
		},
	})
}

// QueryGeoRegionsByGeometry ermöglicht räumliche Gebietsanfragen per GeoJSON.
// @Summary Räumliche Geo-Abfrage via GeoJSON
// @Description Führt eine räumliche Abfrage (intersects/contains) gegen NUTS oder LAU Geometrien aus.
// @Tags Geo
// @Accept json
// @Produce json
// @Param request body models.GeoSpatialQueryRequest true "GeoJSON-Abfrage. geometry_strategy und geometry_year wirken nur bei geo_type=nuts."
// @Success 200 {object} models.APIResponse{data=[]models.GeoRegionSummary}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /geo/query [post]
func (h *Handler) QueryGeoRegionsByGeometry(c *gin.Context) {
	var filter models.GeoSpatialQueryRequest
	if err := c.ShouldBindJSON(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültiger Request-Body: %v", err))
		return
	}

	geoType, err := normalizeGeoType(filter.GeoType)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if geoType == "auto" {
		geoType = "nuts"
	}

	operation, err := normalizeSpatialOperation(filter.Operation)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	geometryStrategy, err := normalizeGeometryStrategy(filter.GeometryStrategy)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	filter.GeometryStrategy = geometryStrategy
	if filter.IncludeGeometry && filter.GeometryStrategy == "exact" && filter.GeometryYear == nil {
		respondBadRequest(c, "geometry_year ist erforderlich, wenn geometry_strategy=exact gesetzt ist")
		return
	}

	geometryJSON, err := extractGeoJSONGeometry(filter.Geometry)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}
	if filter.Limit < 1 || filter.Limit > 1000 {
		respondBadRequest(c, "limit muss zwischen 1 und 1000 liegen")
		return
	}
	if filter.Offset < 0 {
		respondBadRequest(c, "offset muss >= 0 sein")
		return
	}

	if geoType == "lau" {
		h.queryGeoRegionsLAU(c, filter, geometryJSON, operation)
		return
	}

	h.queryGeoRegionsNUTS(c, filter, geometryJSON, operation)
}

func (h *Handler) queryGeoRegionsLAU(c *gin.Context, filter models.GeoSpatialQueryRequest, geometryJSON string, operation string) {
	geometrySelect := `'null'::json AS geometry_wgs84, null::boolean AS has_valid_geometry`
	if filter.IncludeGeometry {
		geometrySelect = `geometry_wgs84, has_valid_geometry`
	}

	query := `SELECT DISTINCT ON (gisco_id)
		gisco_id AS geo_id,
		'LAU' AS geo_type,
		lau_name AS region_name,
		country_code,
		` + geometrySelect + `,
		year
	FROM transformed_data.lau_integrated
	WHERE has_valid_geometry
	  AND lau_name IS NOT NULL
	  AND ` + spatialPredicateSQL(operation, "geometry_3035", 1)

	args := []interface{}{geometryJSON}
	argPos := 2

	yearFilterApplied := false
	if filter.Year != nil {
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
		yearFilterApplied = true
	} else if filter.IncludeGeometry && filter.GeometryYear != nil {
		// Wenn kein Datenjahr gesetzt ist, aber eine explizite Geometrieversion
		// angefordert wird, wird geometry_year als Jahresfilter auf lau_integrated
		// verwendet (year-Spalte entspricht dort dem Vintage-Jahr der Geometrie).
		query += fmt.Sprintf(" AND year = $%d", argPos)
		args = append(args, *filter.GeometryYear)
		argPos++
		yearFilterApplied = true
	}
	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND year >= $%d", argPos)
		args = append(args, *filter.FromYear)
		argPos++
		yearFilterApplied = true
	}
	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND year <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
		yearFilterApplied = true
	}
	if !yearFilterApplied {
		// Ohne jeglichen Jahresfilter nur den aktuellsten Vintage-Jahrgang
		// berücksichtigen - siehe getGeoRegionsLAU für die ausführliche
		// Begründung (analog zum NUTS-valid_to-Bug).
		query += " AND year = (SELECT MAX(year) FROM transformed_data.lau_integrated)"
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.Get(&total, countQuery, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"gisco_id, year DESC",
		100,
		filter.Limit,
		filter.Offset,
		false,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	type row struct {
		GeoID            string          `db:"geo_id"`
		GeoType          string          `db:"geo_type"`
		RegionName       string          `db:"region_name"`
		Country          string          `db:"country_code"`
		GeometryWGS84    json.RawMessage `db:"geometry_wgs84"`
		HasValidGeometry *bool           `db:"has_valid_geometry"`
		Year             int             `db:"year"`
	}

	rows := make([]row, 0)
	if err := h.DB.Select(&rows, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	data := make([]models.GeoRegionSummary, 0, len(rows))
	for _, r := range rows {
		country := r.Country
		data = append(data, models.GeoRegionSummary{
			GeoID:            r.GeoID,
			GeoType:          r.GeoType,
			RegionName:       r.RegionName,
			CountryCode:      &country,
			GeometryWGS84:    r.GeometryWGS84,
			HasValidGeometry: r.HasValidGeometry,
			Year:             r.Year,
		})
	}

	if filter.IncludeDatasets {
		for i := range data {
			var nuts3Code string
			if err := h.DB.Get(&nuts3Code,
				`SELECT nuts_3_code FROM transformed_data.lau_integrated WHERE gisco_id = $1 AND nuts_3_code IS NOT NULL AND lau_name IS NOT NULL LIMIT 1`,
				data[i].GeoID,
			); err == nil && nuts3Code != "" {
				if datasets, dsErr := h.regionNutsDatasets(nuts3Code); dsErr == nil {
					data[i].Datasets = datasets
				}
			}
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}

func (h *Handler) queryGeoRegionsNUTS(c *gin.Context, filter models.GeoSpatialQueryRequest, geometryJSON string, operation string) {
	geometrySelect := `'null'::json AS geometry_wgs84, null::boolean AS has_valid_geometry`
	if filter.IncludeGeometry {
		geometrySelect = `base.geometry_wgs84, base.has_valid_geometry`
	}

	argPos := 1
	query := `WITH base AS (
		SELECT DISTINCT ON (nuts_id)
			nuts_id,
			levl_code,
			region_name,
			valid_from,
			max_vintage_year,
			geometry_3035,
			geometry_wgs84,
			has_valid_geometry
		FROM transformed_data.mart_nuts_geometry_historized
		WHERE has_valid_geometry`

	args := make([]interface{}, 0)
	// In der historisierten Tabelle deckt genau eine Periode ein Jahr ab;
	// "exact" und "latest-before" fallen daher zusammen.
	// geometry_year wird nur angewendet, wenn year nicht gesetzt ist –
	// andernfalls würden zwei überschneidungsfreie Perioden-Filter zu 0
	// Ergebnissen führen, wenn geometry_year und year auf verschiedene
	// Perioden zeigen.
	if filter.GeometryYear != nil && filter.Year == nil {
		query += fmt.Sprintf(" AND valid_from <= $%d AND (valid_to IS NULL OR valid_to >= $%d)", argPos, argPos)
		args = append(args, *filter.GeometryYear)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND valid_from <= $%d AND (valid_to IS NULL OR valid_to >= $%d)", argPos, argPos)
		args = append(args, *filter.Year)
		argPos++
	}
	if filter.FromYear != nil {
		query += fmt.Sprintf(" AND (valid_to IS NULL OR valid_to >= $%d)", argPos)
		args = append(args, *filter.FromYear)
		argPos++
	}
	if filter.ToYear != nil {
		query += fmt.Sprintf(" AND valid_from <= $%d", argPos)
		args = append(args, *filter.ToYear)
		argPos++
	}
	query += `
		ORDER BY nuts_id, valid_from DESC
	)
	SELECT
		base.nuts_id AS geo_id,
		'NUTS' AS geo_type,
		base.region_name,
		base.levl_code AS nuts_level,
		CASE base.levl_code
			WHEN 0 THEN 'NUTS-0'
			WHEN 1 THEN 'NUTS-1'
			WHEN 2 THEN 'NUTS-2'
			ELSE 'NUTS-3'
		END AS nuts_level_code,
		null::text AS country_code,
		` + geometrySelect + `,
		base.max_vintage_year AS year
	FROM base
	WHERE ` + spatialPredicateSQL(operation, "base.geometry_3035", argPos)
	args = append(args, geometryJSON)
	argPos++

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.Get(&total, countQuery, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		"geo_id, year DESC",
		100,
		filter.Limit,
		filter.Offset,
		false,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	type row struct {
		GeoID            string          `db:"geo_id"`
		GeoType          string          `db:"geo_type"`
		RegionName       string          `db:"region_name"`
		NutsLevel        int             `db:"nuts_level"`
		NutsLevelCode    string          `db:"nuts_level_code"`
		GeometryWGS84    json.RawMessage `db:"geometry_wgs84"`
		HasValidGeometry *bool           `db:"has_valid_geometry"`
		Year             int             `db:"year"`
	}

	rows := make([]row, 0)
	if err := h.DB.Select(&rows, pagedQuery, pagedArgs...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	data := make([]models.GeoRegionSummary, 0, len(rows))
	for _, r := range rows {
		level := r.NutsLevel
		code := r.NutsLevelCode
		year := r.Year
		if filter.Year != nil {
			year = *filter.Year
		}
		data = append(data, models.GeoRegionSummary{
			GeoID:            r.GeoID,
			GeoType:          r.GeoType,
			RegionName:       r.RegionName,
			NutsLevel:        &level,
			NutsLevelCode:    &code,
			GeometryWGS84:    r.GeometryWGS84,
			HasValidGeometry: r.HasValidGeometry,
			Year:             year,
		})
	}

	if filter.IncludeDatasets {
		for i := range data {
			if data[i].NutsLevel != nil && *data[i].NutsLevel == 3 {
				if datasets, dsErr := h.regionNutsDatasets(data[i].GeoID); dsErr == nil {
					data[i].Datasets = datasets
				}
			}
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}
