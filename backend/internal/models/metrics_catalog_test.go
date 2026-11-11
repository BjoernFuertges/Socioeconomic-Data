package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMetricsCatalogAndFind(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := "derived_metrics:\n  - slug: dichte\n    label: Dichte\n    formula:\n      type: ratio\n      scale: 2\n"
	if err := os.WriteFile(filepath.Join(dir, "configs", "c.yaml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	cat, err := LoadMetricsCatalog("c.yaml")
	if err != nil {
		t.Fatal(err)
	}
	e, ok := cat.FindBySlug("dichte")
	if !ok || e.Formula.Scale != 2 {
		t.Errorf("unexpected entry: %+v ok=%v", e, ok)
	}
	if _, ok := cat.FindBySlug("nope"); ok {
		t.Error("unknown slug found")
	}
	if _, err := LoadMetricsCatalog("missing.yaml"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestShippedCatalogParses(t *testing.T) {
	chdir(t, "../..")
	cat, err := LoadMetricsCatalog("metrics_catalog.yaml")
	if err != nil || len(cat) == 0 {
		t.Fatalf("shipped catalog: len=%d err=%v", len(cat), err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}
