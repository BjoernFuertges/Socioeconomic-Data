package models

// RegionDataRow is a row of the combined long-format export
// (GET /regions/{region_type}/{region_id}/data). Raw and aggregated values
// share the same schema; is_computed distinguishes them.
type RegionDataRow struct {
	RegionType  string `db:"region_type"  json:"region_type"  example:"nuts" enums:"nuts,lau,agentur"`
	RegionID    string `db:"region_id"    json:"region_id"    example:"DE300"`
	DatasetSlug string `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int    `db:"year"         json:"year"         example:"2022"`
	// Only for monthly values (Agenturbezirk raw values), otherwise empty.
	Month *int     `db:"month" json:"month" example:"8"`
	Value *float64 `db:"value" json:"value" example:"340123"`
	// true = computed from several source regions, false = the region's original value.
	IsComputed bool `db:"is_computed" json:"is_computed"`
	// true = not an original value but imputed (NUTS-3/LAU raw values only).
	IsInterpolated bool `db:"is_interpolated" json:"is_interpolated"`
	// Only for aggregated values: sum, weighted_avg_population, weighted_avg_area.
	AggregationMethod *string `db:"aggregation_method" json:"aggregation_method" example:"weighted_avg_population"`
	// Only for Agentur aggregates: count, rate, mean, median.
	ValueKind *string `db:"value_kind" json:"value_kind" example:"rate"`
	// Number of source regions (Kreise/Gemeinden) behind an aggregated value.
	SourceRegionCount *int `db:"source_region_count" json:"source_region_count" example:"3"`
}
