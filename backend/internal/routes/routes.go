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
			lau.GET("/:gisco_id", handler.GetLAURegionByID)
		}

		// Arbeitslosenzahlen
		unemployment := v1.Group("/unemployment")
		{
			unemployment.GET("", handler.GetUnemploymentData)
			unemployment.GET("/aggregate", handler.GetUnemploymentAggregated)
			unemployment.GET("/geodata/:geodata_id", handler.GetUnemploymentByGeodataID)
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
