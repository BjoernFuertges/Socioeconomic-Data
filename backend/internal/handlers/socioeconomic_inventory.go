package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

var sqlIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func qualifiedTableName(schema string, table string) (string, error) {
	if !sqlIdentifierPattern.MatchString(schema) {
		return "", fmt.Errorf("ungültiger Schema-Name in Dataset-Konfiguration: %q", schema)
	}
	if !sqlIdentifierPattern.MatchString(table) {
		return "", fmt.Errorf("ungültiger Tabellenname in Dataset-Konfiguration: %q", table)
	}

	return fmt.Sprintf(`"%s"."%s"`, schema, table), nil
}

type socioeconomicInventoryFilter struct {
	Dataset      string `form:"dataset"`
	Year         *int   `form:"year"`
	GeodataLevel string `form:"geodata_level"`
}

// GetSoziooekonomischesInventar gibt zurück, welche Soziodaten je Jahr und Geodatenebene verfügbar sind.
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

	configs := models.AllSoziodataConfigs()
	if dataset := strings.TrimSpace(filter.Dataset); dataset != "" {
		filteredConfigs := make([]models.KreisDatasetConfig, 0, 1)
		for _, cfg := range configs {
			if strings.EqualFold(cfg.RouteSlug, dataset) {
				filteredConfigs = append(filteredConfigs, cfg)
				break
			}
		}
		configs = filteredConfigs
	}

	if len(configs) == 0 {
		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: []models.SoziooekonomischesInventarEintrag{}})
		return
	}

	selectParts := make([]string, 0, len(configs))
	args := make([]interface{}, 0, len(configs))

	for idx, cfg := range configs {
		fullTableName, err := qualifiedTableName(cfg.Schema, cfg.Table)
		if err != nil {
			respondInternalError(c, err.Error())
			return
		}

		slugArgPos := idx + 1
		selectParts = append(selectParts, fmt.Sprintf(
			`SELECT $%d::text AS dataset,
					year,
					geodata_level,
					COUNT(*)::int AS records,
					COUNT(DISTINCT NULLIF(CAST(geodata_id_nuts AS text), ''))::int AS distinct_geodata_nuts,
					COUNT(DISTINCT NULLIF(CAST(geodata_id_lau AS text), ''))::int AS distinct_geodata_lau,
					COUNT(DISTINCT NULLIF(CAST(geodata_id_ars AS text), ''))::int AS distinct_geodata_ars
				 FROM %s
				 GROUP BY year, geodata_level`,
			slugArgPos,
			fullTableName,
		))
		args = append(args, cfg.RouteSlug)
	}

	query := `SELECT * FROM (` + strings.Join(selectParts, ` UNION ALL `) + `) AS inventory`

	whereParts := make([]string, 0, 2)
	argPos := len(args) + 1
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
