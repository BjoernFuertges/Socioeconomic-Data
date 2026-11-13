package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"strings"
	"testing"
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
