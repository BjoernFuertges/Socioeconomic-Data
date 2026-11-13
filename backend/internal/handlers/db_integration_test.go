package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// DB integration tests against a real PostGIS. Opt-in: SOZ_TEST_PG_DSN is a
// lib/pq DSN of a server where the user may CREATE DATABASE (e.g. a throwaway
// `docker run postgis/postgis:17-3.5` container). The tests create their own
// database with synthetic fixtures and drop it afterwards, so no existing
// database is touched.
//
//	SOZ_TEST_PG_DSN="host=127.0.0.1 port=55439 user=dbt password=... dbname=soz sslmode=disable" go test ./internal/handlers/ -run DB
func openIntegrationDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("SOZ_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SOZ_TEST_PG_DSN not set")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	dbName := fmt.Sprintf("soz_handlers_it_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		admin.Close()
		t.Fatalf("create database: %v", err)
	}
	db, err := sqlx.Connect("postgres", dsn+" dbname="+dbName)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		admin.Close()
	})
	if _, err := db.Exec(integrationFixtureSQL); err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	return db
}

// Column types follow the dbt models: integer year/target_version in the
// harmonized mart, bigint valid_from/max_vintage_year (loaded_data.year) in
// the geometry mart - the metrics queries bind $2 against both.
const integrationFixtureSQL = `
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE SCHEMA transformed_data;

CREATE TABLE transformed_data.mart_nuts3_sociodata_harmonized (
	dataset_slug text, target_nuts_id text, year integer, target_version integer,
	harmonized_value double precision, is_crosswalked boolean,
	source_version integer, n_hops integer, has_valid_value boolean);

-- DE001..DE003 exist in the 2024 geometry; DE004 has no geometry.
INSERT INTO transformed_data.mart_nuts3_sociodata_harmonized VALUES
	('schuldnerquote', 'DE001', 2020, 2024, 5,   false, 2024, 0, true),
	('schuldnerquote', 'DE002', 2020, 2024, 150, false, 2024, 0, false),
	('schuldnerquote', 'DE003', 2020, 2024, 7,   false, 2024, 0, true),
	('schuldnerquote', 'DE001', 2021, 2024, 6,   false, 2024, 0, true),
	('schuldnerquote', 'DE002', 2021, 2024, 8,   false, 2024, 0, true),
	('schuldnerquote', 'DE001', 2022, 2024, -1,  false, 2024, 0, false),
	('bevoelkerung',   'DE001', 2020, 2024, 1000, false, 2024, 0, true),
	('bevoelkerung',   'DE002', 2020, 2024, -5,   false, 2024, 0, false),
	('bevoelkerung',   'DE003', 2020, 2024, 3000, false, 2024, 0, true),
	('bevoelkerung',   'DE004', 2020, 2024, 4000, false, 2024, 0, true),
	('arbeitslose',    'DE001', 2020, 2024, 10,  false, 2024, 0, true),
	('arbeitslose',    'DE002', 2020, 2024, 20,  false, 2024, 0, true),
	('arbeitslose',    'DE003', 2020, 2024, 30,  false, 2024, 0, false),
	('arbeitslose',    'DE005', 2020, 2024, 50,  false, 2024, 0, true),
	('beschaeftigungsquote-nuts3', 'DE001', 2020, 2024, 60, false, 2024, 0, true),
	('beschaeftigungsquote-nuts3', 'DE002', 2020, 2024, 70, false, 2024, 0, true),
	-- Rate per 1,000 SV employees x SV employees (product metric).
	('auszubildende',    'DE001', 2020, 2024, 50,   false, 2024, 0, true),
	('auszubildende',    'DE002', 2020, 2024, 40,   false, 2024, 0, true),
	('auszubildende',    'DE003', 2020, 2024, 30,   false, 2024, 0, false),
	('auszubildende',    'DE004', 2020, 2024, 20,   false, 2024, 0, true),
	('auszubildende',    'DE005', 2020, 2024, 60,   false, 2024, 0, true),
	('sv_beschaeftigte', 'DE001', 2020, 2024, 2000, false, 2024, 0, true),
	('sv_beschaeftigte', 'DE002', 2020, 2024, 3000, false, 2024, 0, false),
	('sv_beschaeftigte', 'DE003', 2020, 2024, 1000, false, 2024, 0, true),
	('sv_beschaeftigte', 'DE004', 2020, 2024, 4000, false, 2024, 0, true);

CREATE TABLE transformed_data.stg_beschaeftigungsquote_nuts3 (
	geodata_id_nuts text, year integer, has_valid_value boolean,
	beschaeftigungsquote_prozent double precision, beschaeftigungsquote_median double precision,
	beschaeftigungsquote_ungewichtet double precision, gemeinden_count bigint);
INSERT INTO transformed_data.stg_beschaeftigungsquote_nuts3 VALUES
	('DE001', 2020, true, 60, 61, 62, 10),
	('DE002', 2020, true, 70, NULL, 72, 20),
	('DE003', 2020, false, NULL, NULL, NULL, 0);

CREATE TABLE transformed_data.mart_nuts_geometry_historized (
	nuts_id text, levl_code bigint, country_code text, region_name text,
	valid_from bigint, valid_to bigint, max_vintage_year bigint,
	geometry_3035 geometry, geometry_wgs84 geometry, has_valid_geometry boolean);
-- 10 km x 10 km squares (100 km²) in EPSG:3035 around 10°E/52°N.
INSERT INTO transformed_data.mart_nuts_geometry_historized
SELECT id, 3, 'DE', 'Region ' || id, 2021, NULL, 2024, g, ST_Transform(g, 4326), true
FROM (VALUES
	('DE001', ST_MakeEnvelope(4300000, 3250000, 4310000, 3260000, 3035)),
	('DE002', ST_MakeEnvelope(4310000, 3250000, 4320000, 3260000, 3035)),
	('DE003', ST_MakeEnvelope(4320000, 3250000, 4330000, 3260000, 3035))
) v(id, g);

CREATE TABLE transformed_data.mart_nuts3_summary (nuts_id text, region_name text, geodata_id_ars bigint);
INSERT INTO transformed_data.mart_nuts3_summary VALUES ('DE001', 'A', 1), ('DE002', 'B', 2), ('DE003', 'C', 3);
`

func integrationRouter(db *sqlx.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(db, models.DerivedMetricsCatalog{
		derivedEntryFor("dichte", "bevoelkerung", "bevoelkerung", "", "", "nuts_geometry_area_sqkm", 1),
		derivedEntryFor("quote", "arbeitslose", "arbeitslose", "bevoelkerung", "bevoelkerung", "", 100),
		func() models.DerivedMetricEntry {
			e := derivedProductEntry("auszubildende", "auszubildende", "sv_beschaeftigte", "sv_beschaeftigte", 0.001)
			e.Slug = "azubi-absolut"
			e.Note = "Rückrechnung."
			return e
		}(),
	})
	r := gin.New()
	r.GET("/metrics", h.GetMetrics)
	r.GET("/metrics/derived/:slug", h.GetDerivedMetricsCatalogEntry)
	r.POST("/geo/query", h.QueryGeoRegionsByGeometry)
	r.GET("/nuts", h.GetAllNutsRegions)
	return r
}

func derivedEntryFor(slug, num, numCol, den, denCol, builtin string, scale float64) models.DerivedMetricEntry {
	e := derivedEntry(num, numCol, den, denCol, builtin)
	e.Slug = slug
	e.Formula.Scale = scale
	return e
}

type metricsResponse struct {
	Success bool             `json:"success"`
	Error   string           `json:"error"`
	Data    json.RawMessage  `json:"data"`
	Meta    *models.MetaInfo `json:"meta"`
}

func getJSON(t *testing.T, r *gin.Engine, target string) (int, metricsResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	var resp metricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: %v (%s)", target, err, w.Body.String())
	}
	return w.Code, resp
}

func TestDBMetricsExcludeInvalidValues(t *testing.T) {
	r := integrationRouter(openIntegrationDB(t))

	code, resp := getJSON(t, r, "/metrics?dataset=schuldnerquote&geodata_id_nuts=DE002")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	var results []models.DatasetMetricsResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || resp.Meta == nil || !strings.Contains(resp.Meta.Note, "Nur gültige Werte") {
		t.Fatalf("results=%+v meta=%+v", results, resp.Meta)
	}
	// With geodata_id_nuts the statistics are filtered to that region
	// (existing behaviour): DE002 is invalid in 2020, so only 2021 remains;
	// there it is ranked among the valid values only (6, 8 -> rank 2 of 2).
	res := results[0]
	if len(res.ByYear) != 1 || res.ExcludedInvalidCount != 1 || res.ExcludedRegions != 1 {
		t.Fatalf("result = %+v", res)
	}
	y2021 := res.ByYear[0]
	if y2021.Year != 2021 || y2021.RegionalRank == nil || *y2021.RegionalRank != 2 || *y2021.RegionalRankTotal != 2 {
		t.Errorf("2021 = %+v", y2021)
	}

	code, resp = getJSON(t, r, "/metrics?dataset=schuldnerquote")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	results = nil
	_ = json.Unmarshal(resp.Data, &results)
	res = results[0]
	// 2020: 5 and 7 valid, 150 excluded; 2022 only has an invalid value: not
	// in by_year, but counted in the totals (DE002 2020, DE001 2022).
	if len(res.ByYear) != 2 || res.ExcludedInvalidCount != 2 || res.ExcludedRegions != 2 {
		t.Fatalf("result = %+v", res)
	}
	y := res.ByYear[0]
	if y.Year != 2020 || y.RegionCount != 2 || y.Max != 7 || y.Mean != 6 || y.ExcludedInvalidCount != 1 {
		t.Errorf("2020 stats = %+v", y)
	}
}

// One query per column, run in parallel: the response keeps the column order
// of the dataset config, harmonized and staging columns mixed.
func TestDBMetricsColumnOrder(t *testing.T) {
	r := integrationRouter(openIntegrationDB(t))
	code, resp := getJSON(t, r, "/metrics?dataset=beschaeftigungsquote-nuts3&dataset=schuldnerquote")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	var results []models.DatasetMetricsResult
	_ = json.Unmarshal(resp.Data, &results)
	var got []string
	for _, res := range results {
		got = append(got, res.Dataset+"/"+res.Column)
	}
	var want []string
	for _, slug := range []string{"beschaeftigungsquote-nuts3", "schuldnerquote"} {
		cfg, _ := models.DatasetConfigBySlug(slug)
		for _, col := range cfg.ValueColumns {
			want = append(want, slug+"/"+col)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for _, res := range results {
		if res.Column == "beschaeftigungsquote_median" {
			// DE002 has a NULL median, DE003 is invalid: both excluded.
			if len(res.ByYear) != 1 || res.ByYear[0].RegionCount != 1 || res.ExcludedInvalidCount != 2 || res.NutsVersion != nil {
				t.Errorf("median = %+v", res)
			}
		}
	}
}

func TestDBDerivedMetricsReportExcludedRegions(t *testing.T) {
	r := integrationRouter(openIntegrationDB(t))

	// Area denominator, $2 bound against integer target_version and bigint
	// valid_from/max_vintage_year: DE004 has no geometry, DE002 is invalid.
	code, resp := getJSON(t, r, "/metrics/derived/dichte?nuts_version=2024")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	var res models.DatasetMetricsResult
	_ = json.Unmarshal(resp.Data, &res)
	if len(res.ByYear) != 1 {
		t.Fatalf("result = %+v", res)
	}
	y := res.ByYear[0]
	// DE001: 1000/100 km² = 10, DE003: 3000/100 = 30.
	if y.RegionCount != 2 || y.Min != 10 || y.Max != 30 || y.ExcludedInvalidCount != 1 || y.RegionsWithoutGeometry != 1 {
		t.Errorf("year = %+v", y)
	}
	if res.ExcludedRegions != 2 || res.RegionsWithoutGeometry != 1 || res.ExcludedInvalidCount != 1 {
		t.Errorf("totals = %+v", res)
	}

	// Dataset denominator: DE002 denominator invalid, DE003 numerator invalid,
	// DE005 has no denominator row; only DE001 (10/1000*100 = 1) is used.
	code, resp = getJSON(t, r, "/metrics/derived/quote")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	res = models.DatasetMetricsResult{}
	_ = json.Unmarshal(resp.Data, &res)
	if len(res.ByYear) != 1 {
		t.Fatalf("result = %+v", res)
	}
	y = res.ByYear[0]
	if y.RegionCount != 1 || y.Mean != 1 || y.ExcludedInvalidCount != 2 || y.RegionsWithoutDenominator != 1 || res.ExcludedRegions != 3 {
		t.Errorf("quote = %+v / %+v", y, res)
	}
}

// Product formula: rate per 1,000 x reference count / 1,000. DE002 has an
// invalid factor, DE003 an invalid rate, DE005 no factor row (counted as
// regions_without_factor, not regions_without_denominator); DE001 (50 x 2000
// / 1000 = 100) and DE004 (20 x 4000 / 1000 = 80) are used.
func TestDBDerivedProductMetric(t *testing.T) {
	r := integrationRouter(openIntegrationDB(t))
	code, resp := getJSON(t, r, "/metrics/derived/azubi-absolut?nuts_version=2024")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	var res models.DatasetMetricsResult
	_ = json.Unmarshal(resp.Data, &res)
	if len(res.ByYear) != 1 || res.NutsVersion == nil || *res.NutsVersion != 2024 {
		t.Fatalf("result = %+v", res)
	}
	near := func(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
	y := res.ByYear[0]
	if y.RegionCount != 2 || !near(y.Min, 80) || !near(y.Max, 100) || !near(y.Mean, 90) ||
		y.ExcludedInvalidCount != 2 || y.RegionsWithoutFactor != 1 || y.RegionsWithoutDenominator != 0 ||
		res.RegionsWithoutFactor != 1 || res.RegionsWithoutDenominator != 0 || res.ExcludedRegions != 3 {
		t.Errorf("product = %+v / %+v", y, res)
	}
	if resp.Meta == nil || !strings.HasPrefix(resp.Meta.Note, metricsValidityNoteBase+" "+metricsProductNote) || !strings.HasSuffix(resp.Meta.Note, " Rückrechnung.") {
		t.Errorf("meta = %+v", resp.Meta)
	}

	// Regional value and rank: DE004 (80) is rank 1 of 2.
	code, resp = getJSON(t, r, "/metrics/derived/azubi-absolut?geodata_id_nuts=DE004")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, resp.Error)
	}
	res = models.DatasetMetricsResult{}
	_ = json.Unmarshal(resp.Data, &res)
	if len(res.ByYear) != 1 || res.ByYear[0].RegionalValue == nil || !near(*res.ByYear[0].RegionalValue, 80) ||
		*res.ByYear[0].RegionalRank != 1 || *res.ByYear[0].RegionalRankTotal != 2 {
		t.Errorf("regional = %+v", res)
	}
}

func postGeoQuery(t *testing.T, r *gin.Engine, body string) (int, metricsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/geo/query", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp metricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%v (%s)", err, w.Body.String())
	}
	return w.Code, resp
}

func TestDBGeoQueryRepairsAndClassifies(t *testing.T) {
	db := openIntegrationDB(t)
	r := integrationRouter(db)

	// WGS84 bounds of DE001 and DE003, to build a bowtie whose two lobes lie in
	// DE001 and DE003 (crossing over DE002).
	var b struct {
		X1 float64 `db:"x1"`
		Y1 float64 `db:"y1"`
		X2 float64 `db:"x2"`
		Y2 float64 `db:"y2"`
	}
	if err := db.Get(&b, `SELECT ST_X(ST_Centroid(a.geometry_wgs84)) x1, ST_Y(ST_Centroid(a.geometry_wgs84)) y1,
		ST_X(ST_Centroid(c.geometry_wgs84)) x2, ST_Y(ST_Centroid(c.geometry_wgs84)) y2
		FROM transformed_data.mart_nuts_geometry_historized a, transformed_data.mart_nuts_geometry_historized c
		WHERE a.nuts_id = 'DE001' AND c.nuts_id = 'DE003'`); err != nil {
		t.Fatal(err)
	}
	d := 0.02
	// Self-intersecting ring: (x1, y1-d) -> (x2, y2+d) -> (x2, y2-d) -> (x1, y1+d).
	bowtie := fmt.Sprintf(`{"type":"Polygon","coordinates":[[[%[1]f,%[2]f],[%[3]f,%[4]f],[%[3]f,%[5]f],[%[1]f,%[6]f],[%[1]f,%[2]f]]]}`,
		b.X1, b.Y1-d, b.X2, b.Y2+d, b.Y2-d, b.Y1+d)

	for _, op := range []string{"intersects", "contains"} {
		code, resp := postGeoQuery(t, r, `{"operation":"`+op+`","geometry":`+bowtie+`}`)
		if code != http.StatusOK {
			t.Fatalf("%s bowtie: status %d (%s), want 200 (repaired)", op, code, resp.Error)
		}
		var data []models.GeoRegionSummary
		_ = json.Unmarshal(resp.Data, &data)
		// Count and page query see the same repaired geometry.
		if resp.Meta == nil || resp.Meta.Total != len(data) {
			t.Errorf("%s: total %+v vs %d rows", op, resp.Meta, len(data))
		}
		if op == "intersects" && len(data) != 3 {
			t.Errorf("intersects bowtie: %d regions, want 3 (both lobes plus DE002 at the crossing)", len(data))
		}
	}

	// A point still works unchanged.
	code, resp := postGeoQuery(t, r, fmt.Sprintf(`{"geometry":{"type":"Point","coordinates":[%f,%f]}}`, b.X1, b.Y1))
	if code != http.StatusOK || resp.Meta == nil || resp.Meta.Total != 1 {
		t.Errorf("point: status %d meta %+v err %s", code, resp.Meta, resp.Error)
	}

	// Degenerate polygon (a spike back and forth: no area after repair in any
	// projection) -> 400 with fixed text.
	code, resp = postGeoQuery(t, r, `{"geometry":{"type":"Polygon","coordinates":[[[10,52],[10.1,52.1],[10,52],[10,52]]]}}`)
	if code != http.StatusBadRequest || !strings.Contains(resp.Error, "keine Fläche") {
		t.Errorf("degenerate polygon: status %d err %q", code, resp.Error)
	}

	// PostGIS parse error -> 400 with the fixed message, no PostGIS text.
	// (Go validation passes: a Polygon whose rings are positions, nested once
	// too deep for the type, is caught by PostGIS.)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := NewHandler(db, nil)
	_, inputErr, dbErr := h.checkGeoJSONInDB(ctx, "Polygon", 1, "", []interface{}{`{"type":"Polygon","coordinates":[[1,2]]}`})
	if dbErr != nil || inputErr == nil || inputErr.Error() != geoJSONParseErrorMessage {
		t.Errorf("parse error: inputErr=%v dbErr=%v", inputErr, dbErr)
	}
	// A broken count statement is a server error, not a 400.
	_, inputErr, dbErr = h.checkGeoJSONInDB(ctx, "Point", 1, "SELECT 1 FROM transformed_data.does_not_exist", []interface{}{`{"type":"Point","coordinates":[10,52]}`})
	if inputErr != nil || dbErr == nil {
		t.Errorf("missing table: inputErr=%v dbErr=%v", inputErr, dbErr)
	}
}

func TestDBNutsLimitNullWithChunkSize(t *testing.T) {
	r := integrationRouter(openIntegrationDB(t))
	prev := UnlimitedRowsCap()
	unlimitedRowsCap.Store(2)
	t.Cleanup(func() { unlimitedRowsCap.Store(prev) })

	if code, resp := getJSON(t, r, "/nuts?level=3&limit=null"); code != http.StatusBadRequest {
		t.Errorf("limit=null above the cap: status %d (%s), want 400", code, resp.Error)
	}
	code, resp := getJSON(t, r, "/nuts?level=3&limit=null&chunk_size=2&chunk=1")
	if code != http.StatusOK {
		t.Fatalf("limit=null with chunk_size: status %d (%s), want 200", code, resp.Error)
	}
	var data []models.NutsRegionSummary
	_ = json.Unmarshal(resp.Data, &data)
	if len(data) != 1 || data[0].NutsID != "DE003" || resp.Meta.Total != 3 {
		t.Errorf("chunk 1 = %+v meta %+v", data, resp.Meta)
	}
}
