package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type negotiateTestRow struct {
	ID    string          `json:"id"`
	Value sql.NullFloat64 `json:"value"`
	Opt   *float64        `json:"opt,omitempty"`
	Flag  bool            `json:"flag"`
}

func newNegotiateRouter(rows []negotiateTestRow) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/things", func(c *gin.Context) {
		respondList(c, rows, &models.MetaInfo{Total: len(rows)})
	})
	return r
}

func doGet(r *gin.Engine, target, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRespondListNegotiation(t *testing.T) {
	half := 1.5
	rows := []negotiateTestRow{
		{ID: "a", Value: sql.NullFloat64{Float64: 2.25, Valid: true}, Opt: &half, Flag: true},
		{ID: "b", Flag: false},
	}
	r := newNegotiateRouter(rows)

	tests := []struct {
		name        string
		target      string
		accept      string
		wantStatus  int
		wantType    string
		wantContain string
	}{
		{"default json", "/api/v1/things", "", 200, "application/json", `"success":true`},
		{"wildcard json", "/api/v1/things", "*/*", 200, "application/json", `"success":true`},
		{"accept csv", "/api/v1/things", "text/csv", 200, "text/csv", "id,value,opt,flag\na,2.25,1.5,true\nb,,,false\n"},
		{"browser accept falls back to json", "/api/v1/things", "text/html,application/xhtml+xml,*/*;q=0.8", 200, "application/json", `"success":true`},
		{"format override beats accept", "/api/v1/things?format=csv", "application/json", 200, "text/csv", "id,value,opt,flag\n"},
		{"format json", "/api/v1/things?format=json", "text/csv", 200, "application/json", `"success":true`},
		{"invalid format", "/api/v1/things?format=xml", "", 400, "application/json", "format muss json oder csv sein"},
		{"not acceptable", "/api/v1/things", "application/xml", 406, "application/json", "Nicht unterstütztes Format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doGet(r, tt.target, tt.accept)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tt.wantStatus, w.Body.String())
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, tt.wantType) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tt.wantType)
			}
			if !strings.Contains(w.Body.String(), tt.wantContain) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tt.wantContain)
			}
		})
	}
}

func TestRespondListEmptyCSVHasHeader(t *testing.T) {
	r := newNegotiateRouter([]negotiateTestRow{})
	w := doGet(r, "/api/v1/things", "text/csv")
	if got, want := w.Body.String(), "id,value,opt,flag\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestRespondListDownloadDisposition(t *testing.T) {
	r := newNegotiateRouter(nil)

	w := doGet(r, "/api/v1/things?format=csv&download=1", "")
	if got, want := w.Header().Get("Content-Disposition"), `attachment; filename="things.csv"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}

	w = doGet(r, "/api/v1/things?format=json&download=1", "")
	if got, want := w.Header().Get("Content-Disposition"), `attachment; filename="things.json"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}

	w = doGet(r, "/api/v1/things?format=csv", "")
	if got := w.Header().Get("Content-Disposition"); got != "" {
		t.Errorf("Content-Disposition = %q, want none without download=1", got)
	}
}

type floatRow struct {
	Big   float64  `json:"big"`
	Small float64  `json:"small"`
	Ptr   *float64 `json:"ptr"`
}

// Large values must not end up in exponential notation - spreadsheets
// otherwise read "3.664088E+06" as text instead of a number.
func TestWriteCSVFormatsFloatsPlain(t *testing.T) {
	big := 12876243.0
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/floats", func(c *gin.Context) {
		respondList(c, []floatRow{{Big: 3664088, Small: 8.78, Ptr: &big}}, nil)
	})

	w := doGet(r, "/api/v1/floats", "text/csv")
	if got, want := w.Body.String(), "big,small,ptr\n3664088,8.78,12876243\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
