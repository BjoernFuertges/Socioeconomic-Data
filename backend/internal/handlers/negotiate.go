package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/jszwec/csvutil"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

const mimeCSV = "text/csv"

// csvRender is a gin renderer that writes a struct slice as CSV. Column names
// come from the models' existing json tags, so the JSON and CSV responses
// carry the same field names.
type csvRender struct {
	Rows interface{}
}

var _ render.Render = csvRender{}

func (r csvRender) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if header.Get("Content-Type") == "" {
		header["Content-Type"] = []string{mimeCSV + "; charset=utf-8"}
	}
}

func (r csvRender) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return writeCSV(w, r.Rows)
}

// csvMarshalers are built once (marshalers are documented as immutable)
// instead of per request via the now-deprecated Encoder.Register:
// MarshalFunc is type-safe and faster since it needs no reflection per
// Encode call.
var csvMarshalers = csvutil.NewMarshalers(
	// Without its own marshaler csvutil writes large values in exponential
	// notation ("3.664088E+06"), which spreadsheets read as text.
	csvutil.MarshalFunc(func(v float64) ([]byte, error) {
		return []byte(strconv.FormatFloat(v, 'f', -1, 64)), nil
	}),
	csvutil.MarshalFunc(func(v sql.NullFloat64) ([]byte, error) {
		if !v.Valid {
			return nil, nil
		}
		return []byte(strconv.FormatFloat(v.Float64, 'f', -1, 64)), nil
	}),
	csvutil.MarshalFunc(func(v sql.NullInt64) ([]byte, error) {
		if !v.Valid {
			return nil, nil
		}
		return []byte(strconv.FormatInt(v.Int64, 10)), nil
	}),
	csvutil.MarshalFunc(func(v sql.NullString) ([]byte, error) {
		if !v.Valid {
			return nil, nil
		}
		return []byte(v.String), nil
	}),
	csvutil.MarshalFunc(func(v sql.NullBool) ([]byte, error) {
		if !v.Valid {
			return nil, nil
		}
		return []byte(strconv.FormatBool(v.Bool)), nil
	}),
)

// writeCSV encodes rows (a slice of structs) as CSV; an empty slice yields
// only the header row. database/sql.Null* types are written as their value or
// an empty field rather than as a struct.
func writeCSV(w http.ResponseWriter, rows interface{}) error {
	cw := csv.NewWriter(w)
	enc := csvutil.NewEncoder(cw)
	enc.Tag = "json"
	enc.WithMarshalers(csvMarshalers)

	// Encode writes nothing for an empty slice - the header row is then produced
	// separately so the response stays valid CSV.
	if elem, empty := emptySliceElement(rows); empty {
		if elem == nil {
			return nil
		}
		if err := enc.EncodeHeader(elem); err != nil {
			return err
		}
	} else if err := enc.Encode(rows); err != nil {
		return err
	}

	cw.Flush()
	return cw.Error()
}

// emptySliceElement reports whether rows is an empty slice/array and returns
// a zero value of the element type for the header row (nil if no header can
// be derived from it).
func emptySliceElement(rows interface{}) (elem interface{}, empty bool) {
	v := reflect.ValueOf(rows)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, true
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, false
	}
	if v.Len() > 0 {
		return nil, false
	}

	elemType := v.Type().Elem()
	for elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return nil, true
	}
	return reflect.New(elemType).Elem().Interface(), true
}

// negotiateFormat determines the response format ("json" or "csv"). The
// Accept header is authoritative (missing header or */* = JSON);
// ?format=csv|json overrides it for browser links that cannot set an Accept
// header. ok=false: an error response has already been written.
func negotiateFormat(c *gin.Context) (format string, ok bool) {
	if override := strings.ToLower(strings.TrimSpace(c.Query("format"))); override != "" {
		switch override {
		case "json", "csv":
			return override, true
		default:
			respondBadRequest(c, fmt.Sprintf("format muss json oder csv sein, erhalten %q", override))
			return "", false
		}
	}

	switch c.NegotiateFormat(gin.MIMEJSON, mimeCSV) {
	case mimeCSV:
		return "csv", true
	case gin.MIMEJSON:
		return "json", true
	default:
		c.JSON(http.StatusNotAcceptable, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Nicht unterstütztes Format im Accept-Header; angeboten: %s, %s", gin.MIMEJSON, mimeCSV),
		})
		return "", false
	}
}

// respondNegotiated responds with jsonBody (as APIResponse.Data) or, if CSV
// is requested, with csvRows (a slice of structs). ?download=1 sets
// Content-Disposition: attachment so browsers save the file.
func respondNegotiated(c *gin.Context, jsonBody interface{}, csvRows interface{}, meta *models.MetaInfo) {
	format, ok := negotiateFormat(c)
	if !ok {
		return
	}

	// The response depends on the Accept header -> caches must vary on it.
	c.Header("Vary", "Accept")

	if format == "json" {
		if isDownloadRequested(c) {
			c.Header("Content-Disposition", contentDisposition(c, "json"))
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: jsonBody, Meta: meta})
		return
	}

	if isDownloadRequested(c) {
		c.Header("Content-Disposition", contentDisposition(c, "csv"))
	}
	c.Render(http.StatusOK, csvRender{Rows: csvRows})
}

// respondList is respondNegotiated for endpoints whose Data field is itself
// the CSV row slice.
func respondList(c *gin.Context, rows interface{}, meta *models.MetaInfo) {
	respondNegotiated(c, rows, rows, meta)
}

func isDownloadRequested(c *gin.Context) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query("download"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// contentDisposition derives the file name from the request path
// (/api/v1/bevoelkerung/timeseries -> bevoelkerung_timeseries.csv).
func contentDisposition(c *gin.Context, ext string) string {
	name := strings.Trim(strings.TrimPrefix(c.Request.URL.Path, "/api/v1"), "/")
	name = strings.NewReplacer("/", "_", " ", "_", "\"", "").Replace(path.Clean(name))
	if name == "" || name == "." {
		name = "daten"
	}
	return fmt.Sprintf("attachment; filename=%q", name+"."+ext)
}
