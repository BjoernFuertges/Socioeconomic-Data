package routes

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/handlers"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRoutes registers all API routes.
func SetupRoutes(handler *handlers.Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	// gin.New instead of gin.Default: Default adds its own logger, which would duplicate loggingMiddleware.
	router := gin.New()
	// Behind Traefik only (shared Docker network): trust X-Forwarded-For from private ranges,
	// not from arbitrary clients (gin's default trusts every proxy).
	if err := router.SetTrustedProxies(trustedProxies()); err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(loggingMiddleware())

	v1 := router.Group("/api/v1")
	v1.Use(cacheControlMiddleware())
	{
		// Health check
		v1.GET("/health", handler.HealthCheck)

		// Supported nuts_version / lau_version values and their defaults
		v1.GET("/versions", handler.GetTargetVersions)

		// LAU regions (Local Administrative Units)
		lau := v1.Group("/lau")
		{
			lau.GET("", handler.GetLAURegions)
			lau.GET("/:gisco_id/datasets", handler.GetLAUDatasets)
			lau.GET("/:gisco_id/versions", handler.GetLAURegionVersions)
			lau.GET("/:gisco_id", handler.GetLAURegionByID)
		}

		// NUTS regions (all indicators per administrative district)
		nutsGroup := v1.Group("/nuts")
		{
			nutsGroup.GET("", handler.GetAllNutsRegions)
			nutsGroup.GET("/:nuts_id/versions", handler.GetNutsRegionVersions)
			nutsGroup.GET("/:nuts_id/datasets", handler.GetNutsDatasets)
			nutsGroup.GET("/:nuts_id/lau-aggregate", handler.GetNutsLauAggregate)
			nutsGroup.GET("/:nuts_id/agenturbezirke", handler.GetNutsAgenturbezirke)
			nutsGroup.GET("/:nuts_id/dataset-aggregate/:slug", handler.GetNutsDatasetAggregateTimeseries)
			nutsGroup.GET("/:nuts_id", handler.GetNutsRegionData)
		}

		geoGroup := v1.Group("/geo")
		{
			geoGroup.GET("", handler.GetGeoRegions)
			geoGroup.GET("/:geo_id/versions", handler.GetGeoRegionVersions)
			geoGroup.GET("/:geo_id", handler.GetGeoRegionByID)
			geoGroup.POST("/query", handler.QueryGeoRegionsByGeometry)
		}

		// Custom regions (GeoJSON per request, nothing is stored)
		customRegionsGroup := v1.Group("/custom-regions")
		{
			customRegionsGroup.GET("/datasets", handler.ListCustomRegionDatasets)
			customRegionsGroup.POST("/dataset-aggregate", handler.QueryCustomRegionDatasetAggregate)
		}

		// Agenturbezirke (distinct territorial division of the Bundesagentur für
		// Arbeit, not congruent with NUTS/LAU/ARS) with monthly Arbeitslose figures
		// (AfA long time series)
		agenturbezirkeGroup := v1.Group("/agenturbezirke")
		{
			agenturbezirkeGroup.GET("", handler.GetAgenturbezirke)
			agenturbezirkeGroup.GET("/latest", handler.GetAgenturbezirkeLatest)
			agenturbezirkeGroup.GET("/periods", handler.GetAgenturbezirkePeriods)
			agenturbezirkeGroup.GET("/period", handler.GetAgenturbezirkeByPeriod)
			agenturbezirkeGroup.GET("/dataset-aggregate", handler.GetAgenturbezirkDatasetAggregateYears)
			agenturbezirkeGroup.GET("/dataset-aggregate/:slug", handler.GetAgenturbezirkeDatasetAggregateByYear)
			agenturbezirkeGroup.GET("/:ba_schl", handler.GetAgenturbezirkByID)
			agenturbezirkeGroup.GET("/:ba_schl/arbeitslosigkeit", handler.GetAgenturbezirkArbeitslosigkeit)
			agenturbezirkeGroup.GET("/:ba_schl/nuts3", handler.GetAgenturbezirkNuts3)
			agenturbezirkeGroup.GET("/:ba_schl/nuts-hierarchy", handler.GetAgenturbezirkNutsHierarchy)
			agenturbezirkeGroup.GET("/:ba_schl/dataset-aggregate/:slug", handler.GetAgenturbezirkDatasetAggregateTimeseries)
		}

		// Combined export of a region (raw and/or aggregated, JSON/CSV)
		v1.GET("/regions/:region_type/:region_id/data", handler.GetRegionData)

		socioeconomic := v1.Group("/socioeconomic")
		{
			socioeconomic.GET("/inventory", handler.GetSoziooekonomischesInventar)
		}

		// Dataset catalog: origin, license, description
		datasetsGroup := v1.Group("/datasets")
		{
			datasetsGroup.GET("", handler.GetDatasetCatalog)
			datasetsGroup.GET("/:slug", handler.GetDatasetCatalogEntry)
			datasetsGroup.GET("/:slug/source", handler.GetDatasetSource)
		}

		// Metrics endpoints: statistical aggregations over Soziodata datasets
		metricsGroup := v1.Group("/metrics")
		{
			metricsGroup.GET("", handler.GetMetrics)
			metricsGroup.GET("/derived", handler.GetDerivedMetricsCatalog)
			metricsGroup.GET("/derived/:slug", handler.GetDerivedMetricsCatalogEntry)
		}

		// Generated Soziodata routes (from dbt schema YAML files)
		RegisterSoziodataRoutes(v1, handler)
	}

	// Swagger documentation
	router.GET("/api/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}

// corsMiddleware allows any origin (public, read-only API). No credentials: the
// wildcard origin is invalid together with Allow-Credentials.
func corsMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Lets cross-origin fetch read the filename of ?download=1 responses.
		c.Header("Access-Control-Expose-Headers", "Content-Disposition")
		c.Header("Access-Control-Max-Age", "600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}

const (
	// cacheControlDefault: read-only data that only changes with a pipeline run.
	cacheControlDefault = "public, max-age=300, stale-while-revalidate=600"
	// cacheControlSlow: metadata that changes rarely (supported versions are read
	// at startup, the dataset catalog is hand-maintained).
	cacheControlSlow = "public, max-age=3600, stale-while-revalidate=86400"
	cacheControlNone = "no-store"
)

// cacheControlFor picks the Cache-Control value for a successful GET on the
// given route pattern (c.FullPath()).
func cacheControlFor(fullPath string) string {
	switch {
	case fullPath == "/api/v1/health":
		return cacheControlNone
	case fullPath == "/api/v1/versions", strings.HasPrefix(fullPath, "/api/v1/datasets"):
		return cacheControlSlow
	}
	return cacheControlDefault
}

// cacheWriter sets Cache-Control right before the status line/body is sent,
// when the final status is known: successful GET responses get the route's
// public value, error responses (>= 400) "no-store". A value the handler set
// itself is kept.
type cacheWriter struct {
	gin.ResponseWriter
	value string
	done  bool
}

func (w *cacheWriter) apply() {
	if w.done {
		return
	}
	w.done = true
	if w.Header().Get("Cache-Control") != "" {
		return
	}
	if w.Status() >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", cacheControlNone)
		return
	}
	w.Header().Set("Cache-Control", w.value)
}

func (w *cacheWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code) // only records the code in gin's writer
	w.apply()
}

func (w *cacheWriter) WriteHeaderNow() {
	w.apply()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *cacheWriter) Write(b []byte) (int, error) {
	w.apply()
	return w.ResponseWriter.Write(b)
}

func (w *cacheWriter) WriteString(s string) (int, error) {
	w.apply()
	return w.ResponseWriter.WriteString(s)
}

// cacheControlMiddleware adds Cache-Control to GET responses of the API so
// that browsers, Traefik and CDNs can absorb repeated reads (the data only
// changes with a pipeline run). Error responses and /health are never cached;
// other methods (POST for GeoJSON queries) are left untouched. Responses are
// negotiated per Accept header (JSON/CSV); negotiate.go sets "Vary: Accept".
func cacheControlMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Writer = &cacheWriter{ResponseWriter: c.Writer, value: cacheControlFor(c.FullPath())}
		}
		c.Next()
	}
}

// loggingMiddleware logs one access line per request.
func loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format("02/Jan/2006:15:04:05 -0700"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	})
}

// defaultTrustedProxies covers loopback and the private ranges Docker bridge networks are
// allocated from (Traefik reaches the backend over the compose network "backend-net").
var defaultTrustedProxies = []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// trustedProxies returns the comma-separated TRUSTED_PROXIES env (CIDRs/IPs) or the private defaults.
func trustedProxies() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return defaultTrustedProxies
	}
	return out
}
