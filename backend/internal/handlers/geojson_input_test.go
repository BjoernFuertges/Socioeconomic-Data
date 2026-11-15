package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func TestValidateGeoJSONGeometry(t *testing.T) {
	tests := []struct {
		name       string
		geometry   string
		wantType   string
		wantPoints int
		wantErr    string
	}{
		{"polygon", testPolygon, "Polygon", 5, ""},
		{"point allowed", `{"type":"Point","coordinates":[13.4,52.5]}`, "Point", 1, ""},
		{"line allowed", `{"type":"LineString","coordinates":[[13,52],[14,53]]}`, "LineString", 2, ""},
		{"multipolygon", `{"type":"MultiPolygon","coordinates":[[[[13,52],[14,52],[14,53],[13,52]]]]}`, "MultiPolygon", 4, ""},
		{"collection", `{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[13,52]},` + testPolygon + `]}`, "GeometryCollection", 6, ""},
		{"position with altitude", `{"type":"Point","coordinates":[13,52,34.5]}`, "Point", 1, ""},
		{"unknown type", `{"type":"Circle","coordinates":[13,52]}`, "", 0, "unbekannter Geometrietyp"},
		{"feature not unwrapped", `{"type":"Feature"}`, "", 0, "unbekannter Geometrietyp"},
		{"no coordinates", `{"type":"Polygon"}`, "", 0, "ohne coordinates"},
		{"empty polygon", `{"type":"Polygon","coordinates":[]}`, "", 0, "leeres coordinates-Array"},
		{"ring too short", `{"type":"Polygon","coordinates":[[[13,52],[14,52],[13,52]]]}`, "", 0, "mindestens 4"},
		{"ring not closed", `{"type":"Polygon","coordinates":[[[13,52],[14,52],[14,53],[13,53]]]}`, "", 0, "nicht geschlossen"},
		{"line with one position", `{"type":"LineString","coordinates":[[13,52]]}`, "", 0, "mindestens 2"},
		{"string coordinate", `{"type":"Point","coordinates":["13","52"]}`, "", 0, "keine Zahl"},
		{"lat/lon swapped out of range", `{"type":"Point","coordinates":[52,213]}`, "", 0, "außerhalb von WGS84"},
		{"projected coordinates", `{"type":"Point","coordinates":[4321000,3210000]}`, "", 0, "außerhalb von WGS84"},
		{"wrong nesting", `{"type":"Polygon","coordinates":[[13,52],[14,52]]}`, "", 0, "muss [lon, lat] sein"},
		{"position instead of array", `{"type":"LineString","coordinates":13}`, "", 0, "falsche Struktur"},
		{"empty collection", `{"type":"GeometryCollection","geometries":[]}`, "", 0, "keine Geometrien"},
		{"malformed json", `{"type":`, "", 0, "ungültiges GeoJSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotPoints, err := validateGeoJSONGeometry(tt.geometry)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType || gotPoints != tt.wantPoints {
				t.Errorf("got (%q, %d), want (%q, %d)", gotType, gotPoints, tt.wantType, tt.wantPoints)
			}
		})
	}
}

// bigLineString builds a LineString with n positions.
func bigLineString(n int) string {
	var b strings.Builder
	b.WriteString(`{"type":"LineString","coordinates":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "[%d.%d,52]", 10+i%5, i%1000)
	}
	b.WriteString("]}")
	return b.String()
}

func TestValidateGeoJSONGeometryPointLimit(t *testing.T) {
	if _, n, err := validateGeoJSONGeometry(bigLineString(geoJSONMaxPoints)); err != nil || n != geoJSONMaxPoints {
		t.Fatalf("exactly at the limit must pass: n=%d err=%v", n, err)
	}
	if _, _, err := validateGeoJSONGeometry(bigLineString(geoJSONMaxPoints + 1)); err == nil || !strings.Contains(err.Error(), "Stützpunkte") {
		t.Fatalf("above the limit must fail with a vertex error, got %v", err)
	}
}

func TestGeometryInputErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string // "" = not an input error (respondDBError)
	}{
		{"unknown type", &pq.Error{Code: "XX000", Message: "unknown GeoJSON type"}, geoJSONParseErrorMessage},
		{"invalid representation", &pq.Error{Code: "XX000", Message: "invalid GeoJson representation"}, geoJSONParseErrorMessage},
		{"missing coordinates", &pq.Error{Code: "XX000", Message: "Unable to find 'coordinates' in GeoJSON string"}, geoJSONParseErrorMessage},
		{"not nested", &pq.Error{Code: "XX000", Message: "The 'coordinates' in GeoJSON are not sufficiently nested"}, geoJSONParseErrorMessage},
		{"json-c tokener", &pq.Error{Code: "XX000", Message: "null expected (at offset 1)"}, geoJSONParseErrorMessage},
		{"proj transform", &pq.Error{Code: "XX000", Message: "transform: latitude or longitude exceeded limits (-14)"}, geoJSONTransformErrorMessage},
		{"data exception class 22", &pq.Error{Code: "22023", Message: "invalid parameter"}, geoJSONParseErrorMessage},
		{"GEOS topology error (may stem from stored geometries)", &pq.Error{Code: "XX000", Message: "GEOSIntersects: TopologyException: side location conflict"}, ""},
		{"other internal error", &pq.Error{Code: "XX000", Message: "could not open relation"}, ""},
		{"undefined table", &pq.Error{Code: "42P01", Message: `relation "transformed_data.x" does not exist`}, ""},
		{"statement timeout", &pq.Error{Code: "57014"}, ""},
		{"connection failure", &pq.Error{Code: "08006"}, ""},
		{"out of memory", &pq.Error{Code: "53200"}, ""},
		{"non-pq error", errors.New("driver: bad connection"), ""},
	}
	for _, tt := range tests {
		msg, ok := geometryInputErrorMessage(tt.err)
		if ok != (tt.wantMsg != "") || msg != tt.wantMsg {
			t.Errorf("%s: got (%q, %v), want %q", tt.name, msg, ok, tt.wantMsg)
		}
		// The raw PostGIS text must never be part of the client message.
		var pqErr *pq.Error
		if ok && errors.As(tt.err, &pqErr) && pqErr.Message != "" && strings.Contains(msg, pqErr.Message) {
			t.Errorf("%s: client message leaks the PostGIS text: %q", tt.name, msg)
		}
	}
}

// Polygonal input is repaired (ST_MakeValid), points/lines are not; the count
// statement and the page query share one geometry preparation.
func TestGeoJSONGeometrySQL(t *testing.T) {
	for _, gt := range []string{"Polygon", "MultiPolygon", "GeometryCollection"} {
		if sql := geoJSONGeometrySQL("x", gt); !strings.Contains(sql, "ST_MakeValid(x)") {
			t.Errorf("%s must be repaired: %s", gt, sql)
		}
	}
	for _, gt := range []string{"Point", "MultiPoint", "LineString", "MultiLineString"} {
		if sql := geoJSONGeometrySQL("x", gt); sql != "x" {
			t.Errorf("%s must stay unchanged: %s", gt, sql)
		}
	}

	cte := geoJSONInputCTE(3, "Polygon")
	if !strings.Contains(cte, "ST_GeomFromGeoJSON($3)") || strings.Count(cte, "ST_GeomFromGeoJSON") != 1 {
		t.Errorf("input must be parsed exactly once from $3: %s", cte)
	}
	check := geoJSONCheckSQL(3, "Polygon", "SELECT 1 WHERE ST_Intersects(g, "+geoInputRef+")")
	if strings.Count(check, "ST_GeomFromGeoJSON") != 1 || !strings.Contains(check, cte) {
		t.Errorf("check statement must reuse geoJSONInputCTE and parse once: %s", check)
	}
	if !strings.Contains(check, "CASE WHEN is_valid AND NOT is_empty THEN (SELECT COUNT(*)") {
		t.Errorf("count must only run for a usable geometry: %s", check)
	}
}

func TestGeoJSONCheckRowInputError(t *testing.T) {
	reason := "Self-intersection[13.5 52.5]"
	tests := []struct {
		name     string
		row      geoJSONCheckRow
		geomType string
		want     string
	}{
		{"valid", geoJSONCheckRow{IsValid: true, Points: 5}, "Polygon", ""},
		{"invalid line", geoJSONCheckRow{IsValid: false, Reason: &reason}, "LineString", "OGC-Validität"},
		{"polygon without area after repair", geoJSONCheckRow{IsValid: true, IsEmpty: true}, "Polygon", "keine Fläche"},
		{"too many points after repair", geoJSONCheckRow{IsValid: true, Points: geoJSONMaxPoints + 1}, "MultiPolygon", "Stützpunkte"},
	}
	for _, tt := range tests {
		err := tt.row.inputError(tt.geomType)
		if (tt.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func doPost(r *gin.Engine, target string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// All of these must be rejected before any DB access - the handler has no DB
// here, so reaching it would panic instead of returning the status.
func TestGeoJSONEndpointsRejectBadInputWithoutDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	r := gin.New()
	r.POST("/geo/query", h.QueryGeoRegionsByGeometry)
	r.POST("/custom-regions/dataset-aggregate", h.QueryCustomRegionDatasetAggregate)

	oversized := []byte(`{"geometry":` + bigLineString(geoJSONMaxBodyBytes/8) + `}`)
	if len(oversized) <= geoJSONMaxBodyBytes {
		t.Fatalf("test body too small: %d bytes", len(oversized))
	}
	tooManyPoints := []byte(`{"geometry":` + bigLineString(geoJSONMaxPoints+1) + `}`)
	if len(tooManyPoints) > geoJSONMaxBodyBytes {
		t.Fatalf("vertex test body exceeds the body limit: %d bytes", len(tooManyPoints))
	}

	tests := []struct {
		name       string
		target     string
		body       []byte
		wantStatus int
	}{
		{"geo/query body too large", "/geo/query", oversized, http.StatusRequestEntityTooLarge},
		{"geo/query too many points", "/geo/query", tooManyPoints, http.StatusBadRequest},
		{"geo/query unknown type", "/geo/query", []byte(`{"geometry":{"type":"Circle","coordinates":[1,2]}}`), http.StatusBadRequest},
		{"geo/query unclosed ring", "/geo/query", []byte(`{"geometry":{"type":"Polygon","coordinates":[[[13,52],[14,52],[14,53],[13,53]]]}}`), http.StatusBadRequest},
		{"geo/query coordinates out of range", "/geo/query", []byte(`{"geometry":{"type":"Point","coordinates":[400,52]}}`), http.StatusBadRequest},
		{"geo/query chunk_size above cap", "/geo/query?chunk_size=10001", []byte(`{"geometry":{"type":"Point","coordinates":[13,52]}}`), http.StatusBadRequest},
		{"geo/query malformed json", "/geo/query", []byte(`{"geometry":`), http.StatusBadRequest},
		{"custom region body too large", "/custom-regions/dataset-aggregate", oversized, http.StatusRequestEntityTooLarge},
		{"custom region unclosed ring", "/custom-regions/dataset-aggregate", []byte(`{"datasets":["bevoelkerung"],"geometry":{"type":"Polygon","coordinates":[[[13,52],[14,52],[14,53],[13,53]]]}}`), http.StatusBadRequest},
		{"custom region point", "/custom-regions/dataset-aggregate", []byte(`{"datasets":["bevoelkerung"],"geometry":{"type":"Point","coordinates":[13,52]}}`), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doPost(r, tt.target, tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %.200s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
