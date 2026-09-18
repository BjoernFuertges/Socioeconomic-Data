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

// AgenturNuts3Share ist eine n:m-Zuordnungszeile Agenturbezirk <-> NUTS-3-
// Region mit Flächenanteil in beide Richtungen (siehe
// mart_agentur_nuts3_crosswalk.sql). Agenturbezirke bestehen in aller
// Regel aus einem oder mehreren ganzen Kreisen (Ausnahme Berlin: 3
// Agenturbezirke teilen sich einen Kreis) - eine räumliche Schätzung,
// keine amtliche Zuordnung. Kein Bevölkerungsanteil (lokal keine validen
// LAU-Bevölkerungsdaten geladen, siehe mart_agentur_nuts3_crosswalk.sql).
type AgenturNuts3Share struct {
	Nuts3ID            string  `json:"nuts3_id" db:"nuts_id" example:"DE300"`
	RegionName         *string `json:"region_name,omitempty" db:"region_name" example:"Berlin"`
	AreaShareOfAgentur float64 `json:"area_share_of_agentur" db:"area_share_of_agentur" example:"0.42"`
	AreaShareOfNuts3   float64 `json:"area_share_of_nuts3" db:"area_share_of_nuts3" example:"1.0"`
}
