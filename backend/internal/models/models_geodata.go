package models

import (
	"database/sql"
	"encoding/json"
)

// LAURegion repräsentiert eine LAU-Region mit Geometrie und Population
type LAURegion struct {
	GiscoID               string          `json:"gisco_id" db:"gisco_id" example:"DE111"`
	Year                  int             `json:"year" db:"year" example:"2024"`
	LAUName               string          `json:"lau_name" db:"lau_name" example:"Stuttgart"`
	CountryCode           string          `json:"country_code" db:"country_code" example:"DE"`
	LAUID                 string          `json:"lau_id" db:"lau_id" example:"08111000"`
	GeometryWGS84         json.RawMessage `json:"geometry_wgs84,omitempty" db:"geometry_wgs84" swaggertype:"object"`
	HasValidGeometry      bool            `json:"has_valid_geometry" db:"has_valid_geometry" example:"true"`
	LAUNameNational       sql.NullString  `json:"lau_name_national,omitempty" db:"lau_name_national" swaggertype:"string"`
	LAUNameLatin          sql.NullString  `json:"lau_name_latin,omitempty" db:"lau_name_latin" swaggertype:"string"`
	NUTS3Code             sql.NullString  `json:"nuts_3_code,omitempty" db:"nuts_3_code" swaggertype:"string"`
	LAUNatCode            sql.NullString  `json:"lau_nat_code,omitempty" db:"lau_nat_code" swaggertype:"string"`
	PopulationCount       sql.NullInt64   `json:"population_count,omitempty" db:"population_count" swaggertype:"integer"`
	Country               sql.NullString  `json:"country,omitempty" db:"country" swaggertype:"string"`
	HasValidPopulation    sql.NullBool    `json:"has_valid_population,omitempty" db:"has_valid_population" swaggertype:"boolean"`
	MissingPopulationData bool            `json:"missing_population_data" db:"missing_population_data" example:"false"`
	YearMatch             bool            `json:"year_match" db:"year_match" example:"true"`
}

// LAURegionFilter für Query-Parameter
type LAURegionFilter struct {
	CountryCode   string `form:"country_code" example:"DE"`
	Year          *int   `form:"year" example:"2024"`
	HasPopulation *bool  `form:"has_population" example:"true"`
	Limit         int    `form:"limit" binding:"omitempty,min=1,max=1000" example:"100"`
	Offset        int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}
