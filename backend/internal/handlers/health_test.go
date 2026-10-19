package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/version"
)

func TestHealthCheckReportsVersion(t *testing.T) {
	oldVersion, oldBuildTime := version.Version, version.BuildTime
	t.Cleanup(func() { version.Version, version.BuildTime = oldVersion, oldBuildTime })
	version.Version, version.BuildTime = "1.2.3", "2026-01-02T03:04:05Z"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/health", (&Handler{}).HealthCheck)

	w := doGet(r, "/api/v1/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "healthy" || body["version"] != "1.2.3" || body["build_time"] != "2026-01-02T03:04:05Z" {
		t.Errorf("unexpected body: %v", body)
	}
}
