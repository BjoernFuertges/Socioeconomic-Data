package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// More DB integration tests (opt-in via SOZ_TEST_PG_DSN, see
// db_integration_test.go): generic Kreis handler (fill_gaps, version choice),
// timeseries endpoint, Agenturbezirk, /geo (incl. batched include_datasets) and
// the LAU detail handler.

const handlersFixtureSQL = `
-- generic Kreis dataset (SozioBaseData columns + val)
CREATE TABLE transformed_data.stg_test_kreise (
	original_id_1 text, original_name text, geodata_name text, geodata_id_nuts text,
	geodata_id_ars bigint, mapper text, parameter text, geodata_level text,
	geodata_year integer, year integer, observation_date date, has_valid_value boolean, val double precision);
INSERT INTO transformed_data.stg_test_kreise VALUES
	('1', 'A', 'Region A', 'DE001', 1, 'exact_id', 'id_ars', 'NUTS_3', 2024, 2020, '2020-01-01', true, 5),
	('1', 'A', 'Region A', 'DE001', 1, 'exact_id', 'id_ars', 'NUTS_3', 2024, 2021, '2021-01-01', true, 6),
	('2', 'B', 'Region B', 'DE002', 2, 'exact_id', 'id_ars', 'NUTS_3', 2024, 2020, '2020-01-01', true, 7);

CREATE TABLE transformed_data.api_test_fill_gaps (
	original_id_1 text, original_name text, geodata_name text, geodata_id_nuts text,
	geodata_id_ars bigint, mapper text, parameter text, geodata_level text,
	geodata_year integer, year integer, observation_date date, has_valid_value boolean, val double precision,
	is_interpolated boolean, is_estimated boolean, value_linear double precision,
	value_peer_trend double precision, n_peer_regions integer, nuts_version integer);
INSERT INTO transformed_data.api_test_fill_gaps VALUES
	('1', 'A', 'Region A', 'DE001', 1, 'x', 'p', 'NUTS_3', 2024, 2020, '2020-01-01', true, 5, false, false, NULL, NULL, NULL, 2024),
	('1', 'A', 'Region A', 'DE001', 1, 'x', 'p', 'NUTS_3', 2024, 2021, '2021-01-01', true, 5.5, true, false, 5.5, 5.4, 3, 2024),
	('1', 'A', 'Region A', 'DE001', 1, 'x', 'p', 'NUTS_3', 2021, 2020, '2020-01-01', true, 4, false, true, NULL, NULL, NULL, 2021);

CREATE TABLE transformed_data.mart_nuts3_timeseries_interpolated (
	nuts3_id text, dataset_slug text, target_version integer, year integer, value double precision,
	is_interpolated boolean, is_estimated boolean, value_linear double precision,
	value_peer_trend double precision, n_peer_regions integer);
INSERT INTO transformed_data.mart_nuts3_timeseries_interpolated VALUES
	('DE001', 'schuldnerquote', 2024, 2020, 5, false, false, NULL, NULL, NULL),
	('DE001', 'schuldnerquote', 2024, 2021, 6, true, false, 6, 5.9, 4),
	('DE001', 'schuldnerquote', 2021, 2020, 4, false, true, NULL, NULL, NULL),
	('DE001', 'bevoelkerung',   2024, 2020, 1000, false, false, NULL, NULL, NULL);

CREATE TABLE transformed_data.mart_region_datasets (geodata_id_nuts text, dataset_slug text, year integer);
INSERT INTO transformed_data.mart_region_datasets VALUES
	('DE001', 'bevoelkerung', 2019), ('DE001', 'bevoelkerung', 2020), ('DE001', 'schuldnerquote', 2020),
	('DE002', 'bevoelkerung', 2020);

CREATE TABLE transformed_data.lau_integrated (
	gisco_id text, year integer, lau_name text, country_code text, lau_id text,
	geometry_wgs84 jsonb, geometry_3035 geometry, has_valid_geometry boolean,
	lau_name_national text, lau_name_latin text, nuts_3_code text, nuts3_id text, nuts3_link_source text,
	lau_nat_code text, population_count bigint, country text, has_valid_population boolean,
	missing_population_data boolean, year_match boolean);
INSERT INTO transformed_data.lau_integrated VALUES
	('DE_A1', 2024, 'Gemeinde A1', 'DE', 'A1', '{"type":"Point","coordinates":[13,52]}', NULL, false, NULL, NULL, NULL, 'DE001', 'declared', NULL, 100, 'DE', true, false, true),
	('DE_A2', 2024, 'Gemeinde A2', 'DE', 'A2', '{"type":"Point","coordinates":[13,52]}', NULL, false, NULL, NULL, NULL, 'DE002', 'declared', NULL, 200, 'DE', true, false, true),
	('DE_A3', 2024, 'Gemeinde A3', 'DE', 'A3', '{"type":"Point","coordinates":[13,52]}', NULL, false, NULL, NULL, NULL, NULL,    NULL,       NULL, 300, 'DE', true, false, true);

UPDATE transformed_data.lau_integrated SET has_valid_geometry = true,
	geometry_3035 = CASE gisco_id
		WHEN 'DE_A1' THEN ST_MakeEnvelope(4301000, 3251000, 4302000, 3252000, 3035)
		WHEN 'DE_A2' THEN ST_MakeEnvelope(4311000, 3251000, 4312000, 3252000, 3035)
		ELSE ST_MakeEnvelope(4321000, 3251000, 4322000, 3252000, 3035) END;

CREATE TABLE transformed_data.mart_agenturbezirk_sociodata (
	ba_schl text, name text, geometry_wgs84 jsonb, has_valid_geometry boolean,
	valid_from date, valid_to date, year integer, month integer, observation_date date,
	arbeitslose double precision, unterbeschaeftigung double precision);
INSERT INTO transformed_data.mart_agenturbezirk_sociodata VALUES
	('962', 'Berlin Mitte', '{"type":"Point","coordinates":[13,52]}', false, '2019-06-01', NULL, 2024, 1, '2024-01-01', 100, 120),
	('962', 'Berlin Mitte', '{"type":"Point","coordinates":[13,52]}', false, '2019-06-01', NULL, 2024, 2, '2024-02-01', NULL, 130),
	('963', 'Berlin Nord',  '{"type":"Point","coordinates":[13,52]}', false, '2019-06-01', NULL, 2024, 1, '2024-01-01', 50, 60);
`

func handlersTestRouter(t *testing.T) (*gin.Engine, *sqlx.DB) {
	t.Helper()
	db := openIntegrationDB(t)
	if _, err := db.Exec(handlersFixtureSQL); err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	setTargetVersionsForTest(t, []int{2021, 2024}, []int{2024})
	gin.SetMode(gin.TestMode)
	h := NewHandler(db, models.DerivedMetricsCatalog{})

	type testData struct {
		models.SozioBaseData
		Val float64 `json:"val" db:"val"`
	}
	plain := models.KreisDatasetConfig{
		Schema: "transformed_data", Table: "stg_test_kreise", ValueColumns: []string{"val"},
		DefaultLimit: 1000, OrderBy: "geodata_id_nuts, year", RouteSlug: "test",
	}
	filled := plain
	filled.InterpolatedTable = "api_test_fill_gaps"
	filled.IsHarmonized = true
	harmonized := models.KreisDatasetConfig{RouteSlug: "schuldnerquote", IsHarmonized: true, DefaultLimit: 1000}

	r := gin.New()
	r.GET("/plain", func(c *gin.Context) { getKreisData[testData](h, c, plain) })
	r.GET("/filled", func(c *gin.Context) { getKreisData[testData](h, c, filled) })
	r.GET("/ts", func(c *gin.Context) { getKreisTimeseriesEndpoint(h, c, harmonized) })
	r.GET("/ts-plain", func(c *gin.Context) { getKreisTimeseriesEndpoint(h, c, plain) })
	r.GET("/agenturbezirke", h.GetAgenturbezirke)
	r.GET("/agenturbezirke/:ba_schl", h.GetAgenturbezirkByID)
	r.GET("/agenturbezirke/:ba_schl/arbeitslosigkeit", h.GetAgenturbezirkArbeitslosigkeit)
	r.GET("/agenturbezirke/:ba_schl/dataset-aggregate/:slug", h.GetAgenturbezirkDatasetAggregateTimeseries)
	r.GET("/geo", h.GetGeoRegions)
	r.POST("/geo/query", h.QueryGeoRegionsByGeometry)
	r.GET("/geo/:geo_id", h.GetGeoRegionByID)
	r.GET("/lau/:gisco_id", h.GetLAURegionByID)
	return r, db
}

func rawGet(t *testing.T, r *gin.Engine, target string) (int, metricsResponse) {
	t.Helper()
	return getJSON(t, r, target)
}

func rowsOf(t *testing.T, resp metricsResponse) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		t.Fatalf("data is not a list: %v (%s)", err, resp.Data)
	}
	return rows
}

func TestDBKreisHandlerPlainAndFillGaps(t *testing.T) {
	r, _ := handlersTestRouter(t)

	code, resp := rawGet(t, r, "/plain?geodata_id_nuts=DE001")
	if code != 200 || len(rowsOf(t, resp)) != 2 {
		t.Fatalf("plain: %d %s", code, resp.Data)
	}

	// fill_gaps: default = newest nuts_version (2024), explicit version selectable.
	code, resp = rawGet(t, r, "/filled?fill_gaps=true&geodata_id_nuts=DE001")
	if rows := rowsOf(t, resp); code != 200 || len(rows) != 2 || rows[0]["nuts_version"].(float64) != 2024 {
		t.Fatalf("fill_gaps default version: %d %s", code, resp.Data)
	}
	code, resp = rawGet(t, r, "/filled?fill_gaps=true&geodata_id_nuts=DE001&nuts_version=2021")
	if rows := rowsOf(t, resp); code != 200 || len(rows) != 1 || rows[0]["is_estimated"] != true {
		t.Fatalf("fill_gaps nuts_version=2021: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/filled?fill_gaps=true&nuts_version=1999"); code != 400 {
		t.Errorf("unsupported nuts_version: %d, want 400", code)
	}
	// dataset without a fill_gaps view
	if code, _ = rawGet(t, r, "/plain?fill_gaps=true"); code != 400 {
		t.Errorf("fill_gaps without view: %d, want 400", code)
	}
}

func TestDBKreisTimeseriesVersionAndScope(t *testing.T) {
	r, _ := handlersTestRouter(t)

	code, resp := rawGet(t, r, "/ts?geodata_id_nuts=DE001")
	rows := rowsOf(t, resp)
	if code != 200 || len(rows) != 2 || rows[0]["nuts_version"].(float64) != 2024 {
		t.Fatalf("default version: %d %s", code, resp.Data)
	}
	// only the requested dataset, not bevoelkerung
	for _, row := range rows {
		if row["dataset_slug"] != "schuldnerquote" {
			t.Errorf("foreign dataset in result: %v", row)
		}
	}
	code, resp = rawGet(t, r, "/ts?geodata_id_nuts=DE001&nuts_version=2021")
	if rows := rowsOf(t, resp); code != 200 || len(rows) != 1 || rows[0]["is_estimated"] != true {
		t.Fatalf("nuts_version=2021: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/ts?geodata_id_nuts=DE001&nuts_version=1999"); code != 400 {
		t.Errorf("unsupported version: %d, want 400", code)
	}
	if code, _ = rawGet(t, r, "/ts"); code != 400 {
		t.Errorf("unbounded request: %d, want 400", code)
	}
	if code, _ = rawGet(t, r, "/ts?year=2020&point=10,52"); code != 400 {
		t.Errorf("point with fill_gaps: %d, want 400", code)
	}
	if code, _ = rawGet(t, r, "/ts-plain?year=2020"); code != 400 {
		t.Errorf("non-harmonized dataset: %d, want 400", code)
	}
}

func TestDBAgenturbezirkHandlers(t *testing.T) {
	r, _ := handlersTestRouter(t)

	code, resp := rawGet(t, r, "/agenturbezirke")
	if code != 200 || len(rowsOf(t, resp)) != 2 {
		t.Fatalf("list: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/agenturbezirke/962"); code != 200 {
		t.Errorf("detail: %d", code)
	}
	if code, _ = rawGet(t, r, "/agenturbezirke/000"); code != 404 {
		t.Errorf("unknown: %d, want 404", code)
	}

	code, resp = rawGet(t, r, "/agenturbezirke/962/arbeitslosigkeit")
	rows := rowsOf(t, resp)
	if code != 200 || len(rows) != 2 || !strings.Contains(string(resp.Data), `"arbeitslose":{"Float64":0,"Valid":false}`) {
		t.Fatalf("series (suppressed value must stay invalid/null): %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/agenturbezirke/000/arbeitslosigkeit"); code != 404 {
		t.Errorf("series unknown: %d, want 404", code)
	}
	if code, _ = rawGet(t, r, "/agenturbezirke/000/dataset-aggregate/bevoelkerung"); code != 404 {
		t.Errorf("aggregate unknown: %d, want 404", code)
	}
	if code, _ = rawGet(t, r, "/agenturbezirke/962/dataset-aggregate/gibt-es-nicht"); code != 400 {
		t.Errorf("aggregate unknown slug: %d, want 400", code)
	}
}

func TestDBGeoQueryIncludeDatasetsBatched(t *testing.T) {
	r, db := handlersTestRouter(t)

	// Polygon covering the three fixture squares (and the LAU squares in them).
	var polygon string
	if err := db.Get(&polygon, `SELECT ST_AsGeoJSON(ST_Transform(ST_MakeEnvelope(4299000, 3249000, 4331000, 3261000, 3035), 4326))`); err != nil {
		t.Fatal(err)
	}
	datasetsPerRegion := func(body string) map[string][]any {
		t.Helper()
		code, resp := postGeoQuery(t, r, body)
		if code != 200 {
			t.Fatalf("%s: %d %s", body, code, resp.Error)
		}
		out := map[string][]any{}
		for _, row := range rowsOf(t, resp) {
			ds, _ := row["datasets"].([]any)
			out[row["geo_id"].(string)] = ds
		}
		return out
	}

	// NUTS: all Kreise in one go; DE003 has no datasets.
	byID := datasetsPerRegion(`{"geo_type":"nuts","operation":"intersects","include_datasets":true,"geometry":` + polygon + `}`)
	if len(byID) != 3 || len(byID["DE001"]) != 2 || len(byID["DE002"]) != 1 || len(byID["DE003"]) != 0 {
		t.Errorf("datasets per Kreis wrong: %v", byID)
	}
	if ds := byID["DE001"]; len(ds) > 0 && ds[0].(map[string]any)["available_years"] == nil {
		t.Errorf("available_years missing: %v", ds[0])
	}

	// LAU: datasets via nuts3_id; DE_A3 has no nuts3_id.
	byID = datasetsPerRegion(`{"geo_type":"lau","operation":"intersects","include_datasets":true,"geometry":` + polygon + `}`)
	if len(byID) != 3 || len(byID["DE_A1"]) != 2 || len(byID["DE_A2"]) != 1 || len(byID["DE_A3"]) != 0 {
		t.Errorf("datasets per LAU wrong: %v", byID)
	}

	// without include_datasets nothing is embedded
	_, resp := postGeoQuery(t, r, `{"geo_type":"nuts","operation":"intersects","geometry":`+polygon+`}`)
	for _, row := range rowsOf(t, resp) {
		if _, ok := row["datasets"]; ok {
			t.Errorf("datasets embedded without include_datasets: %v", row)
		}
	}
}

func TestDBGeoList(t *testing.T) {
	r, _ := handlersTestRouter(t)

	code, resp := rawGet(t, r, "/geo?geo_type=nuts&level=3")
	if rows := rowsOf(t, resp); code != 200 || len(rows) != 3 {
		t.Errorf("nuts list: %d %s", code, resp.Data)
	}
	if code, resp = rawGet(t, r, "/geo?geo_type=lau"); code != 200 || len(rowsOf(t, resp)) != 3 {
		t.Errorf("lau list: %d %s", code, resp.Data)
	}
	if code, resp = rawGet(t, r, "/geo?geo_type=agentur"); code != 200 || len(rowsOf(t, resp)) != 2 {
		t.Errorf("agentur list: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/geo?geo_type=bogus"); code != 400 {
		t.Errorf("invalid geo_type: %d, want 400", code)
	}
}

func TestDBGeoDetailDelegation(t *testing.T) {
	r, _ := handlersTestRouter(t)

	// purely numeric ID -> Agenturbezirk
	code, resp := rawGet(t, r, "/geo/962")
	if code != 200 || !strings.Contains(string(resp.Data), "Berlin Mitte") {
		t.Errorf("agentur via /geo: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/geo/000"); code != 404 {
		t.Errorf("unknown agentur: %d, want 404", code)
	}
	// LAU (explicit geo_type; a 5-character ID would be auto-detected as NUTS)
	if code, resp = rawGet(t, r, "/geo/DE_A1?geo_type=lau"); code != 200 || !strings.Contains(string(resp.Data), "Gemeinde A1") {
		t.Errorf("lau via /geo: %d %s", code, resp.Data)
	}
}

func TestDBLAURegionByID(t *testing.T) {
	r, _ := handlersTestRouter(t)

	code, resp := rawGet(t, r, "/lau/DE_A1?include_datasets=true")
	if code != 200 || !strings.Contains(string(resp.Data), `"datasets"`) {
		t.Fatalf("detail with datasets: %d %s", code, resp.Data)
	}
	// lower-case ID is normalized like in the other LAU handlers
	if code, resp = rawGet(t, r, "/lau/de_a1"); code != 200 {
		t.Errorf("lower-case gisco_id: %d %s", code, resp.Data)
	}
	// no NUTS-3 link -> no datasets, but still 200
	code, resp = rawGet(t, r, "/lau/DE_A3?include_datasets=true")
	if code != 200 || strings.Contains(string(resp.Data), `"datasets"`) {
		t.Errorf("without nuts3_id: %d %s", code, resp.Data)
	}
	if code, _ = rawGet(t, r, "/lau/DE_ZZ"); code != 404 {
		t.Errorf("unknown: %d, want 404", code)
	}
	if code, _ = rawGet(t, r, "/lau/DE_A1?year=abc"); code != 400 {
		t.Errorf("invalid year: %d, want 400", code)
	}
}
