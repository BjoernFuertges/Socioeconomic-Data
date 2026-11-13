package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// Target versions of the harmonized marts (nuts_version / lau_version).
//
// The supported versions are read once at startup from the data itself
// (ConfigureTargetVersions: distinct target_version of
// mart_nuts3_sociodata_harmonized / mart_lau_sociodata_harmonized, i.e. the
// dbt vars nuts_versions / lau_versions the marts were built with). The
// default - used whenever a request omits nuts_version / lau_version - is the
// newest of them. A new NUTS/LAU version therefore needs no backend change:
// rebuild the marts with the extended dbt vars and restart the backend.
//
// Until ConfigureTargetVersions ran (unit tests without a DB), or when the
// query fails or finds no rows, the static fallback lists below apply.

var (
	fallbackNutsVersions = []int{2003, 2006, 2010, 2013, 2016, 2021, 2024}
	fallbackLauVersions  = []int{2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024}
)

// versionSet is an immutable, ascending, non-empty list of target versions.
type versionSet struct {
	versions []int
}

func newVersionSet(vs []int) *versionSet {
	s := slices.Clone(vs)
	slices.Sort(s)
	return &versionSet{versions: slices.Compact(s)}
}

func (s *versionSet) latest() int { return s.versions[len(s.versions)-1] }

func (s *versionSet) contains(v int) bool {
	_, ok := slices.BinarySearch(s.versions, v)
	return ok
}

// Swapped atomically as a whole, so readers never see a half-updated set.
var (
	nutsTargetVersions atomic.Pointer[versionSet]
	lauTargetVersions  atomic.Pointer[versionSet]
)

func init() { resetTargetVersions() }

// resetTargetVersions restores the static fallback lists (also used by tests).
func resetTargetVersions() {
	nutsTargetVersions.Store(newVersionSet(fallbackNutsVersions))
	lauTargetVersions.Store(newVersionSet(fallbackLauVersions))
}

// supportedNutsVersions returns the NUTS-3 target versions (ascending). The
// slice is shared: do not modify it.
func supportedNutsVersions() []int { return nutsTargetVersions.Load().versions }

// defaultNutsVersion is the target version when nuts_version is not set: the
// newest supported version.
func defaultNutsVersion() int { return nutsTargetVersions.Load().latest() }

func isSupportedNutsVersion(v int) bool { return nutsTargetVersions.Load().contains(v) }

// supportedLauVersions returns the LAU target versions (ascending). The slice
// is shared: do not modify it.
func supportedLauVersions() []int { return lauTargetVersions.Load().versions }

// defaultLauVersion is the target version when lau_version is not set: the
// newest supported version.
func defaultLauVersion() int { return lauTargetVersions.Load().latest() }

func isSupportedLauVersion(v int) bool { return lauTargetVersions.Load().contains(v) }

// distinctTargetVersionsSQL lists the distinct target_version values of a
// harmonized mart as a loose index scan over its (target_version, ...) index
// (idx_mart_*_harm_target), so startup does not scan the whole table. The
// table name is a fixed constant, never user input.
const distinctTargetVersionsSQL = `
WITH RECURSIVE v AS (
	SELECT min(target_version)::int AS tv FROM transformed_data.%[1]s
	UNION ALL
	SELECT (SELECT min(t.target_version)::int FROM transformed_data.%[1]s t WHERE t.target_version > v.tv)
	FROM v WHERE v.tv IS NOT NULL
)
SELECT tv FROM v WHERE tv IS NOT NULL ORDER BY tv`

// TargetVersionsResult reports what ConfigureTargetVersions set up for one
// kind (NUTS or LAU).
type TargetVersionsResult struct {
	Versions []int
	Default  int
	// Fallback is true when the static list is in effect; Err then says why
	// (query error, or no rows).
	Fallback bool
	Err      error
}

func loadTargetVersions(ctx context.Context, db *sqlx.DB, table string, fallback []int, dst *atomic.Pointer[versionSet]) TargetVersionsResult {
	var vs []int
	err := db.SelectContext(ctx, &vs, fmt.Sprintf(distinctTargetVersionsSQL, table))
	if err == nil && len(vs) == 0 {
		err = errors.New("keine Zeilen in transformed_data." + table)
	}
	set := newVersionSet(fallback)
	if err == nil {
		set = newVersionSet(vs)
	}
	dst.Store(set)
	return TargetVersionsResult{Versions: set.versions, Default: set.latest(), Fallback: err != nil, Err: err}
}

// ConfigureTargetVersions reads the available NUTS and LAU target versions
// from the harmonized marts. Call once at startup; each kind falls back to its
// static list independently (see TargetVersionsResult.Fallback).
func ConfigureTargetVersions(db *sqlx.DB) (nuts, lau TargetVersionsResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nuts = loadTargetVersions(ctx, db, "mart_nuts3_sociodata_harmonized", fallbackNutsVersions, &nutsTargetVersions)
	lau = loadTargetVersions(ctx, db, "mart_lau_sociodata_harmonized", fallbackLauVersions, &lauTargetVersions)
	return nuts, lau
}

// TargetVersionList is the set of target versions of one region kind.
type TargetVersionList struct {
	// Ascending list of the supported target versions
	Versions []int `json:"versions" example:"2003,2006,2010,2013,2016,2021,2024"`
	// Version used when the request omits the parameter: the newest one
	Default int `json:"default" example:"2024"`
}

// TargetVersionsResponse lists the supported nuts_version / lau_version values.
type TargetVersionsResponse struct {
	// Values of nuts_version (NUTS-3 harmonized datasets, /metrics)
	Nuts TargetVersionList `json:"nuts"`
	// Values of lau_version (LAU harmonized datasets)
	Lau TargetVersionList `json:"lau"`
}

// GetTargetVersions lists the supported target versions.
// @Summary Unterstützte NUTS-/LAU-Zielversionen
// @Description Listet die Werte, die nuts_version (harmonisierte Kreis-Datensätze, /metrics) und lau_version (harmonisierte LAU-Datensätze) annehmen können, und den jeweiligen Default.
// @Description Ohne Angabe von nuts_version/lau_version liefert die API immer die neueste verfügbare Version (default = Maximum von versions). Kommt eine neue NUTS-/LAU-Version hinzu, ändern sich der Default und damit Regions-IDs (z.B. nach Kreisreformen); wer stabile IDs braucht, gibt die Version explizit an.
// @Description Die Liste wird beim Start des Backends aus den harmonisierten Marts gelesen.
// @Tags Meta
// @Produce json
// @Success 200 {object} models.APIResponse{data=TargetVersionsResponse}
// @Router /versions [get]
func (h *Handler) GetTargetVersions(c *gin.Context) {
	n, l := nutsTargetVersions.Load(), lauTargetVersions.Load()
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: TargetVersionsResponse{
		Nuts: TargetVersionList{Versions: n.versions, Default: n.latest()},
		Lau:  TargetVersionList{Versions: l.versions, Default: l.latest()},
	}})
}
