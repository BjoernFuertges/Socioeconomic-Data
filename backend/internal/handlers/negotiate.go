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

// csvRender ist ein gin-Renderer, der eine Struct-Slice als CSV schreibt.
// Die Spaltennamen kommen aus den vorhandenen json-Tags der Modelle, sodass
// JSON- und CSV-Antwort dieselben Feldnamen tragen.
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

// csvMarshalers sind einmal gebaut (Marshalers sind laut Dokumentation
// unveränderlich) statt bei jeder Anfrage neu über das inzwischen veraltete
// Encoder.Register: MarshalFunc ist typsicher und schneller, da es ohne
// Reflection pro Encode-Aufruf auskommt.
var csvMarshalers = csvutil.NewMarshalers(
	// Ohne eigenen Marshaler schreibt csvutil große Werte in Exponential-
	// schreibweise ("3.664088E+06"), was Tabellenkalkulationen als Text lesen.
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

// writeCSV kodiert rows (Slice von Structs) als CSV; ein leerer Slice ergibt
// nur die Kopfzeile. database/sql.Null*-Typen werden als Wert bzw. leeres
// Feld geschrieben statt als Struct.
func writeCSV(w http.ResponseWriter, rows interface{}) error {
	cw := csv.NewWriter(w)
	enc := csvutil.NewEncoder(cw)
	enc.Tag = "json"
	enc.WithMarshalers(csvMarshalers)

	// Encode schreibt bei einem leeren Slice gar nichts - die Kopfzeile wird
	// dann separat erzeugt, damit die Antwort ein gültiges CSV bleibt.
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

// emptySliceElement meldet, ob rows ein leerer Slice/Array ist, und liefert
// dazu einen Nullwert des Elementtyps für die Kopfzeile (nil, wenn sich
// daraus keine Kopfzeile ableiten lässt).
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

// negotiateFormat bestimmt das Antwortformat ("json" oder "csv"). Der
// Accept-Header ist maßgeblich (fehlender Header oder */* = JSON);
// ?format=csv|json überschreibt ihn für Browser-Links, die keinen Accept-
// Header setzen können. ok=false: es wurde bereits eine Fehlerantwort
// geschrieben.
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

// respondNegotiated antwortet mit jsonBody (als APIResponse.Data) oder, wenn
// CSV angefragt ist, mit csvRows (Slice von Structs). ?download=1 setzt
// Content-Disposition: attachment, damit Browser die Datei speichern.
func respondNegotiated(c *gin.Context, jsonBody interface{}, csvRows interface{}, meta *models.MetaInfo) {
	format, ok := negotiateFormat(c)
	if !ok {
		return
	}

	// Antwort hängt vom Accept-Header ab -> Caches müssen danach unterscheiden.
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

// respondList ist respondNegotiated für Endpoints, deren Data-Feld selbst
// die CSV-Zeilen-Slice ist.
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

// contentDisposition leitet den Dateinamen aus dem Request-Pfad ab
// (/api/v1/bevoelkerung/timeseries -> bevoelkerung_timeseries.csv).
func contentDisposition(c *gin.Context, ext string) string {
	name := strings.Trim(strings.TrimPrefix(c.Request.URL.Path, "/api/v1"), "/")
	name = strings.NewReplacer("/", "_", " ", "_", "\"", "").Replace(path.Clean(name))
	if name == "" || name == "." {
		name = "daten"
	}
	return fmt.Sprintf("attachment; filename=%q", name+"."+ext)
}
