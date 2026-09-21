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
// statt über eine NUTS-Code-Präfixbeziehung.
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
}
