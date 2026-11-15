package models

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Guard: AllSoziodataConfigs() is maintained by hand, harmonizedRouteSlugs
// (cmd/gen-models) too. A new dataset that is only added in one place would
// silently be missing from GET /nuts/{id}/datasets, /metrics or the fill_gaps
// support. Compare the sets against the generated configs and the dbt marts.

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(rel...))
	if err != nil {
		t.Skipf("Quelle nicht verfügbar: %v", err)
	}
	return string(b)
}

func diffSets(t *testing.T, what string, got, want map[string]bool) {
	t.Helper()
	for k := range want {
		if !got[k] {
			t.Errorf("%s: %q fehlt (erwartet laut Gegenstück)", what, k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("%s: %q ist zu viel (nicht im Gegenstück)", what, k)
		}
	}
}

func TestAllSoziodataConfigsMatchGenerated(t *testing.T) {
	src := readRepoFile(t, "models_soziodata_generated.go")

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^var (\w+Config) = KreisDatasetConfig\{`).FindAllStringSubmatch(src, -1) {
		declared[m[1]] = true
	}
	slugs := map[string]bool{}
	for _, m := range regexp.MustCompile(`RouteSlug:\s+"([^"]+)"`).FindAllStringSubmatch(src, -1) {
		slugs[m[1]] = true
	}
	if len(declared) == 0 || len(slugs) != len(declared) {
		t.Fatalf("Generator-Ausgabe nicht lesbar: %d Configs, %d RouteSlugs", len(declared), len(slugs))
	}

	all := AllSoziodataConfigs()
	got := map[string]bool{}
	for _, cfg := range all {
		if got[cfg.RouteSlug] {
			t.Errorf("AllSoziodataConfigs: RouteSlug %q doppelt", cfg.RouteSlug)
		}
		got[cfg.RouteSlug] = true
	}
	if len(all) != len(declared) {
		t.Errorf("AllSoziodataConfigs hat %d Einträge, models_soziodata_generated.go deklariert %d Configs", len(all), len(declared))
	}
	diffSets(t, "AllSoziodataConfigs RouteSlugs", got, slugs)
}

func TestHarmonizedFlagsMatchDbt(t *testing.T) {
	marts := []string{"..", "..", "..", "pipeline", "dbt", "socioeconomic_data_pipeline", "models", "marts"}
	nuts := readRepoFile(t, append(marts, "mart_nuts3_sociodata_harmonized.sql")...)
	lau := readRepoFile(t, append(marts, "mart_lau_sociodata_harmonized.sql")...)

	list := func(sql, name string) map[string]bool {
		block := regexp.MustCompile(`(?s)\{%\s*set\s+` + name + `\s*=\s*\[(.*?)\]\s*%\}`).FindStringSubmatch(sql)
		if block == nil {
			t.Fatalf("Liste %s nicht in der dbt-Datei gefunden", name)
		}
		out := map[string]bool{}
		for _, m := range regexp.MustCompile(`'slug':\s*'([^']+)'`).FindAllStringSubmatch(block[1], -1) {
			out[m[1]] = true
		}
		return out
	}
	wantNuts := list(nuts, "source_datasets")
	// not a loaded_data source, but harmonized separately in the same mart
	wantNuts["beschaeftigungsquote-nuts3"] = true
	wantLau := map[string]bool{}
	for _, m := range regexp.MustCompile(`'slug':\s*'([^']+)'`).FindAllStringSubmatch(
		regexp.MustCompile(`(?s)\{%\s*set\s+lau_source_datasets\s*=\s*\[(.*?)\]\s*%\}`).FindStringSubmatch(lau)[1], -1) {
		wantLau[m[1]] = true
	}

	gotNuts, gotLau := map[string]bool{}, map[string]bool{}
	for _, cfg := range AllSoziodataConfigs() {
		if cfg.IsHarmonized && cfg.IsHarmonizedLau {
			t.Errorf("%s: IsHarmonized und IsHarmonizedLau gleichzeitig", cfg.RouteSlug)
		}
		if cfg.IsHarmonized {
			gotNuts[cfg.RouteSlug] = true
		}
		if cfg.IsHarmonizedLau {
			gotLau[cfg.RouteSlug] = true
		}
	}
	diffSets(t, "IsHarmonized (harmonizedRouteSlugs) vs. mart_nuts3_sociodata_harmonized", gotNuts, wantNuts)
	diffSets(t, "IsHarmonizedLau (harmonizedLauRouteSlugs) vs. mart_lau_sociodata_harmonized", gotLau, wantLau)
}
