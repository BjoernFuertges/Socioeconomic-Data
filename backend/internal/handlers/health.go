package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck gibt den Status der API zurück
// @Summary Health Check
// @Description Gibt den Status der API zurück
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "soziooekonomische-daten-api",
	})
}
