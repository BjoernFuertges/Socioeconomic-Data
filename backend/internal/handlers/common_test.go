package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

func TestResolvePaginationRejectsChunkOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?chunk_size=1000&chunk="+strconv.Itoa(math.MaxInt/10), nil)

	if _, _, err := resolvePagination(c, 100, 0, 0, false, 0); err == nil {
		t.Fatal("expected an error for chunk*chunk_size overflow")
	}
}

func TestResolvePaginationChunkOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?chunk_size=50&chunk=3", nil)

	limit, offset, err := resolvePagination(c, 100, 0, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if limit == nil || *limit != 50 || offset != 150 {
		t.Fatalf("limit=%v offset=%d, want 50/150", limit, offset)
	}
}

// Invalid numeric query params must be rejected before any DB access. The
// handler has no DB here, so reaching the DB would panic instead of 400.
func TestInvalidIntQueryParamsReturn400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	r := gin.New()
	r.GET("/nuts", h.GetAllNutsRegions)
	r.GET("/nuts/:nuts_id", h.GetNutsRegionData)
	r.GET("/lau/:gisco_id", h.GetLAURegionByID)

	for _, target := range []string{
		"/nuts?limit=abc",
		"/nuts?offset=1.5",
		"/nuts?offset=-1",
		"/nuts?limit=10001",
		"/nuts?chunk_size=10001",
		"/nuts/DE111?year=abc",
		"/lau/DE_11000000?year=2024x",
	} {
		w := doGet(r, target, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", target, w.Code, w.Body.String())
		}
	}
}

func newPaginationContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func TestResolvePaginationChunkSizeCap(t *testing.T) {
	tests := []struct {
		target  string
		wantErr bool
	}{
		{"/?chunk_size=10000", false},
		{"/?chunk_size=10001", true},
		{"/?chunk_size=1000000", true},
		{"/?chunk_size=0", true},
	}
	for _, tt := range tests {
		_, _, err := resolvePagination(newPaginationContext(tt.target), 100, 0, 0, false, 0)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.target, err, tt.wantErr)
		}
	}
}

func TestResolvePaginationLimitNullCap(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		offset  int
		wantErr bool
	}{
		{"small result", 10800, 0, false},
		{"exactly at cap", maxUnlimitedRows, 0, false},
		{"above cap", maxUnlimitedRows + 1, 0, true},
		{"above cap but offset brings it below", maxUnlimitedRows + 10, 10, false},
		{"largest dataset (beschaeftigungsquote-gemeinden) fits", 251676, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, offset, err := resolvePagination(newPaginationContext("/"), 100, 0, tt.offset, true, tt.total)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "chunk_size") {
					t.Errorf("error should point to chunk_size: %v", err)
				}
				return
			}
			if limit != nil || offset != tt.offset {
				t.Errorf("limit=%v offset=%d, want nil/%d", limit, offset, tt.offset)
			}
		})
	}

	// limit=null does not apply when chunk_size is set (chunk paging wins).
	limit, _, err := resolvePagination(newPaginationContext("/?chunk_size=500"), 100, 0, 0, true, maxUnlimitedRows*2)
	if err != nil || limit == nil || *limit != 500 {
		t.Errorf("chunk_size with limit=null: limit=%v err=%v", limit, err)
	}
}

func TestRespondDBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := errors.New(`pq: relation "transformed_data.secret_table" does not exist`)

	tests := []struct {
		name       string
		err        error
		cancelCtx  bool
		wantStatus int
	}{
		{"plain DB error", secret, false, http.StatusInternalServerError},
		{"deadline exceeded", fmt.Errorf("query: %w", context.DeadlineExceeded), false, http.StatusServiceUnavailable},
		{"canceled", context.Canceled, false, http.StatusServiceUnavailable},
		{"request context gone", secret, true, http.StatusServiceUnavailable},
		{"statement_timeout", &pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"}, false, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tt.cancelCtx {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			c.Request = req

			respondDBError(c, "Testabfrage", tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var body models.APIResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Success || body.Error != dbErrorMessage {
				t.Errorf("body = %+v, want generic error %q", body, dbErrorMessage)
			}
			if strings.Contains(w.Body.String(), "secret_table") || strings.Contains(w.Body.String(), "pq:") {
				t.Errorf("response leaks the DB error: %s", w.Body.String())
			}
			if len(c.Errors) != 1 || !strings.Contains(c.Errors.String(), "Testabfrage") {
				t.Errorf("real error not recorded on the context: %v", c.Errors)
			}
		})
	}
}

// /nuts?limit=null must pass validation (gin's query cache still returns
// "null" for limit after normalizeNullableLimitQuery). Without a DB the
// handler panics at the first query; the recovery turns that into 599, which
// proves the request got past the parameter checks instead of a 400.
func TestNutsLimitNullPassesValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	r := gin.New()
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(599)
	}))
	r.GET("/nuts", h.GetAllNutsRegions)

	for _, target := range []string{"/nuts?limit=null", "/nuts?level=3&limit=null"} {
		if w := doGet(r, target, ""); w.Code != 599 {
			t.Errorf("%s: status = %d, want to reach the DB (body %s)", target, w.Code, w.Body.String())
		}
	}
}

func TestCapForLargestTable(t *testing.T) {
	tests := []struct {
		name    string
		largest int64
		want    int64
	}{
		{"unanalyzed table (-1)", -1, maxUnlimitedRows},
		{"empty database", 0, maxUnlimitedRows},
		{"today's biggest dataset gets headroom", 251676, 314595},
		{"just below floor after headroom", 240000, maxUnlimitedRows},
		{"above floor gets 25% headroom", 400000, 500000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capForLargestTable(tt.largest); got != tt.want {
				t.Fatalf("capForLargestTable(%d) = %d, want %d", tt.largest, got, tt.want)
			}
		})
	}
}

// limit=null together with chunk_size is paged, so the cap must not apply -
// neither in resolvePagination nor in the two-step parse/resolve that
// GET /nuts uses (validate before the DB, apply the cap after counting).
func TestPaginationLimitNullWithChunkSizeIgnoresCap(t *testing.T) {
	huge := int(UnlimitedRowsCap()) * 3

	p, err := parsePagination(newPaginationContext("/?chunk_size=500&chunk=2"), 100, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	limit, offset, err := p.resolve(huge)
	if err != nil || limit == nil || *limit != 500 || offset != 1000 {
		t.Errorf("chunked limit=null: limit=%v offset=%d err=%v, want 500/1000/nil", limit, offset, err)
	}

	p, err = parsePagination(newPaginationContext("/"), 100, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.resolve(huge); err == nil {
		t.Error("unpaged limit=null above the cap must be rejected")
	}
	if limit, _, err := p.resolve(10); err != nil || limit != nil {
		t.Errorf("unpaged limit=null below the cap: limit=%v err=%v", limit, err)
	}

	// Parameter errors still surface before any DB access.
	if _, err := parsePagination(newPaginationContext("/?chunk_size=10001"), 100, 0, 0, true); err == nil {
		t.Error("chunk_size above maxChunkSize must fail in parsePagination")
	}
}

func TestApplyUnlimitedRowsCapEnv(t *testing.T) {
	prev := UnlimitedRowsCap()
	t.Cleanup(func() { unlimitedRowsCap.Store(prev) })

	t.Setenv(unlimitedRowsCapEnv, "")
	if _, applied, err := ApplyUnlimitedRowsCapEnv(); applied || err != nil {
		t.Fatalf("unset: applied=%v err=%v", applied, err)
	}
	for _, bad := range []string{"abc", "0", "-5"} {
		t.Setenv(unlimitedRowsCapEnv, bad)
		if _, applied, err := ApplyUnlimitedRowsCapEnv(); applied || err == nil || UnlimitedRowsCap() != prev {
			t.Errorf("%q: applied=%v err=%v cap=%d", bad, applied, err, UnlimitedRowsCap())
		}
	}
	t.Setenv(unlimitedRowsCapEnv, "1234")
	if v, applied, err := ApplyUnlimitedRowsCapEnv(); !applied || err != nil || v != 1234 || UnlimitedRowsCap() != 1234 {
		t.Errorf("valid: v=%d applied=%v err=%v", v, applied, err)
	}
}
