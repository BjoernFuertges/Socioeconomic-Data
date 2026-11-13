package handlers

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// setTargetVersionsForTest replaces the target versions for one test.
func setTargetVersionsForTest(t *testing.T, nuts, lau []int) {
	t.Helper()
	nutsTargetVersions.Store(newVersionSet(nuts))
	lauTargetVersions.Store(newVersionSet(lau))
	t.Cleanup(resetTargetVersions)
}

func TestTargetVersionsFallback(t *testing.T) {
	resetTargetVersions()
	if !slices.Equal(supportedNutsVersions(), fallbackNutsVersions) || defaultNutsVersion() != 2024 {
		t.Errorf("nuts = %v default %d", supportedNutsVersions(), defaultNutsVersion())
	}
	if !slices.Equal(supportedLauVersions(), fallbackLauVersions) || defaultLauVersion() != 2024 {
		t.Errorf("lau = %v default %d", supportedLauVersions(), defaultLauVersion())
	}
}

func TestTargetVersionsDefaultIsNewest(t *testing.T) {
	// Unsorted input with a duplicate: sorted, deduplicated, default = max.
	setTargetVersionsForTest(t, []int{2027, 2016, 2024, 2027}, []int{2022, 2020})
	if got := supportedNutsVersions(); !slices.Equal(got, []int{2016, 2024, 2027}) {
		t.Errorf("nuts = %v", got)
	}
	if defaultNutsVersion() != 2027 || defaultLauVersion() != 2022 {
		t.Errorf("defaults = %d / %d", defaultNutsVersion(), defaultLauVersion())
	}
	if !isSupportedNutsVersion(2027) || isSupportedNutsVersion(2021) || !isSupportedLauVersion(2020) || isSupportedLauVersion(2024) {
		t.Error("isSupported* does not follow the configured lists")
	}

	v, err := resolveMetricsNutsVersion(models.MetricsFilter{})
	if err != nil || v != 2027 {
		t.Errorf("resolveMetricsNutsVersion() = %d, %v", v, err)
	}
	bad := 2021
	if _, err := resolveMetricsNutsVersion(models.MetricsFilter{NutsVersion: &bad}); err == nil ||
		!strings.Contains(err.Error(), "[2016 2024 2027]") {
		t.Errorf("error must list the configured versions: %v", err)
	}
	if !strings.Contains(customRegionUnitsSQL("nuts3"), "2027") || !strings.Contains(customRegionUnitsSQL("lau"), "2022") {
		t.Error("custom region units must use the current defaults")
	}
}

func TestGetTargetVersions(t *testing.T) {
	setTargetVersionsForTest(t, []int{2021, 2024}, []int{2020, 2022, 2024})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/versions", (&Handler{}).GetTargetVersions)
	w := doGet(r, "/versions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Success bool                   `json:"success"`
		Data    TargetVersionsResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Success {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	body := resp.Data
	if !slices.Equal(body.Nuts.Versions, []int{2021, 2024}) || body.Nuts.Default != 2024 ||
		!slices.Equal(body.Lau.Versions, []int{2020, 2022, 2024}) || body.Lau.Default != 2024 {
		t.Errorf("body = %+v", body)
	}
}
