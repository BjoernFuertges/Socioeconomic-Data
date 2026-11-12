package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

const (
	// maxChunkSize caps chunk_size. Without it chunk_size bypassed every
	// endpoint's limit validation (e.g. the 1000 of POST /geo/query).
	maxChunkSize = 10000
	// maxUnlimitedRows caps limit=null. Above it the request is rejected with
	// 400 and a hint to page via chunk_size instead of being silently cut off.
	// This is only the floor: at startup the cap is raised to 1.25x the largest
	// table (see row_cap.go), so the biggest dataset
	// (beschaeftigungsquote-gemeinden, ~251,700 rows) stays downloadable in one
	// piece from the frontend's dataset catalog as the data grows.
	maxUnlimitedRows = 300000
	// dbErrorMessage is the generic client-facing text for database failures;
	// the real error is only logged (see respondDBError).
	dbErrorMessage = "Interner Fehler beim Datenbankzugriff"
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

// paginationRequest is the validated pagination of a request (parsePagination),
// still independent of the result size.
type paginationRequest struct {
	limit     *int // nil: limit=null (unpaginated)
	offset    int
	chunked   bool // chunk_size/chunk set: paged, limit=null does not apply
	limitNull bool
}

// parsePagination validates chunk_size/chunk, limit/offset and limit=null
// without the result size, so handlers can reject bad parameters before any
// DB access. chunk_size wins over limit/offset and limit=null.
func parsePagination(c *gin.Context, defaultLimit int, requestedLimit int, requestedOffset int, limitNull bool) (paginationRequest, error) {
	if chunkSizeRaw := strings.TrimSpace(c.Query("chunk_size")); chunkSizeRaw != "" {
		chunkSize, err := strconv.Atoi(chunkSizeRaw)
		if err != nil || chunkSize <= 0 {
			return paginationRequest{}, fmt.Errorf("chunk_size muss eine positive Zahl sein")
		}
		if chunkSize > maxChunkSize {
			return paginationRequest{}, fmt.Errorf("chunk_size darf höchstens %d sein", maxChunkSize)
		}

		chunk := 0
		if chunkRaw := strings.TrimSpace(c.Query("chunk")); chunkRaw != "" {
			chunk, err = strconv.Atoi(chunkRaw)
			if err != nil || chunk < 0 {
				return paginationRequest{}, fmt.Errorf("chunk muss eine Zahl >= 0 sein")
			}
		}

		// Guard against int overflow, which would yield a negative OFFSET (DB error -> 500).
		if chunk > math.MaxInt/chunkSize {
			return paginationRequest{}, fmt.Errorf("chunk ist zu groß")
		}
		return paginationRequest{limit: &chunkSize, offset: chunk * chunkSize, chunked: true}, nil
	}

	offset := requestedOffset
	if offset < 0 {
		return paginationRequest{}, fmt.Errorf("offset muss eine Zahl >= 0 sein")
	}

	if limitNull {
		return paginationRequest{offset: offset, limitNull: true}, nil
	}

	limit := requestedLimit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 0 {
		return paginationRequest{}, fmt.Errorf("limit muss >= 0 sein")
	}
	return paginationRequest{limit: &limit, offset: offset}, nil
}

// resolve applies the result size: an unpaginated (limit=null) request whose
// result (total minus offset) exceeds the cap is rejected with a hint to page
// via chunk_size. Paged requests (limit or chunk_size) are never checked
// against the cap. This is the only place the limit=null cap is enforced.
func (p paginationRequest) resolve(total int) (*int, int, error) {
	if p.limitNull && !p.chunked {
		if cap := UnlimitedRowsCap(); int64(total-p.offset) > cap {
			return nil, 0, fmt.Errorf(
				"limit=null ist auf %d Zeilen begrenzt, die Anfrage ergäbe %d Zeilen; bitte seitenweise mit chunk_size (höchstens %d) und chunk=0,1,... abrufen (meta.total = %d)",
				cap, total-p.offset, maxChunkSize, total,
			)
		}
	}
	return p.limit, p.offset, nil
}

// resolvePagination turns chunk_size/chunk, limit/offset and limit=null into
// LIMIT/OFFSET (parsePagination + resolve). total is the row count of the
// unpaginated query (counted by every caller beforehand); it is only used to
// reject limit=null requests whose result would exceed the cap.
func resolvePagination(c *gin.Context, defaultLimit int, requestedLimit int, requestedOffset int, limitNull bool, total int) (*int, int, error) {
	p, err := parsePagination(c, defaultLimit, requestedLimit, requestedOffset, limitNull)
	if err != nil {
		return nil, 0, err
	}
	return p.resolve(total)
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
	total int,
) (string, []interface{}, int, error) {
	limit, offset, err := resolvePagination(c, defaultLimit, requestedLimit, requestedOffset, limitNull, total)
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

// respondDBError answers a failed database access generically: the real error
// (SQL, schema and connection details) is only recorded server-side via
// c.Error and the log, never sent to the client. op names the failed step for
// the log. Timeouts and cancellations (request context, statement_timeout)
// yield 503 instead of 500, since they are transient overload rather than a bug.
func respondDBError(c *gin.Context, op string, err error) {
	_ = c.Error(fmt.Errorf("%s: %w", op, err))
	route := c.FullPath()
	if route == "" && c.Request != nil {
		route = c.Request.URL.Path
	}
	log.Printf("DB-Fehler %s (%s): %v", op, route, err)

	status := http.StatusInternalServerError
	if isDBTimeoutOrCanceled(c, err) {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, models.APIResponse{
		Success: false,
		Error:   dbErrorMessage,
	})
}

// isDBTimeoutOrCanceled reports whether err stems from a cancelled or timed
// out request context, or from Postgres' statement_timeout (SQLSTATE 57014,
// query_canceled).
func isDBTimeoutOrCanceled(c *gin.Context, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return true
	}
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "57014"
}
