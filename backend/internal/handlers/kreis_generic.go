package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/internal/models"
)

// sozioKreisBaseColumns ist die SELECT-Spaltenliste gemeinsam für alle Kreis-Datensätze.
// observation_date wird explizit als text gecastet, damit sqlx es in ein string-Feld scannen kann.
const sozioKreisBaseColumns = `d.original_id_1, d.original_name, d.geodata_name, d.geodata_id_nuts, ` +
	`coalesce(` +
	`nullif(to_jsonb(d)->>'geodata_id_lau', ''), ` +
	`case when d.geodata_level = 'LAU' then nullif(cast(d.geodata_id_ars as text), '') end, ` +
	`case when d.geodata_level = 'LAU' then cast(d.original_id_1 as text) end, ''` +
	`) as geodata_id_lau, ` +
	`cast(d.geodata_id_ars as text) as geodata_id_ars, d.mapper, d.parameter, d.geodata_level, d.geodata_year, ` +
	`d.year, d.observation_date::text, d.has_valid_value`

// getKreisData ist der generische Handler-Kern für alle Kreis-Datensätze.
// T muss SozioBaseData einbetten und alle cfg.ValueColumns via db-Tags mappen.
func getKreisData[T any](h *Handler, c *gin.Context, cfg models.KreisDatasetConfig) {
	limitNull := normalizeNullableLimitQuery(c)

	var filter models.SozioKreisFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondBadRequest(c, fmt.Sprintf("Ungültige Query-Parameter: %v", err))
		return
	}

	if filter.Limit == 0 {
		filter.Limit = cfg.DefaultLimit
	}

	query := fmt.Sprintf(
		`SELECT %s, %s FROM %s.%s d WHERE 1=1`,
		sozioKreisBaseColumns,
		strings.Join(cfg.ValueColumns, ", "),
		cfg.Schema,
		cfg.Table,
	)

	args := []interface{}{}
	argPos := 1

	if filter.GeodataIDNUTS != "" {
		query += fmt.Sprintf(" AND d.geodata_id_nuts = $%d", argPos)
		args = append(args, filter.GeodataIDNUTS)
		argPos++
	}

	if filter.Year != nil {
		query += fmt.Sprintf(" AND d.year = $%d", argPos)
		args = append(args, *filter.Year)
		argPos++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") as cnt"
	var total int
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(c, fmt.Sprintf("Fehler beim Zählen der Datensätze: %v", err))
		return
	}

	pagedQuery, pagedArgs, offset, err := resolveAndApplyOrderedPagination(
		c,
		query,
		args,
		argPos,
		cfg.OrderBy,
		cfg.DefaultLimit,
		filter.Limit,
		filter.Offset,
		limitNull,
	)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	query = pagedQuery
	args = pagedArgs

	data := make([]T, 0)
	if err := h.DB.Select(&data, query, args...); err != nil {
		respondInternalError(c, fmt.Sprintf("Datenbankfehler: %v", err))
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    data,
		Meta: &models.MetaInfo{
			Total:  total,
			Offset: offset,
		},
	})
}
