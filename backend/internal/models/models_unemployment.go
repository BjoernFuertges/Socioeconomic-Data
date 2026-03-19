package models

import "time"

// UnemploymentData repräsentiert Arbeitslosenzahlen
type UnemploymentData struct {
	ArbeitslosenzahlenPK    string    `json:"arbeitslosenzahlen_pk" db:"arbeitslosenzahlen_pk"`
	GeodataID               string    `json:"geodata_id" db:"geodata_id" example:"DE111"`
	GeodataName             string    `json:"geodata_name" db:"geodata_name" example:"Stuttgart, Stadtkreis"`
	GeodataIDARS            string    `json:"geodata_id_ars" db:"geodata_id_ars" example:"08111000"`
	GeodataLevel            string    `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	GeodataYear             int       `json:"geodata_year" db:"geodata_year" example:"2021"`
	Year                    int       `json:"year" db:"year" example:"2024"`
	ObservationDate         time.Time `json:"observation_date" db:"observation_date" example:"2024-01-01T00:00:00Z"`
	Arbeitslose             float64   `json:"arbeitslose" db:"arbeitslose" example:"20749"`
	ArbeitslosePreviousYear *float64  `json:"arbeitslose_previous_year,omitempty" db:"arbeitslose_previous_year" example:"18416"`
	YoYChangePercent        *float64  `json:"yoy_change_percent,omitempty" db:"yoy_change_percent" example:"12.67"`
}

// UnemploymentFilter für Query-Parameter
type UnemploymentFilter struct {
	GeodataID string `form:"geodata_id" example:"DE111"`
	FromDate  string `form:"from_date" example:"2024-01-01"` // Format: YYYY-MM-DD
	ToDate    string `form:"to_date" example:"2024-12-31"`   // Format: YYYY-MM-DD
	Year      *int   `form:"year" example:"2024"`
	Limit     int    `form:"limit" binding:"omitempty,min=1,max=10000" example:"100"`
	Offset    int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// UnemploymentAggregation repräsentiert aggregierte Statistiken
type UnemploymentAggregation struct {
	GeodataID           string    `json:"geodata_id" db:"geodata_id" example:"DE111"`
	GeodataName         string    `json:"geodata_name" db:"geodata_name" example:"Stuttgart, Stadtkreis"`
	TotalRecords        int       `json:"total_records" db:"total_records" example:"351"`
	AvgArbeitslose      float64   `json:"avg_arbeitslose" db:"avg_arbeitslose" example:"18622.03"`
	MinArbeitslose      float64   `json:"min_arbeitslose" db:"min_arbeitslose" example:"13334"`
	MaxArbeitslose      float64   `json:"max_arbeitslose" db:"max_arbeitslose" example:"31655"`
	LatestObservation   time.Time `json:"latest_observation" db:"latest_observation" example:"2024-12-31T00:00:00Z"`
	EarliestObservation time.Time `json:"earliest_observation" db:"earliest_observation" example:"1998-01-01T00:00:00Z"`
}
