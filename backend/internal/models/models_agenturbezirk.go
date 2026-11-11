package models

import (
	"database/sql"
	"encoding/json"
)

// AgenturbezirkSummary represents an Agenturbezirk of the Bundesagentur
// für Arbeit - a distinct territorial division, not congruent with
// NUTS-3/LAU/ARS.
type AgenturbezirkSummary struct {
	BASchl           string          `json:"ba_schl" db:"ba_schl" example:"962"`
	Name             string          `json:"name" db:"name" example:"Berlin Mitte"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" db:"geometry_wgs84" swaggertype:"object"`
	HasValidGeometry bool            `json:"has_valid_geometry" db:"has_valid_geometry"`
	ValidFrom        *string         `json:"valid_from,omitempty" db:"valid_from" example:"2019-06-01"`
	ValidTo          *string         `json:"valid_to,omitempty" db:"valid_to" example:"2050-12-31"`
}

// AgenturbezirkArbeitslosigkeitPoint is a monthly value of the Arbeitslose /
// Unterbeschäftigung figures for an Agenturbezirk (AfA long time series).
// Arbeitslose/Unterbeschaeftigung are null when the value was statistically
// suppressed in the source ("x").
type AgenturbezirkArbeitslosigkeitPoint struct {
	Year                int             `json:"year" db:"year" example:"2026"`
	Month               int             `json:"month" db:"month" example:"8"`
	ObservationDate     string          `json:"observation_date" db:"observation_date" example:"2026-08-01"`
	Arbeitslose         sql.NullFloat64 `json:"arbeitslose" db:"arbeitslose" swaggertype:"number"`
	Unterbeschaeftigung sql.NullFloat64 `json:"unterbeschaeftigung" db:"unterbeschaeftigung" swaggertype:"number"`
}

// AgenturbezirkLatestValue is an Agenturbezirk with geometry and its latest
// available reporting month (arbeitslose/unterbeschaeftigung), so the map can
// be rendered accordingly
type AgenturbezirkLatestValue struct {
	BASchl              string          `json:"ba_schl" db:"ba_schl" example:"962"`
	Name                string          `json:"name" db:"name" example:"Berlin Mitte"`
	GeometryWGS84       json.RawMessage `json:"geometry_wgs84,omitempty" db:"geometry_wgs84" swaggertype:"object"`
	HasValidGeometry    bool            `json:"has_valid_geometry" db:"has_valid_geometry"`
	Year                int             `json:"year" db:"year" example:"2026"`
	Month               int             `json:"month" db:"month" example:"8"`
	ObservationDate     string          `json:"observation_date" db:"observation_date" example:"2026-08-01"`
	Arbeitslose         sql.NullFloat64 `json:"arbeitslose" db:"arbeitslose" swaggertype:"number"`
	Unterbeschaeftigung sql.NullFloat64 `json:"unterbeschaeftigung" db:"unterbeschaeftigung" swaggertype:"number"`
}

// AgenturbezirkPeriod is an available (year, month) combination with
// Agenturbezirk Arbeitslose data - for the year/month picker on the map
// (analogous to the year/version picker for NUTS/LAU).
type AgenturbezirkPeriod struct {
	Year            int    `json:"year" db:"year" example:"2026"`
	Month           int    `json:"month" db:"month" example:"8"`
	ObservationDate string `json:"observation_date" db:"observation_date" example:"2026-08-01"`
}

// AgenturNuts3Share is an n:m mapping row Agenturbezirk <-> NUTS-3 region
// with area share in both directions (see mart_agentur_nuts3_crosswalk.sql).
// Agenturbezirke generally consist of one or more whole Kreise (exception
// Berlin: 3 Agenturbezirke share one Kreis) - a spatial estimate, not an
// official mapping. No population share - at this join granularity there is
// not enough population data, see mart_agentur_nuts3_crosswalk.sql.
type AgenturNuts3Share struct {
	Nuts3ID            string  `json:"nuts3_id" db:"nuts_id" example:"DE300"`
	RegionName         *string `json:"region_name,omitempty" db:"region_name" example:"Berlin"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"0.42"`
	AreaShareOfNuts3   float64 `json:"area_share_of_nuts3" db:"area_share_of_nuts3" example:"1.0"`
}

// AgenturDatasetAggregatePoint holds a time-series point of a Kreis-grain
// Soziodata dataset, aggregated to Agenturbezirk level weighted by area
// share - see mart_agentur_dataset_aggregate. The Agentur counterpart of
// NutsDatasetAggregatePoint (NUTS-0/1/2), but weighted via the n:m area-share
// crosswalk (mart_agentur_nuts3_crosswalk) instead of a NUTS-code prefix
// relation. Rates/averages are population-weighted averages of the Kreis
// values - an approximation (see AggregationMethod/ValueKind), not an exact
// recomputation.
type AgenturDatasetAggregatePoint struct {
	BASchl      string  `db:"ba_schl"      json:"ba_schl"      example:"962"`
	DatasetSlug string  `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int     `db:"year"         json:"year"         example:"2022"`
	Value       float64 `db:"value"        json:"value"        example:"340123"`
	// Always false - no gap-filled version is aggregated here. Not scanned from
	// the DB, so TimeseriesChart.vue/DataTable.vue can show the same
	// interpolation-status column as for the other aggregate endpoints.
	IsInterpolated bool `json:"is_interpolated" example:"false"`
	// Always true: every point is an area-weighted aggregation over several
	// Kreise, not a single observation read from one source.
	IsComputed bool `db:"is_computed" json:"is_computed" example:"true"`
	// Number of Kreise that actually contributed a value for this year.
	SourceRegionCount int `db:"source_region_count" json:"source_region_count" example:"3"`
	// How the value was derived (see mart_agentur_dataset_aggregate):
	// "sum" = sum of the Kreis values; "weighted_avg_population" =
	// population-weighted average of the Kreis values; "weighted_avg_area" =
	// area-weighted average (fallback when population is missing for a
	// contributing Kreis). The two averages are an approximation, not an exact
	// recomputation.
	AggregationMethod string `db:"aggregation_method" json:"aggregation_method" example:"weighted_avg_population" enums:"sum,weighted_avg_population,weighted_avg_area"`
	// Kind of quantity: "count", "rate", "mean" or "median". Medians cannot be
	// composed from Kreis medians - the value is then only an estimate.
	ValueKind string `db:"value_kind" json:"value_kind" example:"rate" enums:"count,rate,mean,median"`
}

// AgenturDatasetAggregateYear is an available (dataset, year) combination in
// mart_agentur_dataset_aggregate - for the dataset/year picker of the map
// (Agenturbezirke).
type AgenturDatasetAggregateYear struct {
	DatasetSlug string `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int    `db:"year"         json:"year"         example:"2022"`
}

// AgenturNutsMembership places an Agenturbezirk in an overlapping NUTS-1/2/3
// region.
// IsFullyContained: the Agenturbezirk lies fully (with tolerance) within the
// region. IsParent: smallest NUTS level that fully contains it - at most one
// row per Agenturbezirk, none if it extends beyond Bundesland borders.
// Spatial estimate, not an official mapping.
type AgenturNutsMembership struct {
	NutsID             string  `json:"nuts_id" db:"nuts_id" example:"DE30"`
	NutsLevel          int     `json:"nuts_level" db:"nuts_level" example:"2"`
	RegionName         *string `json:"region_name,omitempty" db:"region_name" example:"Berlin"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"1.0"`
	IsFullyContained   bool    `json:"is_fully_contained" db:"is_fully_contained"`
	IsParent           bool    `json:"is_parent" db:"is_parent"`
}

// NutsAgenturbezirk is an Agenturbezirk that overlaps a NUTS region
// (GET /nuts/{nuts_id}/agenturbezirke). IsFullyContained distinguishes true
// "children" of the region from only partially overlapping Agenturbezirke.
type NutsAgenturbezirk struct {
	BASchl             string  `json:"ba_schl" db:"ba_schl" example:"962"`
	Name               *string `json:"name,omitempty" db:"name" example:"Berlin Mitte"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"1.0"`
	IsFullyContained   bool    `json:"is_fully_contained" db:"is_fully_contained"`
}
