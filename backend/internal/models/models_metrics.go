package models

// MetricsFilter holds the query parameters of the metrics endpoint.
type MetricsFilter struct {
	Year          *int   `form:"year"`
	FromYear      *int   `form:"from_year"`
	ToYear        *int   `form:"to_year"`
	NutsParentID  string `form:"nuts_parent_id"`
	GeodataIDNUTS string `form:"geodata_id_nuts"`
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
}

// DatasetMetricsResult holds metrics for a dataset and a value column across all years.
type DatasetMetricsResult struct {
	Dataset string               `json:"dataset" example:"schuldnerquote"`
	Column  string               `json:"column" example:"schuldnerquote_prozent"`
	ByYear  []DatasetYearMetrics `json:"by_year"`
}
