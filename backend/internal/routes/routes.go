package routes

import (
	"fmt"

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
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(loggingMiddleware())

	v1 := router.Group("/api/v1")
	{
		// Health check
		v1.GET("/health", handler.HealthCheck)

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
