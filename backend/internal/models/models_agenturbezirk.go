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
	// Nuts3ID ist eine räumliche Schätzung (größte Schnittfläche, siehe
	// int_agentur_nuts3_spatial_link.sql), keine amtliche Zuordnung -
	// verweist auf den ergänzenden Kreis-Level-Datensatz "Arbeitslose".
	Nuts3ID *string `json:"nuts3_id,omitempty" db:"nuts3_id" example:"DE300"`
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
	Nuts3ID             *string         `json:"nuts3_id,omitempty" db:"nuts3_id" example:"DE300"`
	Year                int             `json:"year" db:"year" example:"2026"`
	Month               int             `json:"month" db:"month" example:"8"`
	ObservationDate     string          `json:"observation_date" db:"observation_date" example:"2026-08-01"`
	Arbeitslose         sql.NullFloat64 `json:"arbeitslose" db:"arbeitslose" swaggertype:"number"`
	Unterbeschaeftigung sql.NullFloat64 `json:"unterbeschaeftigung" db:"unterbeschaeftigung" swaggertype:"number"`
}
