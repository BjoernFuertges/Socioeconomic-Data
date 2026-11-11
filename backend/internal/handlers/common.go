package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

func normalizeNullableLimitQuery(c *gin.Context) bool {
	rawLimit, exists := c.GetQuery("limit")
	if !exists || !strings.EqualFold(strings.TrimSpace(rawLimit), "null") {
		return false
	}

	q := c.Request.URL.Query()
	q.Del("limit")
	c.Request.URL.RawQuery = q.Encode()
	return true
}

func resolvePagination(c *gin.Context, defaultLimit int, requestedLimit int, requestedOffset int, limitNull bool) (*int, int, error) {
	if chunkSizeRaw := strings.TrimSpace(c.Query("chunk_size")); chunkSizeRaw != "" {
		chunkSize, err := strconv.Atoi(chunkSizeRaw)
		if err != nil || chunkSize <= 0 {
			return nil, 0, fmt.Errorf("chunk_size muss eine positive Zahl sein")
		}

		chunk := 0
		if chunkRaw := strings.TrimSpace(c.Query("chunk")); chunkRaw != "" {
			chunk, err = strconv.Atoi(chunkRaw)
			if err != nil || chunk < 0 {
				return nil, 0, fmt.Errorf("chunk muss eine Zahl >= 0 sein")
			}
		}

		// Guard against int overflow, which would yield a negative OFFSET (DB error -> 500).
		if chunk > math.MaxInt/chunkSize {
			return nil, 0, fmt.Errorf("chunk ist zu groß")
		}
		offset := chunk * chunkSize
		return &chunkSize, offset, nil
	}

	offset := requestedOffset
	if offset < 0 {
		return nil, 0, fmt.Errorf("offset muss eine Zahl >= 0 sein")
	}

	if limitNull {
		return nil, offset, nil
	}

	limit := requestedLimit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 0 {
		return nil, 0, fmt.Errorf("limit muss >= 0 sein")
	}

	return &limit, offset, nil
}

func applyPaginationClause(query string, argPos int, limit *int, offset int) (string, []interface{}) {
	if limit == nil {
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argPos)
			return query, []interface{}{offset}
		}
		return query, nil
	}

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)
	return query, []interface{}{*limit, offset}
}

func resolveAndApplyOrderedPagination(
	c *gin.Context,
	query string,
	args []interface{},
	argPos int,
	orderByExpr string,
	defaultLimit int,
	requestedLimit int,
	requestedOffset int,
	limitNull bool,
) (string, []interface{}, int, error) {
	limit, offset, err := resolvePagination(c, defaultLimit, requestedLimit, requestedOffset, limitNull)
	if err != nil {
		return "", nil, 0, err
	}

	query += fmt.Sprintf(" ORDER BY %s", orderByExpr)
	paginationQuery, paginationArgs := applyPaginationClause(query, argPos, limit, offset)
	return paginationQuery, append(args, paginationArgs...), offset, nil
}

// parseOptionalIntQuery reads an optional integer query parameter. Raw strings
// must not reach the DB as query args: Postgres would reject them with a 500
// instead of the 400 the caller deserves.
func parseOptionalIntQuery(c *gin.Context, name string) (*int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("%s muss eine ganze Zahl sein", name)
	}
	return &v, nil
}

func parseLonLatPoint(rawPoint string) (float64, float64, error) {
	parts := strings.Split(strings.TrimSpace(rawPoint), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("point muss im Format lon,lat angegeben werden")
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("point enthält ungültigen Längengrad")
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("point enthält ungültigen Breitengrad")
	}

	if lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("point Längengrad außerhalb von -180 bis 180")
	}
	if lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("point Breitengrad außerhalb von -90 bis 90")
	}

	return lon, lat, nil
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
