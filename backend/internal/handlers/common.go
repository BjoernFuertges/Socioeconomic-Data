package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

func parseLimitOffset(c *gin.Context, defaultLimit int) (int, int) {
	limit := defaultLimit
	offset := 0
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}
	return limit, offset
}

func respondBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, models.APIResponse{
		Success: false,
		Error:   message,
	})
}

func respondNotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, models.APIResponse{
		Success: false,
		Error:   message,
	})
}

func respondInternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, models.APIResponse{
		Success: false,
		Error:   message,
	})
}
