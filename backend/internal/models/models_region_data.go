package models

// RegionDataRow ist eine Zeile des kombinierten Long-Format-Exports
// (GET /regions/{region_type}/{region_id}/data). Rohwerte und aggregierte
// Werte teilen sich dasselbe Schema; is_computed unterscheidet sie.
type RegionDataRow struct {
	RegionType  string `db:"region_type"  json:"region_type"  example:"nuts" enums:"nuts,lau,agentur"`
	RegionID    string `db:"region_id"    json:"region_id"    example:"DE300"`
	DatasetSlug string `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int    `db:"year"         json:"year"         example:"2022"`
	// Nur bei Monatswerten (Agenturbezirk-Rohwerte), sonst leer.
	Month *int     `db:"month" json:"month" example:"8"`
	Value *float64 `db:"value" json:"value" example:"340123"`
	// true = aus mehreren Quellregionen berechnet, false = Ursprungswert der Region.
	IsComputed bool `db:"is_computed" json:"is_computed"`
	// true = kein Originalwert, sondern imputiert (nur NUTS-3/LAU-Rohwerte).
	IsInterpolated bool `db:"is_interpolated" json:"is_interpolated"`
	// Nur bei aggregierten Werten: sum, weighted_avg_population, weighted_avg_area.
	AggregationMethod *string `db:"aggregation_method" json:"aggregation_method" example:"weighted_avg_population"`
	// Nur bei Agentur-Aggregaten: count, rate, mean, median.
	ValueKind *string `db:"value_kind" json:"value_kind" example:"rate"`
	// Anzahl der Quellregionen (Kreise/Gemeinden) hinter einem aggregierten Wert.
	SourceRegionCount *int `db:"source_region_count" json:"source_region_count" example:"3"`
}
