package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/version"
)

// HealthCheck returns the status of the API
// @Summary Health Check
// @Description Gibt den Status der API zurück
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":     "healthy",
		"service":    "soziooekonomische-daten-api",
		"version":    version.Version,
		"build_time": version.BuildTime,
	})
}
