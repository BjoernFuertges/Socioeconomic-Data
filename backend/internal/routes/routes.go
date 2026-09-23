package routes

import (
	"fmt"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/handlers"
)

// SetupRoutes konfiguriert alle API-Routen
func SetupRoutes(handler *handlers.Handler) *gin.Engine {
	// Gin im Release-Modus für Produktion
	gin.SetMode(gin.ReleaseMode)

	// Gin-Router erstellen
	router := gin.Default()

	// Middleware hinzufügen
	router.Use(corsMiddleware())
	router.Use(loggingMiddleware())

	// API-Versionsgruppe
	v1 := router.Group("/api/v1")
	{
		// Health Check
		v1.GET("/health", handler.HealthCheck)

		// LAU-Regionen (Local Administrative Units)
		lau := v1.Group("/lau")
		{
			lau.GET("", handler.GetLAURegions)
			lau.GET("/:gisco_id/datasets", handler.GetLAUDatasets)
			lau.GET("/:gisco_id/versions", handler.GetLAURegionVersions)
			lau.GET("/:gisco_id", handler.GetLAURegionByID)
		}

		// NUTS-Regionen (alle Indikatoren je Verwaltungsbezirk)
		// NUTS-Regionen (alle Indikatoren je Verwaltungsbezirk)
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

		// Agenturbezirke (eigenständige Gebietsgliederung der Bundesagentur
		// für Arbeit, nicht deckungsgleich mit NUTS/LAU/ARS) mit monatlichen
		// Arbeitslosenzahlen (AfA Lange Zeitreihen)
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

		// Kombinierter Export einer Region (roh und/oder aggregiert, JSON/CSV)
		v1.GET("/regions/:region_type/:region_id/data", handler.GetRegionData)

		socioeconomic := v1.Group("/socioeconomic")
		{
			socioeconomic.GET("/inventory", handler.GetSoziooekonomischesInventar)
		}

		// Datensatz-Katalog: Herkunft, Lizenz, Beschreibung
		datasetsGroup := v1.Group("/datasets")
		{
			datasetsGroup.GET("", handler.GetDatasetCatalog)
			datasetsGroup.GET("/:slug", handler.GetDatasetCatalogEntry)
		}

		// Metriken-Endpunkte: statistische Aggregationen über Soziodata-Datensätze
		metricsGroup := v1.Group("/metrics")
		{
			metricsGroup.GET("", handler.GetMetrics)
			metricsGroup.GET("/derived", handler.GetDerivedMetricsCatalog)
			metricsGroup.GET("/derived/:slug", handler.GetDerivedMetricsCatalogEntry)
		}

		// Generierte Soziodata-Routen (aus dbt-Schema-YAML-Dateien)
		RegisterSoziodataRoutes(v1, handler)
	}

	// Swagger-Dokumentation
	router.GET("/api/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}

// corsMiddleware fügt CORS-Header hinzu
func corsMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		// Damit Cross-Origin-fetch den Dateinamen von ?download=1-Antworten lesen kann
		c.Header("Access-Control-Expose-Headers", "Content-Disposition")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}

// loggingMiddleware fügt Logging hinzu
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
