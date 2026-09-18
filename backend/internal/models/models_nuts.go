package models

import "encoding/json"

// NutsRegionData enthält alle sozioökonomischen Indikatoren einer NUTS-Region (Level 0–3) für ein Jahr.
type NutsRegionData struct {
	NutsID     string `db:"nuts_id"                json:"nuts_id"                example:"DE111"`
	RegionName string `db:"region_name"            json:"region_name"            example:"Stuttgart, Stadtkreis"`
	Year       int    `db:"year"                   json:"year"                   example:"2022"`
	// Kann null sein: nur für NUTS-3, ca. 80% Abdeckung
	GeodataIDArs          *string  `db:"geodata_id_ars"         json:"-"`
	Bevoelkerung          *float64 `db:"bevoelkerung"           json:"bevoelkerung"           example:"634830"`
	SchuldnerquoteProzent *float64 `db:"schuldnerquote_prozent" json:"schuldnerquote_prozent" example:"9.8"`
	// Kann null sein: Arbeitsagenturbezirke entsprechen nicht 1:1 den NUTS-Kreisen
	Arbeitslose      *float64 `db:"arbeitslose"        json:"arbeitslose"        example:"17423"`
	YoYChangePercent *float64 `db:"yoy_change_percent" json:"yoy_change_percent" example:"-2.3"`
}

// NutsRegionMetadata enthält geographische Metadaten einer NUTS-Region.
type NutsRegionMetadata struct {
	NutsID        string `json:"nuts_id"         example:"DE111"`
	RegionName    string `json:"region_name"     example:"Stuttgart, Stadtkreis"`
	NutsLevel     int    `json:"nuts_level"      example:"3"`
	NutsLevelCode string `json:"nuts_level_code" example:"NUTS-3"`
	// ARS (Amtlicher Regionalschlüssel) – nur für NUTS-3 verfügbar, sonst null.
	ARS              *string         `json:"ars" example:"08111000"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" swaggertype:"object"`
	HasValidGeometry bool            `json:"has_valid_geometry" example:"false"`
}

// NutsSocioeconomicData enthält die sozioökonomischen Indikatoren für ein Jahr.
type NutsSocioeconomicData struct {
	Year                  int      `json:"year"                   example:"2022"`
	Bevoelkerung          *float64 `json:"bevoelkerung"           example:"634830"`
	SchuldnerquoteProzent *float64 `json:"schuldnerquote_prozent" example:"9.8"`
	Arbeitslose           *float64 `json:"arbeitslose"            example:"17423"`
	YoYChangePercent      *float64 `json:"yoy_change_percent"     example:"-2.3"`
}

// NutsRegionResponse trennt Metadaten von sozioökonomischen Daten für ein einzelnes Jahr.
type NutsRegionResponse struct {
	Metadata      *NutsRegionMetadata    `json:"metadata,omitempty"`
	Socioeconomic *NutsSocioeconomicData `json:"socioeconomic,omitempty"`
	Datasets      []RegionDataset        `json:"datasets,omitempty"`
}

// NutsRegionTimeSeries enthält Metadaten und die Zeitreihe sozioökonomischer Daten.
type NutsRegionTimeSeries struct {
	Metadata      *NutsRegionMetadata     `json:"metadata,omitempty"`
	Socioeconomic []NutsSocioeconomicData `json:"socioeconomic,omitempty"`
	Datasets      []RegionDataset         `json:"datasets,omitempty"`
}

// NutsRegionSummary ist ein einzelner Eintrag in der NUTS-Regionenliste.
type NutsRegionSummary struct {
	NutsID        string  `db:"nuts_id"        json:"nuts_id"         example:"DE111"`
	RegionName    string  `db:"region_name"    json:"region_name"     example:"Stuttgart, Stadtkreis"`
	NutsLevel     int     `db:"-"              json:"nuts_level"      example:"3"`
	NutsLevelCode string  `db:"-"              json:"nuts_level_code" example:"NUTS-3"`
	ARS           *string `db:"geodata_id_ars" json:"ars"             example:"08111000"`
}

// NutsRegionVersions enthält die Geometrie-Perioden einer NUTS-Region.
// Jede Periode repräsentiert einen Zeitraum, in dem eine Geometrie-Version gültig war.
// ValidTo ist nil, wenn die Version noch aktuell ist.
type NutsRegionVersions struct {
	NutsID        string      `json:"nuts_id"         example:"DE111"`
	NutsLevel     int         `json:"nuts_level"      example:"3"`
	NutsLevelCode string      `json:"nuts_level_code" example:"NUTS-3"`
	Periods       []GeoPeriod `json:"periods"`
}

// NutsLauAggregatePoint enthält die auf NUTS-3-Ebene hochaggregierten
// LAU-Werte (Gemeinden) für ein Jahr - siehe mart_nuts3_lau_aggregate.
// Ergänzt/kreuzvalidiert die nativen Kreis-Werte aus NutsSocioeconomicData
// (z.B. Bevölkerung direkt auf Kreisebene vs. Summe der Gemeinden).
type NutsLauAggregatePoint struct {
	Year int `db:"year" json:"year" example:"2022"`
	// Kann null sein, wenn für keine zugeordnete Gemeinde ein Bevölkerungswert vorliegt.
	LauPopulationSum *float64 `db:"lau_population_sum" json:"lau_population_sum" example:"634830"`
	// Anzahl der diesem Kreis zugeordneten Gemeinden.
	LauRegionCount int `db:"lau_region_count" json:"lau_region_count" example:"23"`
	// Davon mit einem tatsächlichen Bevölkerungswert (<=lau_region_count).
	LauRegionCountWithPopulation int `db:"lau_region_count_with_population" json:"lau_region_count_with_population" example:"21"`
	// Bevölkerungsgewichteter Durchschnitt der Gemeinde-Beschäftigungsquote; null ohne Datenabdeckung.
	LauBeschaeftigungsquoteWeightedAvg *float64 `db:"lau_beschaeftigungsquote_weighted_avg" json:"lau_beschaeftigungsquote_weighted_avg" example:"64.2"`
}

// NutsLauAggregateResponse bündelt die Zeitreihe der LAU-Aggregate für eine NUTS-3-Region.
type NutsLauAggregateResponse struct {
	NutsID string                  `json:"nuts_id" example:"DE300"`
	Series []NutsLauAggregatePoint `json:"series"`
}

// NutsDatasetAggregatePoint enthält einen auf NUTS-0/1/2-Ebene hochaggregierten
// Zeitreihenpunkt eines Kreis-grain-Soziodata-Datensatzes - siehe
// mart_nuts012_dataset_aggregate. Das Pendant zu NutsTimeseriesPoint (NUTS-3),
// aber ohne Interpolation: es fließen nur tatsächlich vorhandene Kreis-Werte
// ein (siehe SourceRegionCount), keine Imputationen.
type NutsDatasetAggregatePoint struct {
	NutsID      string  `db:"nuts_id"      json:"nuts_id"      example:"DE1"`
	DatasetSlug string  `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int     `db:"year"         json:"year"         example:"2022"`
	Value       float64 `db:"value"        json:"value"        example:"12876243"`
	// Immer false - siehe Typkommentar oben (keine gap-gefüllte Version wird hier aggregiert).
	// Nicht aus der DB gescannt (kein db-Tag), damit TimeseriesChart.vue/DataTable.vue
	// dieselbe Interpolationsstatus-Spalte wie bei NutsTimeseriesPoint anzeigen können.
	IsInterpolated bool `json:"is_interpolated" example:"false"`
	// Immer true (siehe SELECT true AS is_computed in GetNutsDatasetAggregateTimeseries):
	// jeder Punkt ist eine Aggregation über mehrere Kreise, keine an einer
	// einzelnen Quelle abgelesene Einzelbeobachtung.
	IsComputed bool `db:"is_computed" json:"is_computed" example:"true"`
	// Anzahl der Kreise, die für dieses Jahr tatsächlich einen Wert beigetragen haben.
	SourceRegionCount int `db:"source_region_count" json:"source_region_count" example:"44"`
}
