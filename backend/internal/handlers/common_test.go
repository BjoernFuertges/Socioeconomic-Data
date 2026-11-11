package handlers

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolvePaginationRejectsChunkOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?chunk_size=1000&chunk="+strconv.Itoa(math.MaxInt/10), nil)

	if _, _, err := resolvePagination(c, 100, 0, 0, false); err == nil {
		t.Fatal("expected an error for chunk*chunk_size overflow")
	}
}

func TestResolvePaginationChunkOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?chunk_size=50&chunk=3", nil)

	limit, offset, err := resolvePagination(c, 100, 0, 0, false)
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
		"/nuts/DE111?year=abc",
		"/lau/DE_11000000?year=2024x",
	} {
		w := doGet(r, target, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", target, w.Code, w.Body.String())
		}
	}
}
