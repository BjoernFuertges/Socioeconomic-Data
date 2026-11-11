package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

type socioeconomicInventoryFilter struct {
	Dataset      string `form:"dataset"`
	Year         *int   `form:"year"`
	GeodataLevel string `form:"geodata_level"`
}

// GetSoziooekonomischesInventar returns which Soziodata are available per year and geodata level.
// @Summary Inventar verfügbarer Soziodaten
// @Description Liefert pro Datensatz, Jahr und Geodatenebene die vorhandenen Datensätze inkl. Geodaten-Verweis-Abdeckung (NUTS/LAU/ARS).
// @Tags Soziooekonomie
// @Accept json
// @Produce json
// @Param dataset query string false "Datensatz-Slug (z.B. arbeitslose)" example(arbeitslose)
// @Param year query int false "Jahr" example(2022)
// @Param geodata_level query string false "Geodatenebene (z.B. NUTS_3, LAU)" example(NUTS_3)
// @Success 200 {object} models.APIResponse{data=[]models.SoziooekonomischesInventarEintrag}
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Router /socioeconomic/inventory [get]
func (h *Handler) GetSoziooekonomischesInventar(c *gin.Context) {
	var filter socioeconomicInventoryFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	query := `SELECT dataset, year, geodata_level, records, distinct_geodata_nuts, distinct_geodata_lau, distinct_geodata_ars
		FROM transformed_data.mart_socioeconomic_inventory`
	args := make([]interface{}, 0, 3)

	whereParts := make([]string, 0, 2)
	argPos := 1
	if dataset := strings.TrimSpace(filter.Dataset); dataset != "" {
		whereParts = append(whereParts, fmt.Sprintf("dataset = $%d", argPos))
		args = append(args, strings.ToLower(dataset))
		argPos++
	}
	if filter.Year != nil {
		whereParts = append(whereParts, fmt.Sprintf("year = $%d", argPos))
		args = append(args, *filter.Year)
		argPos++
	}
	if geodataLevel := strings.TrimSpace(filter.GeodataLevel); geodataLevel != "" {
		whereParts = append(whereParts, fmt.Sprintf("UPPER(geodata_level) = UPPER($%d)", argPos))
		args = append(args, geodataLevel)
	}
	if len(whereParts) > 0 {
		query += ` WHERE ` + strings.Join(whereParts, ` AND `)
	}
	query += ` ORDER BY dataset, year DESC, geodata_level`

	inventory := make([]models.SoziooekonomischesInventarEintrag, 0)
	if err := h.DB.Select(&inventory, query, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler beim Soziodaten-Inventar: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    inventory,
		Meta: &models.MetaInfo{
			Total: len(inventory),
		},
	})
}
