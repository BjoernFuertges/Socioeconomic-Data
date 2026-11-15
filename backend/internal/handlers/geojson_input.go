package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// Shared limits and checks for request bodies carrying a GeoJSON geometry
// (POST /geo/query, POST /custom-regions/dataset-aggregate). They keep a
// single request from tying up a DB connection with a huge or broken
// geometry and turn malformed input into a 400 instead of a database 500.
const (
	geoJSONMaxBodyBytes = 5 << 20 // 5 MiB GeoJSON
	geoJSONMaxPoints    = 200000
	// geoJSONMaxNesting bounds GeometryCollection recursion.
	geoJSONMaxNesting = 4
)

// bindGeoJSONBody binds a JSON body limited to geoJSONMaxBodyBytes and writes
// the error response itself (413 if the body is too large, 400 otherwise).
// Returns false if the handler must stop.
func bindGeoJSONBody(c *gin.Context, dst interface{}) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, geoJSONMaxBodyBytes)
	if err := c.ShouldBindJSON(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, models.APIResponse{
				Success: false,
				Error:   fmt.Sprintf("Request-Body zu groß (maximal %d MiB)", geoJSONMaxBodyBytes>>20),
			})
			return false
		}
		respondBadRequest(c, fmt.Sprintf("Ungültiger Request-Body: %v", err))
		return false
	}
	return true
}

type geoJSONGeometry struct {
	Type        string            `json:"type"`
	Coordinates json.RawMessage   `json:"coordinates"`
	Geometries  []json.RawMessage `json:"geometries"`
}

// validateGeoJSONGeometry checks a bare GeoJSON geometry object (output of
// extractGeoJSONGeometry) without the database: known type, well-formed
// coordinates (RFC 7946: lines >= 2 positions, closed rings >= 4 positions),
// WGS84 ranges and at most geoJSONMaxPoints positions in total. Returns the
// geometry type and the number of positions.
func validateGeoJSONGeometry(geometryJSON string) (string, int, error) {
	points := 0
	geomType, err := validateGeoJSONNode(json.RawMessage(geometryJSON), 0, &points)
	if err != nil {
		return "", 0, err
	}
	if points == 0 {
		return "", 0, fmt.Errorf("ungültiges GeoJSON: die Geometrie ist leer")
	}
	return geomType, points, nil
}

func validateGeoJSONNode(raw json.RawMessage, nesting int, points *int) (string, error) {
	var g geoJSONGeometry
	if err := json.Unmarshal(raw, &g); err != nil {
		return "", fmt.Errorf("ungültiges GeoJSON: %v", err)
	}

	// Nesting depth of the coordinates array per type; rings must be closed.
	var depth int
	ring := false
	minPositions := 1
	switch g.Type {
	case "Point":
		depth = 0
	case "MultiPoint":
		depth = 1
	case "LineString":
		depth, minPositions = 1, 2
	case "MultiLineString":
		depth, minPositions = 2, 2
	case "Polygon":
		depth, minPositions, ring = 2, 4, true
	case "MultiPolygon":
		depth, minPositions, ring = 3, 4, true
	case "GeometryCollection":
		if nesting >= geoJSONMaxNesting {
			return "", fmt.Errorf("ungültiges GeoJSON: GeometryCollection zu tief verschachtelt")
		}
		if len(g.Geometries) == 0 {
			return "", fmt.Errorf("ungültiges GeoJSON: GeometryCollection enthält keine Geometrien")
		}
		for _, child := range g.Geometries {
			if _, err := validateGeoJSONNode(child, nesting+1, points); err != nil {
				return "", err
			}
		}
		return g.Type, nil
	default:
		return "", fmt.Errorf("ungültiges GeoJSON: unbekannter Geometrietyp %q", g.Type)
	}

	if len(g.Coordinates) == 0 {
		return "", fmt.Errorf("ungültiges GeoJSON: %s ohne coordinates", g.Type)
	}
	var coords interface{}
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return "", fmt.Errorf("ungültiges GeoJSON: %v", err)
	}
	if err := validateGeoJSONCoords(g.Type, coords, depth, minPositions, ring, points); err != nil {
		return "", err
	}
	return g.Type, nil
}

// validateGeoJSONCoords walks depth array levels down to the positions. At
// depth 1 the array is a sequence of positions (line, ring, multipoint
// members) checked against minPositions/ring.
func validateGeoJSONCoords(geomType string, v interface{}, depth int, minPositions int, ring bool, points *int) error {
	if depth == 0 {
		return validateGeoJSONPosition(geomType, v, points)
	}
	arr, ok := v.([]interface{})
	if !ok {
		return fmt.Errorf("ungültiges GeoJSON: coordinates von %s haben die falsche Struktur", geomType)
	}
	if len(arr) == 0 {
		return fmt.Errorf("ungültiges GeoJSON: %s enthält ein leeres coordinates-Array", geomType)
	}
	if depth == 1 {
		for _, p := range arr {
			if err := validateGeoJSONPosition(geomType, p, points); err != nil {
				return err
			}
		}
		if len(arr) < minPositions {
			return fmt.Errorf("ungültiges GeoJSON: %s braucht mindestens %d Positionen je Linie/Ring, erhalten %d", geomType, minPositions, len(arr))
		}
		if ring && !samePosition(arr[0], arr[len(arr)-1]) {
			return fmt.Errorf("ungültiges GeoJSON: Ring von %s ist nicht geschlossen (erste und letzte Position müssen gleich sein)", geomType)
		}
		return nil
	}
	for _, child := range arr {
		if err := validateGeoJSONCoords(geomType, child, depth-1, minPositions, ring, points); err != nil {
			return err
		}
	}
	return nil
}

func validateGeoJSONPosition(geomType string, v interface{}, points *int) error {
	pos, ok := v.([]interface{})
	if !ok || len(pos) < 2 || len(pos) > 4 {
		return fmt.Errorf("ungültiges GeoJSON: Position in %s muss [lon, lat] sein", geomType)
	}
	for _, n := range pos {
		f, ok := n.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("ungültiges GeoJSON: Position in %s enthält keine Zahl", geomType)
		}
	}
	lon, lat := pos[0].(float64), pos[1].(float64)
	if lon < -180 || lon > 180 || lat < -90 || lat > 90 {
		return fmt.Errorf("ungültiges GeoJSON: Position [%g, %g] liegt außerhalb von WGS84 (erwartet [lon, lat], EPSG:4326)", lon, lat)
	}
	*points++
	if *points > geoJSONMaxPoints {
		return fmt.Errorf("Geometrie hat zu viele Stützpunkte (maximal %d)", geoJSONMaxPoints)
	}
	return nil
}

func samePosition(a, b interface{}) bool {
	pa, okA := a.([]interface{})
	pb, okB := b.([]interface{})
	if !okA || !okB || len(pa) < 2 || len(pb) < 2 {
		return false
	}
	return pa[0] == pb[0] && pa[1] == pb[1]
}

// Client-facing texts for geometries PostGIS rejects. Fixed wording: the raw
// PostGIS/GEOS message never reaches the client (it is only logged).
const (
	geoJSONParseErrorMessage     = "Ungültiges GeoJSON: PostGIS konnte die Geometrie nicht lesen (erwartet eine GeoJSON-Geometrie in WGS84/EPSG:4326, [lon, lat])"
	geoJSONTransformErrorMessage = "Geometrie nicht transformierbar: Umrechnung von WGS84 (EPSG:4326) nach EPSG:3035 fehlgeschlagen (erwartet [lon, lat])"
)

// postGISGeoJSONParseErrors are the messages ST_GeomFromGeoJSON raises (all
// SQLSTATE XX000) for malformed input (PostGIS 3.4/3.5, lwin_geojson.c).
var postGISGeoJSONParseErrors = []string{
	"invalid GeoJson representation",
	"invalid GeoJSON representation",
	"unknown GeoJSON type",
	"Unable to find 'coordinates' in GeoJSON string",
	"Unable to find 'geometries' in GeoJSON string",
	"The 'coordinates' in GeoJSON are not an array",
	"The 'coordinates' in GeoJSON are not sufficiently nested",
	"The 'coordinates' in GeoJSON ring are not an array",
	"Too few ordinates in GeoJSON",
	"lwgeom_from_geojson returned NULL",
}

// jsonTokenerError matches json-c parse errors as PostGIS reports them,
// "<description> (at offset N)".
var jsonTokenerError = regexp.MustCompile(`\(at offset \d+\)$`)

// geometryInputErrorMessage classifies a DB error of a query that evaluates
// the client's GeoJSON. It returns a fixed client message and true only for
// an explicit whitelist of input errors: SQLSTATE class 22 (data exception)
// and XX000 with a known ST_GeomFromGeoJSON parse message or a PROJ transform
// failure ("transform: ..."). Everything else - including GEOS errors, which
// may just as well come from stored geometries - is a server error for
// respondDBError (500/503, logged).
func geometryInputErrorMessage(err error) (string, bool) {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return "", false
	}
	if pqErr.Code.Class() == "22" {
		return geoJSONParseErrorMessage, true
	}
	if pqErr.Code != "XX000" {
		return "", false
	}
	msg := strings.TrimSpace(pqErr.Message)
	for _, known := range postGISGeoJSONParseErrors {
		if strings.HasPrefix(msg, known) {
			return geoJSONParseErrorMessage, true
		}
	}
	if jsonTokenerError.MatchString(msg) {
		return geoJSONParseErrorMessage, true
	}
	if strings.HasPrefix(msg, "transform: ") {
		return geoJSONTransformErrorMessage, true
	}
	return "", false
}

// geoJSONGeometrySQL is the single definition of how a client GeoJSON
// geometry becomes the EPSG:3035 geometry the spatial queries use, shared by
// POST /geo/query and POST /custom-regions/dataset-aggregate. src is an
// EPSG:3035 geometry expression of the parsed input. Polygons are repaired
// (self-intersections, overlapping parts) with ST_MakeValid and reduced to
// their areal part; a GeometryCollection is only made valid; points and lines
// stay as they are.
func geoJSONGeometrySQL(src string, geomType string) string {
	switch geomType {
	case "Polygon", "MultiPolygon":
		return "ST_Multi(ST_CollectionExtract(ST_UnaryUnion(ST_MakeValid(" + src + ")), 3))"
	case "GeometryCollection":
		return "ST_MakeValid(" + src + ")"
	}
	return src
}

// geoJSONParsedSQL parses the GeoJSON text bound at $param into EPSG:3035.
func geoJSONParsedSQL(param int) string {
	return fmt.Sprintf("ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326), 3035)", param)
}

// geoInputRef refers to the prepared client geometry inside a query that
// starts with geoJSONInputCTE.
const geoInputRef = "(SELECT g FROM geo_input)"

// geoJSONInputCTE returns the CTEs geo_raw (input in WGS84) and geo_input
// (column g: the geometry in EPSG:3035, repaired per geoJSONGeometrySQL).
// The input is parsed and transformed exactly once per query.
func geoJSONInputCTE(param int, geomType string) string {
	return fmt.Sprintf(`geo_raw AS (
		SELECT ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326) AS g4326
	),
	geo_input AS (
		SELECT %s AS g, g4326 FROM geo_raw
	)`, param, geoJSONGeometrySQL("ST_Transform(g4326, 3035)", geomType))
}

// geoJSONCheckSQL checks the prepared client geometry (validity, emptiness,
// vertex count) and - only if it is usable - counts the rows of countInner in
// the same statement, so POST /geo/query needs no separate check round trip.
// countInner may reference geoInputRef; empty means "no count" (total NULL).
// The validity reason is taken from the WGS84 input, so it names [lon, lat].
func geoJSONCheckSQL(param int, geomType string, countInner string) string {
	total := "NULL::bigint"
	if countInner != "" {
		total = "CASE WHEN is_valid AND NOT is_empty THEN (SELECT COUNT(*) FROM (" + countInner + ") cnt) END"
	}
	return "WITH " + geoJSONInputCTE(param, geomType) + `,
	geo_check AS (
		SELECT ST_IsValid(g) AS is_valid, ST_IsEmpty(g) AS is_empty, ST_NPoints(g) AS points, g4326
		FROM geo_input
	)
	SELECT is_valid, is_empty, points,
		CASE WHEN NOT is_valid THEN ST_IsValidReason(g4326) END AS reason,
		` + total + ` AS total
	FROM geo_check`
}

type geoJSONCheckRow struct {
	IsValid bool    `db:"is_valid"`
	IsEmpty bool    `db:"is_empty"`
	Points  int     `db:"points"`
	Reason  *string `db:"reason"`
	Total   *int    `db:"total"`
}

// inputError turns a check row into the client error for an unusable
// geometry (respond 400), or nil.
func (r geoJSONCheckRow) inputError(geomType string) error {
	if !r.IsValid {
		reason := "unbekannt"
		if r.Reason != nil {
			reason = *r.Reason
		}
		return fmt.Errorf("Ungültige Geometrie (OGC-Validität): %s", reason)
	}
	if r.IsEmpty {
		if geomType == "Polygon" || geomType == "MultiPolygon" {
			return fmt.Errorf("Die Geometrie enthält keine Fläche")
		}
		return fmt.Errorf("Die Geometrie ist leer")
	}
	if r.Points > geoJSONMaxPoints {
		return fmt.Errorf("Geometrie hat zu viele Stützpunkte (%d, maximal %d)", r.Points, geoJSONMaxPoints)
	}
	return nil
}

// checkGeoJSONInDB lets PostGIS parse and prepare the geometry exactly as the
// spatial queries do (geoJSONInputCTE) and checks the result. With a
// countInner query (see geoJSONCheckSQL) it also returns that query's row
// count; args are the bind args of the statement, the geometry at $param.
// inputErr is a client error (400, fixed text), dbErr anything else
// (respondDBError).
func (h *Handler) checkGeoJSONInDB(ctx context.Context, geomType string, param int, countInner string, args []interface{}) (total int, inputErr error, dbErr error) {
	var row geoJSONCheckRow
	if err := h.DB.GetContext(ctx, &row, geoJSONCheckSQL(param, geomType, countInner), args...); err != nil {
		if msg, ok := geometryInputErrorMessage(err); ok {
			log.Printf("Geometrie-Eingabefehler (400): %v", err)
			return 0, errors.New(msg), nil
		}
		return 0, nil, err
	}
	if err := row.inputError(geomType); err != nil {
		return 0, err, nil
	}
	if row.Total != nil {
		total = *row.Total
	}
	return total, nil, nil
}
