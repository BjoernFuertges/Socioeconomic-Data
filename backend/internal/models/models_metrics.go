package models

// MetricsFilter holds the query parameters of the metrics endpoint.
type MetricsFilter struct {
	Year          *int   `form:"year"`
	FromYear      *int   `form:"from_year"`
	ToYear        *int   `form:"to_year"`
	NutsParentID  string `form:"nuts_parent_id"`
	GeodataIDNUTS string `form:"geodata_id_nuts"`
	// NutsVersion selects the NUTS-3 target version of the harmonized values
	// (mart_nuts3_sociodata_harmonized.target_version); default 2024, allowed
	// values as for SozioKreisFilter.NutsVersion.
	NutsVersion *int `form:"nuts_version"`
}

// DatasetYearMetrics holds statistical figures for a dataset in one year.
type DatasetYearMetrics struct {
	Year              int      `json:"year" example:"2022"`
	RegionCount       int      `json:"region_count" example:"400"`
	Min               float64  `json:"min" example:"1.2"`
	Max               float64  `json:"max" example:"42.7"`
	Mean              float64  `json:"mean" example:"12.3"`
	Median            float64  `json:"median" example:"11.8"`
	StdDev            float64  `json:"stddev" example:"4.5"`
	YoYMeanPct        *float64 `json:"yoy_mean_pct,omitempty" example:"2.1"`
	YoYMedianPct      *float64 `json:"yoy_median_pct,omitempty" example:"1.8"`
	RegionalValue     *float64 `json:"regional_value,omitempty" example:"9.4"`
	RegionalRank      *int     `json:"regional_rank,omitempty" example:"142"`
	RegionalRankTotal *int     `json:"regional_rank_total,omitempty" example:"400"`
	// ExcludedInvalidCount is the number of region values of this year left
	// out as invalid (has_valid_value = false, missing value; for derived
	// metrics: numerator, denominator or factor invalid, or a zero
	// denominator of a ratio; a zero factor of a product is valid).
	ExcludedInvalidCount int `json:"excluded_invalid_count" example:"2"`
	// RegionsWithoutGeometry (derived metrics with the area denominator only)
	// counts regions of this year left out because no NUTS-3 geometry matches.
	RegionsWithoutGeometry int `json:"regions_without_geometry,omitempty" example:"1"`
	// RegionsWithoutDenominator (derived metrics with a dataset denominator
	// only) counts regions of this year without a denominator value.
	RegionsWithoutDenominator int `json:"regions_without_denominator,omitempty" example:"0"`
	// RegionsWithoutFactor (derived metrics of formula type product only)
	// counts regions of this year without a factor value.
	RegionsWithoutFactor int `json:"regions_without_factor,omitempty" example:"0"`
}

// DatasetMetricsResult holds metrics for a dataset and a value column across all years.
type DatasetMetricsResult struct {
	Dataset string `json:"dataset" example:"schuldnerquote"`
	Column  string `json:"column" example:"schuldnerquote_prozent"`
	// NutsVersion is the NUTS-3 version the values were harmonized to; absent
	// when the column has no harmonized counterpart and the unharmonized
	// staging values were used.
	NutsVersion *int `json:"nuts_version,omitempty" example:"2024"`
	// ExcludedInvalidCount sums DatasetYearMetrics.ExcludedInvalidCount over
	// all years of the filter (also years without any valid value, which are
	// missing from by_year).
	ExcludedInvalidCount int `json:"excluded_invalid_count" example:"5"`
	// ExcludedRegions is the number of distinct regions with at least one
	// value left out for any reason (invalid, no geometry, no denominator,
	// no factor).
	ExcludedRegions int `json:"excluded_regions" example:"3"`
	// RegionsWithoutGeometry / RegionsWithoutDenominator (ratio) and
	// RegionsWithoutFactor (product): distinct regions left out at least once
	// for that reason (derived metrics only).
	RegionsWithoutGeometry    int                  `json:"regions_without_geometry,omitempty" example:"1"`
	RegionsWithoutDenominator int                  `json:"regions_without_denominator,omitempty" example:"0"`
	RegionsWithoutFactor      int                  `json:"regions_without_factor,omitempty" example:"0"`
	ByYear                    []DatasetYearMetrics `json:"by_year"`
}
