package models

// BevoelkerungData repräsentiert Bevölkerungsdaten je Kreis
type BevoelkerungData struct {
	OriginalID1     string  `json:"original_id_1" db:"original_id_1" example:"01001"`
	OriginalName    string  `json:"original_name" db:"original_name" example:"Flensburg, Stadt"`
	GeodataName     string  `json:"geodata_name" db:"geodata_name" example:"Flensburg"`
	GeodataIDNUTS   string  `json:"geodata_id_nuts" db:"geodata_id_nuts" example:"DEF01"`
	GeodataIDARS    string  `json:"geodata_id_ars" db:"geodata_id_ars" example:"01001"`
	Mapper          string  `json:"mapper" db:"mapper" example:"exact_id"`
	Parameter       string  `json:"parameter" db:"parameter" example:"id_ars"`
	GeodataLevel    string  `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	GeodataYear     int     `json:"geodata_year" db:"geodata_year" example:"2024"`
	Year            int     `json:"year" db:"year" example:"2020"`
	Bevoelkerung    float64 `json:"bevoelkerung" db:"bevoelkerung" example:"89934"`
	ObservationDate string  `json:"observation_date" db:"observation_date" example:"2020-01-01"`
	HasValidValue   bool    `json:"has_valid_value" db:"has_valid_value" example:"true"`
}

// BevoelkerungFilter für Query-Parameter
type BevoelkerungFilter struct {
	GeodataIDNUTS string `form:"geodata_id_nuts" example:"DEF01"`
	Year          *int   `form:"year" example:"2020"`
	Limit         int    `form:"limit" binding:"omitempty,min=1,max=10000" example:"100"`
	Offset        int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// SchuldnerquoteData repräsentiert Schuldnerquoten je Kreis
type SchuldnerquoteData struct {
	OriginalID1           string  `json:"original_id_1" db:"original_id_1" example:"01001"`
	OriginalName          string  `json:"original_name" db:"original_name" example:"Flensburg, Stadt"`
	GeodataName           string  `json:"geodata_name" db:"geodata_name" example:"Flensburg"`
	GeodataIDNUTS         string  `json:"geodata_id_nuts" db:"geodata_id_nuts" example:"DEF01"`
	GeodataIDARS          string  `json:"geodata_id_ars" db:"geodata_id_ars" example:"01001"`
	Mapper                string  `json:"mapper" db:"mapper" example:"exact_id"`
	Parameter             string  `json:"parameter" db:"parameter" example:"id_ars"`
	GeodataLevel          string  `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	GeodataYear           int     `json:"geodata_year" db:"geodata_year" example:"2024"`
	Year                  int     `json:"year" db:"year" example:"2020"`
	SchuldnerquoteProzent float64 `json:"schuldnerquote_prozent" db:"schuldnerquote_prozent" example:"14.96"`
	ObservationDate       string  `json:"observation_date" db:"observation_date" example:"2020-01-01"`
	HasValidValue         bool    `json:"has_valid_value" db:"has_valid_value" example:"true"`
}

// SchuldnerquoteFilter für Query-Parameter
type SchuldnerquoteFilter struct {
	GeodataIDNUTS string `form:"geodata_id_nuts" example:"DEF01"`
	Year          *int   `form:"year" example:"2020"`
	Limit         int    `form:"limit" binding:"omitempty,min=1,max=10000" example:"100"`
	Offset        int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}
