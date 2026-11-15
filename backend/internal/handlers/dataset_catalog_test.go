package handlers

import (
	"strings"
	"testing"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func TestDatasetCatalogQuery(t *testing.T) {
	query, args := datasetCatalogQuery(nil)
	if strings.Contains(query, "WHERE") {
		t.Errorf("query without slugs should have no WHERE clause, got %q", query)
	}
	if len(args) != 0 {
		t.Errorf("query without slugs should have no args, got %v", args)
	}

	query, args = datasetCatalogQuery([]string{"bevoelkerung", "arbeitslose"})
	if !strings.Contains(query, "WHERE dataset_slug = ANY($1)") {
		t.Errorf("query with slugs should filter by dataset_slug = ANY($1), got %q", query)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (the slug array), got %v", args)
	}
}

// TestDatasetCatalogEntryNegotiation checks that GetDatasetCatalogEntry yields
// exactly one row for CSV (respondNegotiated gets a single
// DatasetCatalogEintrag as jsonBody but a 1-element slice as csvRows - see
// comment in dataset_catalog.go).
func TestDatasetCatalogEntryNegotiation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	name := "Bevölkerung"
	entry := models.DatasetCatalogEintrag{
		DatasetSlug: "bevoelkerung",
		DatasetURI:  "urn:soziodata:dataset:bevoelkerung",
		Name:        &name,
		License:     "unbekannt",
	}
	r.GET("/api/v1/datasets/:slug", func(c *gin.Context) {
		respondNegotiated(c, entry, []models.DatasetCatalogEintrag{entry}, nil)
	})

	w := doGet(r, "/api/v1/datasets/bevoelkerung", "text/csv")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	lines := strings.Split(strings.TrimRight(w.Body.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + exactly 1 data row, got %d lines: %q", len(lines), w.Body.String())
	}
	if !strings.Contains(lines[1], "bevoelkerung") || !strings.Contains(lines[1], "unbekannt") {
		t.Errorf("data row = %q, missing expected fields", lines[1])
	}

	w = doGet(r, "/api/v1/datasets/bevoelkerung", "application/json")
	if !strings.Contains(w.Body.String(), `"dataset_slug":"bevoelkerung"`) {
		t.Errorf("json body = %q, expected single object with dataset_slug", w.Body.String())
	}
}
