package models

import "encoding/json"

// NutsRegionData holds all socioeconomic indicators of a NUTS region (levels 0-3) for one year.
type NutsRegionData struct {
	NutsID     string `db:"nuts_id"                json:"nuts_id"                example:"DE111"`
	RegionName string `db:"region_name"            json:"region_name"            example:"Stuttgart, Stadtkreis"`
	Year       int    `db:"year"                   json:"year"                   example:"2022"`
	// May be null: NUTS-3 only, ca. 80% coverage
	GeodataIDArs          *string  `db:"geodata_id_ars"         json:"-"`
	Bevoelkerung          *float64 `db:"bevoelkerung"           json:"bevoelkerung"           example:"634830"`
	SchuldnerquoteProzent *float64 `db:"schuldnerquote_prozent" json:"schuldnerquote_prozent" example:"9.8"`
	// May be null: Arbeitsagenturbezirke do not map 1:1 to NUTS Kreise
	Arbeitslose      *float64 `db:"arbeitslose"        json:"arbeitslose"        example:"17423"`
	YoYChangePercent *float64 `db:"yoy_change_percent" json:"yoy_change_percent" example:"-2.3"`
}

// NutsRegionMetadata holds geographic metadata of a NUTS region.
type NutsRegionMetadata struct {
	NutsID        string `json:"nuts_id"         example:"DE111"`
	RegionName    string `json:"region_name"     example:"Stuttgart, Stadtkreis"`
	NutsLevel     int    `json:"nuts_level"      example:"3"`
	NutsLevelCode string `json:"nuts_level_code" example:"NUTS-3"`
	// ARS (Amtlicher Regionalschlüssel) - available for NUTS-3 only, otherwise null.
	ARS              *string         `json:"ars" example:"08111000"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" swaggertype:"object"`
	HasValidGeometry bool            `json:"has_valid_geometry" example:"false"`
}

// NutsSocioeconomicData holds the socioeconomic indicators for one year.
type NutsSocioeconomicData struct {
	Year                  int      `json:"year"                   example:"2022"`
	Bevoelkerung          *float64 `json:"bevoelkerung"           example:"634830"`
	SchuldnerquoteProzent *float64 `json:"schuldnerquote_prozent" example:"9.8"`
	Arbeitslose           *float64 `json:"arbeitslose"            example:"17423"`
	YoYChangePercent      *float64 `json:"yoy_change_percent"     example:"-2.3"`
}

// NutsRegionResponse separates metadata from socioeconomic data for a single year.
type NutsRegionResponse struct {
	Metadata      *NutsRegionMetadata    `json:"metadata,omitempty"`
	Socioeconomic *NutsSocioeconomicData `json:"socioeconomic,omitempty"`
	Datasets      []RegionDataset        `json:"datasets,omitempty"`
}

// NutsRegionTimeSeries holds metadata and the time series of socioeconomic data.
type NutsRegionTimeSeries struct {
	Metadata      *NutsRegionMetadata     `json:"metadata,omitempty"`
	Socioeconomic []NutsSocioeconomicData `json:"socioeconomic,omitempty"`
	Datasets      []RegionDataset         `json:"datasets,omitempty"`
}

// NutsRegionSummary is a single entry in the NUTS region list.
type NutsRegionSummary struct {
	NutsID        string  `db:"nuts_id"        json:"nuts_id"         example:"DE111"`
	RegionName    string  `db:"region_name"    json:"region_name"     example:"Stuttgart, Stadtkreis"`
	NutsLevel     int     `db:"-"              json:"nuts_level"      example:"3"`
	NutsLevelCode string  `db:"-"              json:"nuts_level_code" example:"NUTS-3"`
	ARS           *string `db:"geodata_id_ars" json:"ars"             example:"08111000"`
}

// NutsRegionVersions holds the geometry periods of a NUTS region.
// Each period represents a time span in which a geometry version was valid.
// ValidTo is nil if the version is still current.
type NutsRegionVersions struct {
	NutsID        string      `json:"nuts_id"         example:"DE111"`
	NutsLevel     int         `json:"nuts_level"      example:"3"`
	NutsLevelCode string      `json:"nuts_level_code" example:"NUTS-3"`
	Periods       []GeoPeriod `json:"periods"`
}

// NutsLauAggregatePoint holds the LAU values (Gemeinden) aggregated to NUTS-3
// level for one year - see mart_nuts3_lau_aggregate.
// Complements/cross-validates the native Kreis values from NutsSocioeconomicData
// (e.g. population directly at Kreis level vs. sum of the Gemeinden).
type NutsLauAggregatePoint struct {
	Year int `db:"year" json:"year" example:"2022"`
	// May be null if no assigned Gemeinde has a population value.
	LauPopulationSum *float64 `db:"lau_population_sum" json:"lau_population_sum" example:"634830"`
	// Number of Gemeinden assigned to this Kreis.
	LauRegionCount int `db:"lau_region_count" json:"lau_region_count" example:"23"`
	// Of these, with an actual population value (<=lau_region_count).
	LauRegionCountWithPopulation int `db:"lau_region_count_with_population" json:"lau_region_count_with_population" example:"21"`
	// Population-weighted average of the Gemeinde Beschäftigungsquote; null without data coverage.
	LauBeschaeftigungsquoteWeightedAvg *float64 `db:"lau_beschaeftigungsquote_weighted_avg" json:"lau_beschaeftigungsquote_weighted_avg" example:"64.2"`
	// Number of Gemeinden with a valid Beschäftigungsquote that went into the average
	// (0 without quote). Independent of the population counts above.
	LauBeschaeftigungsquoteRegionCount int `db:"lau_beschaeftigungsquote_region_count" json:"lau_beschaeftigungsquote_region_count" example:"21"`
}

// NutsLauAggregateResponse bundles the time series of LAU aggregates for a NUTS-3 region.
type NutsLauAggregateResponse struct {
	NutsID string                  `json:"nuts_id" example:"DE300"`
	Series []NutsLauAggregatePoint `json:"series"`
}

// NutsDatasetAggregatePoint holds a time-series point of a Kreis-grain
// Soziodata dataset, aggregated to NUTS-0/1/2 level - see
// mart_nuts012_dataset_aggregate. The counterpart of NutsTimeseriesPoint
// (NUTS-3), but without interpolation: only Kreis values that actually exist
// are included (see SourceRegionCount), no imputations.
type NutsDatasetAggregatePoint struct {
	NutsID      string  `db:"nuts_id"      json:"nuts_id"      example:"DE1"`
	DatasetSlug string  `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int     `db:"year"         json:"year"         example:"2022"`
	Value       float64 `db:"value"        json:"value"        example:"12876243"`
	// Always false - see type comment above (no gap-filled version is aggregated
	// here). Not scanned from the DB (no db tag), so TimeseriesChart.vue/DataTable.vue
	// can show the same interpolation-status column as for NutsTimeseriesPoint.
	IsInterpolated bool `json:"is_interpolated" example:"false"`
	// Always true (see SELECT true AS is_computed in GetNutsDatasetAggregateTimeseries):
	// every point is an aggregation over several Kreise, not a single
	// observation read from one source.
	IsComputed bool `db:"is_computed" json:"is_computed" example:"true"`
	// Number of Kreise that actually contributed a value for this year.
	SourceRegionCount int `db:"source_region_count" json:"source_region_count" example:"44"`
}
