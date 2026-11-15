package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware())
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing wildcard origin")
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("credentials must not be allowed with a wildcard origin")
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/x", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
}

func TestTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	probe := func(remote, xff string) string {
		r := gin.New()
		if err := r.SetTrustedProxies(trustedProxies()); err != nil {
			t.Fatal(err)
		}
		r.GET("/ip", func(c *gin.Context) { c.String(200, c.ClientIP()) })
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = remote + ":1234"
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Body.String()
	}

	t.Setenv("TRUSTED_PROXIES", "")
	if got := probe("172.18.0.2", "203.0.113.7"); got != "203.0.113.7" {
		t.Errorf("Traefik (private) must be trusted, ClientIP = %q", got)
	}
	if got := probe("198.51.100.9", "203.0.113.7"); got != "198.51.100.9" {
		t.Errorf("public peer must not be trusted, ClientIP = %q", got)
	}

	t.Setenv("TRUSTED_PROXIES", "198.51.100.0/24, 10.1.2.3")
	if got := probe("198.51.100.9", "203.0.113.7"); got != "203.0.113.7" {
		t.Errorf("TRUSTED_PROXIES override ignored, ClientIP = %q", got)
	}
}

func TestCacheControlMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.Use(cacheControlMiddleware())
	v1.GET("/nuts", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	v1.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	v1.GET("/versions", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	v1.GET("/datasets/:slug", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	v1.GET("/missing", func(c *gin.Context) { c.JSON(404, gin.H{"ok": false}) })
	v1.GET("/boom", func(c *gin.Context) { c.Status(503) })
	v1.GET("/own", func(c *gin.Context) { c.Header("Cache-Control", "private"); c.String(200, "x") })
	v1.POST("/geo/query", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	tests := []struct {
		method, path string
		want         string
	}{
		{"GET", "/api/v1/nuts", "public, max-age=300, stale-while-revalidate=600"},
		{"GET", "/api/v1/health", "no-store"},
		{"GET", "/api/v1/versions", cacheControlSlow},
		{"GET", "/api/v1/datasets/foo", cacheControlSlow},
		{"GET", "/api/v1/missing", "no-store"},
		{"GET", "/api/v1/boom", "no-store"},
		{"GET", "/api/v1/own", "private"},
		{"POST", "/api/v1/geo/query", ""},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if got := w.Header().Get("Cache-Control"); got != tt.want {
			t.Errorf("%s %s: Cache-Control = %q, want %q", tt.method, tt.path, got, tt.want)
		}
	}
}
