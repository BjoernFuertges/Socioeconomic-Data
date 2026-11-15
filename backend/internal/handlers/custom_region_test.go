package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const testPolygon = `{"type":"Polygon","coordinates":[[[13,52],[14,52],[14,53],[13,53],[13,52]]]}`

func TestNormalizeCustomRegionRequest(t *testing.T) {
	from, to := 2020, 2010
	badShare := 1.0
	okShare := 0.2

	tests := []struct {
		name      string
		req       CustomRegionAggregateRequest
		wantErr   string
		wantSlugs []string
		wantShare float64
	}{
		{"ok polygon, dedup and trim", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(testPolygon), Datasets: []string{" bevoelkerung", "bevoelkerung", ""},
		}, "", []string{"bevoelkerung"}, customRegionDefaultShare},
		{"feature wrapper accepted", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(`{"type":"Feature","geometry":` + testPolygon + `}`), Datasets: []string{"a"},
		}, "", []string{"a"}, customRegionDefaultShare},
		{"custom min_share", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(testPolygon), Datasets: []string{"a"}, MinShare: &okShare,
		}, "", []string{"a"}, 0.2},
		{"missing geometry", CustomRegionAggregateRequest{Datasets: []string{"a"}}, "geometry ist erforderlich", nil, 0},
		{"point rejected", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(`{"type":"Point","coordinates":[13,52]}`), Datasets: []string{"a"},
		}, "Polygon oder MultiPolygon", nil, 0},
		{"no datasets", CustomRegionAggregateRequest{Geometry: json.RawMessage(testPolygon)}, "datasets ist erforderlich", nil, 0},
		{"year range", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(testPolygon), Datasets: []string{"a"}, FromYear: &from, ToYear: &to,
		}, "from_year", nil, 0},
		{"min_share out of range", CustomRegionAggregateRequest{
			Geometry: json.RawMessage(testPolygon), Datasets: []string{"a"}, MinShare: &badShare,
		}, "min_share", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, slugs, share, err := normalizeCustomRegionRequest(&tt.req)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("erwartete Fehler mit %q, erhielt %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unerwarteter Fehler: %v", err)
			}
			if strings.Join(slugs, ",") != strings.Join(tt.wantSlugs, ",") {
				t.Errorf("slugs = %v, erwartet %v", slugs, tt.wantSlugs)
			}
			if share != tt.wantShare {
				t.Errorf("min_share = %v, erwartet %v", share, tt.wantShare)
			}
		})
	}
}

func nf(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }

func TestComputeCustomRegionValue(t *testing.T) {
	tests := []struct {
		name       string
		measure    string
		row        customRegionAggRow
		wantValue  float64
		wantMethod string
		wantOK     bool
	}{
		{"sum", "sum", customRegionAggRow{SumValue: nf(150)}, 150, "sum", true},
		{"sum without value", "sum", customRegionAggRow{}, 0, "", false},
		{"avg by population", "weighted_avg", customRegionAggRow{
			PopNum: nf(30), PopDen: nf(10), PopComplete: true, AreaNum: nf(1), AreaDen: nf(1), PlainAvg: nf(9),
		}, 3, "weighted_avg_population", true},
		{"avg falls back to area", "weighted_avg", customRegionAggRow{
			PopNum: nf(30), PopDen: nf(10), PopComplete: false, AreaNum: nf(8), AreaDen: nf(2), PlainAvg: nf(9),
		}, 4, "weighted_avg_area", true},
		{"avg falls back to plain", "weighted_avg", customRegionAggRow{PlainAvg: nf(9)}, 9, "avg", true},
		{"avg nothing", "weighted_avg", customRegionAggRow{}, 0, "", false},
		{"avg by employees", "weighted_avg_employees", customRegionAggRow{
			EmpNum: nf(60), EmpDen: nf(40), EmpComplete: true, PopNum: nf(30), PopDen: nf(10), PopComplete: true,
			AreaNum: nf(1), AreaDen: nf(1), PlainAvg: nf(9),
		}, 1.5, "weighted_avg_employees", true},
		{"employees fall back to population", "weighted_avg_employees", customRegionAggRow{
			EmpNum: nf(60), EmpDen: nf(40), EmpComplete: false, PopNum: nf(30), PopDen: nf(10), PopComplete: true,
			AreaNum: nf(8), AreaDen: nf(2), PlainAvg: nf(9),
		}, 3, "weighted_avg_population", true},
		{"employees fall back to area without population", "weighted_avg_employees", customRegionAggRow{
			EmpNum: nf(60), EmpDen: nf(40), EmpComplete: false, PopComplete: false,
			AreaNum: nf(8), AreaDen: nf(2), PlainAvg: nf(9),
		}, 4, "weighted_avg_area", true},
		{"employees fall back to plain", "weighted_avg_employees", customRegionAggRow{PlainAvg: nf(9)}, 9, "avg", true},
		{"population weights ignored for sum", "sum", customRegionAggRow{SumValue: nf(7), EmpNum: nf(1), EmpDen: nf(1), EmpComplete: true}, 7, "sum", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, m, ok := computeCustomRegionValue(tt.measure, tt.row)
			if ok != tt.wantOK || m != tt.wantMethod || math.Abs(v-tt.wantValue) > 1e-9 {
				t.Errorf("got (%v, %q, %v), erwartet (%v, %q, %v)", v, m, ok, tt.wantValue, tt.wantMethod, tt.wantOK)
			}
		})
	}
}

func TestCustomRegionLevelChain(t *testing.T) {
	finer := "beschaeftigungsquote-gemeinden"
	nuts := customRegionMeta{DatasetSlug: "beschaeftigungsquote-nuts3", Indicator: "beschaeftigungsquote-nuts3",
		Level: "nuts3", FinerSlug: &finer}
	lau := customRegionMeta{DatasetSlug: "beschaeftigungsquote-gemeinden", Indicator: "beschaeftigungsquote-gemeinden", Level: "lau"}
	plain := customRegionMeta{DatasetSlug: "bevoelkerung", Indicator: "bevoelkerung", Level: "nuts3"}
	bySlug := map[string][]customRegionMeta{
		nuts.DatasetSlug: {nuts}, lau.DatasetSlug: {lau}, plain.DatasetSlug: {plain},
	}

	chain := customRegionLevelChain(nuts, bySlug)
	if len(chain) != 2 || chain[0].Level != "lau" || chain[1].Level != "nuts3" {
		t.Errorf("feinste Ebene muss zuerst kommen, erhielt %+v", chain)
	}
	if chain := customRegionLevelChain(plain, bySlug); len(chain) != 1 {
		t.Errorf("ohne finer_slug nur ein Kandidat, erhielt %+v", chain)
	}
}

func TestCustomRegionUnitsSQLLevels(t *testing.T) {
	for _, level := range []string{"nuts3", "lau", "agentur"} {
		sqlText, err := customRegionUnitsSQL(level)
		if err != nil || !strings.Contains(sqlText, "unit_id") || !strings.Contains(sqlText, " AS g") {
			t.Errorf("level %q: %q, %v", level, sqlText, err)
		}
	}
	for _, level := range []string{"", "NUTS3", "region"} {
		if sqlText, err := customRegionUnitsSQL(level); err == nil || sqlText != "" {
			t.Errorf("unknown level %q must be an error, got %q, %v", level, sqlText, err)
		}
	}
}

// Every level that mart_dataset_aggregation_meta can emit (the
// `{% set <level>_datasets %}` lists) needs a units fragment; otherwise a new
// level would only fail at request time. Skipped when the dbt sources are not
// part of the checkout.
func TestCustomRegionUnitsSQLCoversAggregationMetaLevels(t *testing.T) {
	b, err := os.ReadFile("../../../pipeline/dbt/socioeconomic_data_pipeline/models/marts/mart_dataset_aggregation_meta.sql")
	if err != nil {
		t.Skipf("dbt-Quelle nicht verfügbar: %v", err)
	}
	// 'level' column literals in the VALUES rows: ..., '<level>', '{{ ds.measure_type }}'
	levels := map[string]bool{}
	for _, m := range regexp.MustCompile(`'\{\{ ds\.(?:slug|indicator) \}\}', '(\w+)', '\{\{ ds\.measure_type`).FindAllStringSubmatch(string(b), -1) {
		levels[m[1]] = true
	}
	if len(levels) == 0 {
		t.Fatal("keine Level in mart_dataset_aggregation_meta.sql gefunden (Regex veraltet?)")
	}
	for level := range levels {
		if _, err := customRegionUnitsSQL(level); err != nil {
			t.Errorf("mart_dataset_aggregation_meta liefert Level %q, customRegionUnitsSQL kennt es nicht", level)
		}
	}
}

func TestCustomRegionBudget(t *testing.T) {
	t.Setenv("CUSTOM_REGION_TIMEOUT_SECONDS", "")
	if got := customRegionBudget(); got != customRegionDefaultBudget {
		t.Errorf("default = %v", got)
	}
	t.Setenv("CUSTOM_REGION_TIMEOUT_SECONDS", "5")
	if got := customRegionBudget(); got != 5*time.Second {
		t.Errorf("override = %v", got)
	}
	for _, bad := range []string{"0", "-3", "abc"} {
		t.Setenv("CUSTOM_REGION_TIMEOUT_SECONDS", bad)
		if got := customRegionBudget(); got != customRegionDefaultBudget {
			t.Errorf("invalid %q must fall back to the default, got %v", bad, got)
		}
	}
}

func TestCustomRegionPlannedQueries(t *testing.T) {
	finer := "b-gemeinden"
	bySlug := map[string][]customRegionMeta{
		"b-nuts3":     {{DatasetSlug: "b-nuts3", Indicator: "b-nuts3", Level: "nuts3", FinerSlug: &finer}},
		"b-gemeinden": {{DatasetSlug: "b-gemeinden", Indicator: "b-gemeinden", Level: "lau"}},
		"agentur":     {{DatasetSlug: "agentur", Indicator: "a1"}, {DatasetSlug: "agentur", Indicator: "a2"}},
	}
	// b-nuts3: LAU + NUTS-3 (2); agentur: 2 indicators (2)
	if got := customRegionPlannedQueries([]string{"b-nuts3", "agentur"}, bySlug); got != 4 {
		t.Errorf("planned = %d, want 4", got)
	}
}

// A request that expands to too many sub-queries is rejected with a generic
// 400 before any aggregation query runs.
func TestDBCustomRegionQueryCap(t *testing.T) {
	db := openIntegrationDB(t)
	if _, err := db.Exec(`CREATE TABLE transformed_data.mart_dataset_aggregation_meta (
		dataset_slug text, indicator text, level text, measure_type text, kind text, finer_slug text, employee_slug text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`INSERT INTO transformed_data.mart_dataset_aggregation_meta
		SELECT 'many', 'ind' || g, 'agentur', 'sum', 'count', NULL, NULL FROM generate_series(1, %d) g`, customRegionMaxQueries+1)); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	h := NewHandler(db, models.DerivedMetricsCatalog{})
	r := gin.New()
	r.POST("/custom-regions/dataset-aggregate", h.QueryCustomRegionDatasetAggregate)

	body := `{"geometry":` + testPolygon + `,"datasets":["many"]}`
	req := httptest.NewRequest(http.MethodPost, "/custom-regions/dataset-aggregate", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "zu viele Teilabfragen") {
		t.Errorf("status %d: %s", w.Code, w.Body.String())
	}
}
