package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

func mustConfig(t *testing.T, slug string) models.KreisDatasetConfig {
	t.Helper()
	cfg, ok := models.DatasetConfigBySlug(slug)
	if !ok {
		t.Fatalf("unknown dataset %q", slug)
	}
	return cfg
}

func TestDatasetMetricsSource(t *testing.T) {
	tests := []struct {
		slug           string
		col            string
		wantHarmonized bool
	}{
		{"schuldnerquote", "schuldnerquote_prozent", true},
		{"bevoelkerung", "bevoelkerung", true},
		{"beschaeftigungsquote-nuts3", "beschaeftigungsquote_prozent", true},
		// No harmonized counterpart: these stay on the staging table.
		{"beschaeftigungsquote-nuts3", "beschaeftigungsquote_median", false},
		{"beschaeftigungsquote-nuts3", "gemeinden_count", false},
		{"beschaeftigungsquote-gemeinden", "beschaeftigungsquote_prozent", false},
	}
	for _, tt := range tests {
		t.Run(tt.slug+"/"+tt.col, func(t *testing.T) {
			cfg := mustConfig(t, tt.slug)
			src := datasetMetricsSource(cfg, tt.col, 2021)
			if tt.wantHarmonized {
				if !strings.Contains(src.SQL, "mart_nuts3_sociodata_harmonized") || !strings.Contains(src.SQL, "target_version = $2") {
					t.Fatalf("expected harmonized source, got %s", src.SQL)
				}
				if len(src.Args) != 2 || src.Args[0] != cfg.RouteSlug || src.Args[1] != 2021 {
					t.Errorf("args = %v, want [%s 2021]", src.Args, cfg.RouteSlug)
				}
				if src.NutsVersion == nil || *src.NutsVersion != 2021 {
					t.Errorf("NutsVersion = %v, want 2021", src.NutsVersion)
				}
				return
			}
			if !strings.Contains(src.SQL, cfg.Table) || !strings.Contains(src.SQL, `"`+tt.col+`"`) || len(src.Args) != 0 {
				t.Errorf("expected staging source on %s.%s, got %s %v", cfg.Table, tt.col, src.SQL, src.Args)
			}
			if src.NutsVersion != nil {
				t.Errorf("staging source must not claim a nuts_version, got %d", *src.NutsVersion)
			}
		})
	}
}

// The filter placeholders must continue after the source's own args.
func TestMetricsQueriesArgNumbering(t *testing.T) {
	year := 2022
	filter := models.MetricsFilter{Year: &year, NutsParentID: "de1", GeodataIDNUTS: "DE111"}
	src := datasetMetricsSource(mustConfig(t, "schuldnerquote"), "schuldnerquote_prozent", 2024)

	agg, aggArgs := metricsAggregateQuery(src, filter)
	for _, want := range []string{"geodata_id_nuts = $3", "geodata_id_nuts LIKE $4", "year = $5"} {
		if !strings.Contains(agg, want) {
			t.Errorf("aggregate query lacks %q: %s", want, agg)
		}
	}
	if len(aggArgs) != 5 || aggArgs[3] != "DE1%" {
		t.Errorf("aggregate args = %v", aggArgs)
	}

	rank, rankArgs := metricsRankQuery(src, filter)
	if strings.Contains(rank, "geodata_id_nuts = $3 ") {
		t.Errorf("ranking must not filter on the region itself before ranking: %s", rank)
	}
	if !strings.Contains(rank, "WHERE geodata_id_nuts = $5") || len(rankArgs) != 5 || rankArgs[4] != "DE111" {
		t.Errorf("rank query/args wrong: %s %v", rank, rankArgs)
	}
}

func derivedEntry(num, numCol, den, denCol, builtin string) models.DerivedMetricEntry {
	var e models.DerivedMetricEntry
	e.Slug = "test"
	e.Formula.Type = "ratio"
	e.Formula.Numerator.Dataset = num
	e.Formula.Numerator.Column = numCol
	e.Formula.Denominator.Dataset = den
	e.Formula.Denominator.Column = denCol
	e.Formula.Denominator.Builtin = builtin
	e.Formula.Scale = 100
	return e
}

// derivedProductEntry is a product formula num x factor x scale.
func derivedProductEntry(num, numCol, factor, factorCol string, scale float64) models.DerivedMetricEntry {
	var e models.DerivedMetricEntry
	e.Slug = "test-product"
	e.Formula.Type = "product"
	e.Formula.Numerator.Dataset = num
	e.Formula.Numerator.Column = numCol
	e.Formula.Factor.Dataset = factor
	e.Formula.Factor.Column = factorCol
	e.Formula.Scale = scale
	return e
}

func TestDerivedMetricSource(t *testing.T) {
	t.Run("area of the same NUTS version", func(t *testing.T) {
		src, err := derivedMetricSource(derivedEntry("bevoelkerung", "bevoelkerung", "", "", "nuts_geometry_area_sqkm"), 2016)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"mart_nuts3_sociodata_harmonized n", "valid_from <= $2::int AND max_vintage_year >= $2::int", "levl_code = 3", "* 100) AS value",
			"LEFT JOIN (", "THEN 'no_geometry'", "NOT (n.has_valid_value)"} {
			if !strings.Contains(src.SQL, want) {
				t.Errorf("SQL lacks %q: %s", want, src.SQL)
			}
		}
		if len(src.Args) != 2 || src.Args[1] != 2016 || src.NutsVersion == nil || *src.NutsVersion != 2016 {
			t.Errorf("args=%v version=%v", src.Args, src.NutsVersion)
		}
	})

	t.Run("dataset denominator within one target_version", func(t *testing.T) {
		src, err := derivedMetricSource(derivedEntry("arbeitslose", "arbeitslose", "bevoelkerung", "bevoelkerung", ""), 2024)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"LEFT JOIN transformed_data.mart_nuts3_sociodata_harmonized dc",
			"dc.target_version = n.target_version AND dc.dataset_slug = $3",
			"THEN 'no_denominator'", "NOT (dc.has_valid_value)"} {
			if !strings.Contains(src.SQL, want) {
				t.Errorf("denominator join lacks %q: %s", want, src.SQL)
			}
		}
		if len(src.Args) != 3 || src.Args[0] != "arbeitslose" || src.Args[2] != "bevoelkerung" {
			t.Errorf("args = %v", src.Args)
		}
	})

	t.Run("non-harmonized part falls back to staging", func(t *testing.T) {
		src, err := derivedMetricSource(derivedEntry("beschaeftigungsquote-nuts3", "gemeinden_count", "bevoelkerung", "bevoelkerung", ""), 2024)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(src.SQL, "harmonized") || src.NutsVersion != nil || len(src.Args) != 0 {
			t.Errorf("expected staging fallback, got %s %v", src.SQL, src.Args)
		}
		// Invalid rows are no longer filtered in the join but marked, so
		// they can be counted.
		if strings.Contains(src.SQL, "INNER JOIN") || !strings.Contains(src.SQL, "NOT (dc.has_valid_value)") || !strings.Contains(src.SQL, "excluded_reason") {
			t.Errorf("staging source must LEFT JOIN and mark excluded rows: %s", src.SQL)
		}
	})

	t.Run("product joins the factor like a denominator and multiplies", func(t *testing.T) {
		src, err := derivedMetricSource(derivedProductEntry("auszubildende", "auszubildende", "sv_beschaeftigte", "sv_beschaeftigte", 0.001), 2021)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"((n.harmonized_value::float8 * dc.harmonized_value::float8) * 0.001) AS value",
			"LEFT JOIN transformed_data.mart_nuts3_sociodata_harmonized dc",
			"dc.target_version = n.target_version AND dc.dataset_slug = $3",
			"THEN 'no_factor'", "NOT (n.has_valid_value) OR NOT (dc.has_valid_value)"} {
			if !strings.Contains(src.SQL, want) {
				t.Errorf("product SQL lacks %q: %s", want, src.SQL)
			}
		}
		if strings.Contains(src.SQL, "no_denominator") {
			t.Errorf("a product must mark a missing factor as no_factor: %s", src.SQL)
		}
		if strings.Contains(src.SQL, "NULLIF") {
			t.Errorf("a product must not treat a zero factor as missing: %s", src.SQL)
		}
		if len(src.Args) != 3 || src.Args[0] != "auszubildende" || src.Args[1] != 2021 || src.Args[2] != "sv_beschaeftigte" || src.NutsVersion == nil || *src.NutsVersion != 2021 {
			t.Errorf("args=%v version=%v", src.Args, src.NutsVersion)
		}
	})

	t.Run("product on the staging fallback", func(t *testing.T) {
		src, err := derivedMetricSource(derivedProductEntry("beschaeftigungsquote-nuts3", "gemeinden_count", "bevoelkerung", "bevoelkerung", 2), 2024)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(src.SQL, `((n."gemeinden_count" * dc."bevoelkerung") * 2) AS value`) || !strings.Contains(src.SQL, "THEN 'no_factor'") || src.NutsVersion != nil {
			t.Errorf("staging product SQL wrong: %s", src.SQL)
		}
	})

	for name, e := range map[string]models.DerivedMetricEntry{
		"product with builtin factor": func() models.DerivedMetricEntry {
			e := derivedProductEntry("bevoelkerung", "bevoelkerung", "", "", 1)
			e.Formula.Factor.Builtin = "nuts_geometry_area_sqkm"
			return e
		}(),
		"product without factor": func() models.DerivedMetricEntry {
			// A denominator is not a factor: product only reads Factor.
			e := derivedProductEntry("auszubildende", "auszubildende", "", "", 1)
			e.Formula.Denominator.Dataset = "sv_beschaeftigte"
			e.Formula.Denominator.Column = "sv_beschaeftigte"
			return e
		}(),
		"product factor column not allowed": derivedProductEntry("auszubildende", "auszubildende", "sv_beschaeftigte", "x", 1),
		"unknown formula": func() models.DerivedMetricEntry {
			e := derivedEntry("bevoelkerung", "bevoelkerung", "", "", "nuts_geometry_area_sqkm")
			e.Formula.Type = "sum"
			return e
		}(),
		"unknown numerator":  derivedEntry("nope", "x", "", "", "nuts_geometry_area_sqkm"),
		"column not allowed": derivedEntry("bevoelkerung", "drop table", "", "", "nuts_geometry_area_sqkm"),
		"no denominator":     derivedEntry("bevoelkerung", "bevoelkerung", "", "", ""),
	} {
		if _, err := derivedMetricSource(e, 2024); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}

// Every entry of the shipped catalog must resolve to a source (known datasets,
// allowed columns, supported formula type), harmonized for the default version.
func TestShippedDerivedCatalogResolves(t *testing.T) {
	t.Chdir("../..")
	cat, err := models.LoadMetricsCatalog("metrics_catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cat {
		src, err := derivedMetricSource(e, defaultNutsVersion)
		if err != nil {
			t.Errorf("%s: %v", e.Slug, err)
			continue
		}
		if src.NutsVersion == nil {
			t.Errorf("%s: expected harmonized source", e.Slug)
		}
	}
	e, ok := cat.FindBySlug("auszubildende-absolut")
	if !ok || e.Formula.Type != "product" || e.Unit != "Personen" || e.Note == "" ||
		!strings.Contains(e.Description, "Eigene Berechnung aus BBSR auszubildende und BA sv_beschaeftigte") {
		t.Errorf("auszubildende-absolut = %+v (found %v)", e, ok)
	}
	// GET /metrics/derived: a product shows its factor, no empty denominator.
	if js, _ := json.Marshal(e.Formula); !strings.Contains(string(js), `"factor":{"dataset":"sv_beschaeftigte"`) || strings.Contains(string(js), "denominator") {
		t.Errorf("formula JSON = %s", js)
	}
	// meta.note of a product names the factor rule, not the denominator rule.
	if note := derivedMetricNote(e); !strings.HasPrefix(note, metricsValidityNoteBase+" "+metricsProductNote) ||
		strings.Contains(note, "Nenner") || !strings.Contains(note, "regions_without_factor") || !strings.Contains(note, "Rückrechnung") {
		t.Errorf("meta.note = %q", note)
	}
	if d, ok := cat.FindBySlug("bevoelkerungsdichte"); !ok || derivedMetricNote(d) != metricsValidityNoteBase+" "+metricsRatioNote {
		t.Errorf("ratio meta.note = %q", derivedMetricNote(d))
	}
}

// Invalid nuts_version / dataset must be rejected before any DB access (nil DB).
func TestMetricsEndpointsRejectBadParamsWithoutDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := models.DerivedMetricsCatalog{derivedEntry("bevoelkerung", "bevoelkerung", "", "", "nuts_geometry_area_sqkm")}
	h := &Handler{DerivedCatalog: catalog}
	r := gin.New()
	r.GET("/metrics", h.GetMetrics)
	r.GET("/metrics/derived/:slug", h.GetDerivedMetricsCatalogEntry)

	for _, target := range []string{
		"/metrics?dataset=schuldnerquote&nuts_version=2019",
		"/metrics?dataset=schuldnerquote&nuts_version=abc",
		"/metrics?dataset=schuldnerquote&dataset=unknown",
		"/metrics/derived/test?nuts_version=1999",
	} {
		w := doGet(r, target, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", target, w.Code, w.Body.String())
		}
	}
}

// Invalid values stay in the source (marked) so they can be counted; the
// statistics and the ranking only use rows with excluded_reason IS NULL.
func TestMetricsSourcesMarkInvalidValues(t *testing.T) {
	harm := datasetMetricsSource(mustConfig(t, "schuldnerquote"), "schuldnerquote_prozent", 2024)
	if !strings.Contains(harm.SQL, "CASE WHEN has_valid_value AND harmonized_value IS NOT NULL THEN NULL ELSE 'invalid' END AS excluded_reason") {
		t.Errorf("harmonized source must mark invalid values: %s", harm.SQL)
	}
	stg := datasetMetricsSource(mustConfig(t, "beschaeftigungsquote-nuts3"), "beschaeftigungsquote_median", 2024)
	if strings.Contains(stg.SQL, "WHERE has_valid_value") || !strings.Contains(stg.SQL, `CASE WHEN has_valid_value AND "beschaeftigungsquote_median" IS NOT NULL`) {
		t.Errorf("staging source must mark, not drop, invalid values: %s", stg.SQL)
	}

	agg, _ := metricsAggregateQuery(harm, models.MetricsFilter{})
	for _, want := range []string{
		"COUNT(*) FILTER (WHERE excluded_reason IS NULL) AS region_count",
		"PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY value) FILTER (WHERE excluded_reason IS NULL) AS median_val",
		"COUNT(*) FILTER (WHERE excluded_reason = 'invalid') AS excluded_invalid_count",
		"AS regions_without_geometry", "AS excluded_regions",
		"GROUP BY GROUPING SETS ((year), ())",
	} {
		if !strings.Contains(agg, want) {
			t.Errorf("aggregate query lacks %q: %s", want, agg)
		}
	}
	rank, _ := metricsRankQuery(harm, models.MetricsFilter{GeodataIDNUTS: "DE111"})
	if !strings.Contains(rank, "WHERE excluded_reason IS NULL") {
		t.Errorf("ranking must only rank valid values: %s", rank)
	}
}

func TestMetricsResultFromAggRows(t *testing.T) {
	y := func(v int) *int { return &v }
	f := func(v float64) *float64 { return &v }
	rows := []metricsAggRow{
		{Year: y(2020), RegionCount: 2, MinVal: f(1), MaxVal: f(3), MeanVal: f(2), MedianVal: f(2), StddevVal: f(1), ExcludedInvalidCount: 1},
		// Only invalid values: no statistics, but still counted in the totals.
		{Year: y(2021), RegionCount: 0, ExcludedInvalidCount: 3},
		{Year: nil, RegionCount: 2, ExcludedInvalidCount: 4, ExcludedRegions: 3, RegionsWithoutGeometry: 1},
	}
	result, byYear := metricsResultFromAggRows(rows)
	if len(byYear) != 1 || byYear[0].Year != 2020 || byYear[0].ExcludedInvalidCount != 1 || byYear[0].Mean != 2 {
		t.Errorf("byYear = %+v", byYear)
	}
	if result.ExcludedInvalidCount != 4 || result.ExcludedRegions != 3 || result.RegionsWithoutGeometry != 1 {
		t.Errorf("totals = %+v", result)
	}
}

func TestRunIndexedLimitedKeepsOrderAndBoundsParallelism(t *testing.T) {
	const n = 20
	out := make([]int, n)
	var running, peak atomic.Int32
	idx, err := runIndexedLimited(context.Background(), n, metricsMaxParallel, func(ctx context.Context, i int) error {
		cur := running.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		// Later indexes finish first, so completion order differs from input order.
		time.Sleep(time.Duration(n-i) * time.Millisecond)
		out[i] = i * 10
		running.Add(-1)
		return nil
	})
	if err != nil || idx != -1 {
		t.Fatalf("idx=%d err=%v", idx, err)
	}
	for i, v := range out {
		if v != i*10 {
			t.Fatalf("result order broken at %d: %v", i, out)
		}
	}
	if p := peak.Load(); p > metricsMaxParallel || p < 2 {
		t.Errorf("peak parallelism = %d, want 2..%d", p, metricsMaxParallel)
	}
}

func TestRunIndexedLimitedPropagatesFirstError(t *testing.T) {
	boom := errors.New("boom")
	var started atomic.Int32
	idx, err := runIndexedLimited(context.Background(), 50, 2, func(ctx context.Context, i int) error {
		started.Add(1)
		if i == 3 {
			return boom
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
			return nil
		}
	})
	if !errors.Is(err, boom) || idx != 3 {
		t.Fatalf("idx=%d err=%v, want 3/boom", idx, err)
	}
	if s := started.Load(); s >= 50 {
		t.Errorf("calls after the error must be skipped, %d started", s)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if idx, err := runIndexedLimited(ctx, 5, 2, func(context.Context, int) error { return nil }); !errors.Is(err, context.Canceled) || idx != -1 {
		t.Errorf("cancelled parent: idx=%d err=%v", idx, err)
	}
}
