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
			lau.GET("/:gisco_id", handler.GetLAURegionByID)
		}

		// NUTS-Regionen (alle Indikatoren je Verwaltungsbezirk)
		// NUTS-Regionen (alle Indikatoren je Verwaltungsbezirk)
		nutsGroup := v1.Group("/nuts")
		{
			nutsGroup.GET("", handler.GetAllNutsRegions)
			nutsGroup.GET("/:nuts_id/versions", handler.GetNutsRegionVersions)
			nutsGroup.GET("/:nuts_id/datasets", handler.GetNutsDatasets)
			nutsGroup.GET("/:nuts_id", handler.GetNutsRegionData)
		}

		geoGroup := v1.Group("/geo")
		{
			geoGroup.GET("", handler.GetGeoRegions)
			geoGroup.GET("/:geo_id/versions", handler.GetGeoRegionVersions)
			geoGroup.GET("/:geo_id", handler.GetGeoRegionByID)
			geoGroup.POST("/query", handler.QueryGeoRegionsByGeometry)
		}

		// Arbeitslosenzahlen
		unemployment := v1.Group("/unemployment")
		{
			unemployment.GET("", handler.GetUnemploymentData)
			unemployment.GET("/aggregated", handler.GetUnemploymentAggregated)
			unemployment.GET("/geodata/:geodata_id", handler.GetUnemploymentByGeodataID)
		}

		socioeconomic := v1.Group("/socioeconomic")
		{
			socioeconomic.GET("/inventory", handler.GetSoziooekonomischesInventar)
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
