package models

import (
	"database/sql"
	"encoding/json"
)

// AgenturbezirkSummary repräsentiert einen Agenturbezirk der Bundesagentur
// für Arbeit - eine eigenständige Gebietsgliederung, nicht deckungsgleich
// mit NUTS-3/LAU/ARS.
type AgenturbezirkSummary struct {
	BASchl           string          `json:"ba_schl" db:"ba_schl" example:"962"`
	Name             string          `json:"name" db:"name" example:"Berlin Mitte"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" db:"geometry_wgs84" swaggertype:"object"`
	HasValidGeometry bool            `json:"has_valid_geometry" db:"has_valid_geometry"`
	ValidFrom        *string         `json:"valid_from,omitempty" db:"valid_from" example:"2019-06-01"`
	ValidTo          *string         `json:"valid_to,omitempty" db:"valid_to" example:"2050-12-31"`
}

// AgenturbezirkArbeitslosigkeitPoint ist ein Monatswert der Arbeitslosen-
// bzw. Unterbeschäftigungszahlen für einen Agenturbezirk (AfA Lange
// Zeitreihen). Arbeitslose/Unterbeschaeftigung sind null, wenn der Wert in
// der Quelle statistisch gesperrt war ("x").
type AgenturbezirkArbeitslosigkeitPoint struct {
	Year                int             `json:"year" db:"year" example:"2026"`
	Month               int             `json:"month" db:"month" example:"8"`
	ObservationDate     string          `json:"observation_date" db:"observation_date" example:"2026-08-01"`
	Arbeitslose         sql.NullFloat64 `json:"arbeitslose" db:"arbeitslose" swaggertype:"number"`
	Unterbeschaeftigung sql.NullFloat64 `json:"unterbeschaeftigung" db:"unterbeschaeftigung" swaggertype:"number"`
}

// AgenturbezirkLatestValue ist ein Agenturbezirk mit Geometrie und dem
// jeweils neuesten verfügbaren Berichtsmonat (arbeitslose/
// unterbeschaeftigung), um entsprechend die Karte darstellen zu können
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

// AgenturbezirkPeriod ist eine verfügbare (Jahr, Monat)-Kombination, für die
// Agenturbezirk-Arbeitslosenzahlen vorliegen - für die Jahr/Monat-Auswahl in
// der Karte (analog zur Jahres-/Versionsauswahl bei NUTS/LAU).
type AgenturbezirkPeriod struct {
	Year            int    `json:"year" db:"year" example:"2026"`
	Month           int    `json:"month" db:"month" example:"8"`
	ObservationDate string `json:"observation_date" db:"observation_date" example:"2026-08-01"`
}

// AgenturNuts3Share ist eine n:m-Zuordnungszeile Agenturbezirk <-> NUTS-3-
// Region mit Flächenanteil in beide Richtungen (siehe
// mart_agentur_nuts3_crosswalk.sql). Agenturbezirke bestehen in aller
// Regel aus einem oder mehreren ganzen Kreisen (Ausnahme Berlin: 3
// Agenturbezirke teilen sich einen Kreis) - eine räumliche Schätzung,
// keine amtliche Zuordnung. Kein Bevölkerungsanteil - an dieser
// Join-Granularität sind nicht ausreichend Bevölkerungsdaten vorhanden,
// siehe mart_agentur_nuts3_crosswalk.sql.
type AgenturNuts3Share struct {
	Nuts3ID            string  `json:"nuts3_id" db:"nuts_id" example:"DE300"`
	RegionName         *string `json:"region_name,omitempty" db:"region_name" example:"Berlin"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"0.42"`
	AreaShareOfNuts3   float64 `json:"area_share_of_nuts3" db:"area_share_of_nuts3" example:"1.0"`
}

// AgenturDatasetAggregatePoint enthält einen auf Agenturbezirk-Ebene
// flächenanteilsgewichtet hochaggregierten Zeitreihenpunkt eines
// Kreis-grain-Soziodata-Datensatzes - siehe mart_agentur_dataset_aggregate.
// Das Agentur-Pendant zu NutsDatasetAggregatePoint (NUTS-0/1/2), gewichtet
// aber über den n:m-Flächenanteils-Crosswalk (mart_agentur_nuts3_crosswalk)
// statt über eine NUTS-Code-Präfixbeziehung. Quoten/Durchschnitte sind
// bevölkerungsgewichtete Durchschnitte der Kreis-Werte - eine Näherung
// (siehe AggregationMethod/ValueKind), keine exakte Neuberechnung.
type AgenturDatasetAggregatePoint struct {
	BASchl      string  `db:"ba_schl"      json:"ba_schl"      example:"962"`
	DatasetSlug string  `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int     `db:"year"         json:"year"         example:"2022"`
	Value       float64 `db:"value"        json:"value"        example:"340123"`
	// Immer false - keine gap-gefüllte Version wird hier aggregiert. Nicht
	// aus der DB gescannt, damit TimeseriesChart.vue/DataTable.vue dieselbe
	// Interpolationsstatus-Spalte wie bei den anderen Aggregat-Endpunkten
	// anzeigen können.
	IsInterpolated bool `json:"is_interpolated" example:"false"`
	// Immer true: jeder Punkt ist eine flächengewichtete Aggregation über
	// mehrere Kreise, keine an einer einzelnen Quelle abgelesene
	// Einzelbeobachtung.
	IsComputed bool `db:"is_computed" json:"is_computed" example:"true"`
	// Anzahl der Kreise, die für dieses Jahr tatsächlich einen Wert beigetragen haben.
	SourceRegionCount int `db:"source_region_count" json:"source_region_count" example:"3"`
	// Wie der Wert entstanden ist (siehe mart_agentur_dataset_aggregate):
	// "sum" = Summe der Kreis-Werte; "weighted_avg_population" =
	// bevölkerungsgewichteter Durchschnitt der Kreis-Werte;
	// "weighted_avg_area" = flächengewichteter Durchschnitt (Rückfall, wenn
	// für einen beteiligten Kreis die Bevölkerung fehlt). Die beiden
	// Durchschnitte sind eine Näherung, keine exakte Neuberechnung.
	AggregationMethod string `db:"aggregation_method" json:"aggregation_method" example:"weighted_avg_population" enums:"sum,weighted_avg_population,weighted_avg_area"`
	// Art der Größe: "count" (Zählgröße), "rate" (Quote), "mean"
	// (Durchschnitt) oder "median". Mediane lassen sich nicht aus Kreis-
	// Medianen zusammensetzen - der Wert ist dann nur eine Schätzung.
	ValueKind string `db:"value_kind" json:"value_kind" example:"rate" enums:"count,rate,mean,median"`
}

// AgenturDatasetAggregateYear ist eine verfügbare (Datensatz, Jahr)-Kombination
// in mart_agentur_dataset_aggregate - für die Datensatz-/Jahresauswahl der
// Karte (Agenturbezirke).
type AgenturDatasetAggregateYear struct {
	DatasetSlug string `db:"dataset_slug" json:"dataset_slug" example:"bevoelkerung"`
	Year        int    `db:"year"         json:"year"         example:"2022"`
}

// AgenturNutsMembership ist die Einordnung eines Agenturbezirks in eine
// überlappende NUTS-1/2/3-Region.
// IsFullyContained: der Agenturbezirk liegt (mit Toleranz) vollständig in der
// Region. IsParent: kleinste NUTS-Ebene, die ihn vollständig enthält -
// höchstens eine Zeile je Agenturbezirk, keine wenn er über
// Bundeslandgrenzen hinausreicht. Räumliche Schätzung, keine amtliche
// Zuordnung.
type AgenturNutsMembership struct {
	NutsID             string  `json:"nuts_id" db:"nuts_id" example:"DE30"`
	NutsLevel          int     `json:"nuts_level" db:"nuts_level" example:"2"`
	RegionName         *string `json:"region_name,omitempty" db:"region_name" example:"Berlin"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"1.0"`
	IsFullyContained   bool    `json:"is_fully_contained" db:"is_fully_contained"`
	IsParent           bool    `json:"is_parent" db:"is_parent"`
}

// NutsAgenturbezirk ist ein Agenturbezirk, der eine NUTS-Region überlappt
// (GET /nuts/{nuts_id}/agenturbezirke). IsFullyContained unterscheidet
// echte "Kinder" der Region von nur teilweise überlappenden Agenturbezirken.
type NutsAgenturbezirk struct {
	BASchl             string  `json:"ba_schl" db:"ba_schl" example:"962"`
	Name               *string `json:"name,omitempty" db:"name" example:"Berlin Mitte"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"1.0"`
	IsFullyContained   bool    `json:"is_fully_contained" db:"is_fully_contained"`
}
