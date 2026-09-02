package models

import (
	"database/sql"
	"encoding/json"
)

// LAURegion repräsentiert eine LAU-Region mit Geometrie und Population
type LAURegion struct {
	GiscoID          string          `json:"gisco_id" db:"gisco_id" example:"DE111"`
	Year             int             `json:"year" db:"year" example:"2024"`
	LAUName          string          `json:"lau_name" db:"lau_name" example:"Stuttgart"`
	CountryCode      string          `json:"country_code" db:"country_code" example:"DE"`
	LAUID            sql.NullString  `json:"lau_id,omitempty" db:"lau_id" swaggertype:"string" example:"08111000"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" db:"geometry_wgs84" swaggertype:"object"`
	HasValidGeometry bool            `json:"has_valid_geometry" db:"has_valid_geometry" example:"true"`
	LAUNameNational  sql.NullString  `json:"lau_name_national,omitempty" db:"lau_name_national" swaggertype:"string"`
	LAUNameLatin     sql.NullString  `json:"lau_name_latin,omitempty" db:"lau_name_latin" swaggertype:"string"`
	NUTS3Code        sql.NullString  `json:"nuts_3_code,omitempty" db:"nuts_3_code" swaggertype:"string"`
	// NUTS3ID ist die kanonische LAU->NUTS-3-Verlinkung: deklarierte
	// Quellspalte (nuts_3_code/nuts_3), sonst räumlich ermittelter Fallback
	// (siehe int_lau_nuts3_spatial_link.sql). Robuster als NUTS3Code, das
	// oft NULL ist, wenn die Quelldaten keinen NUTS-3-Code führen.
	NUTS3ID sql.NullString `json:"nuts3_id,omitempty" db:"nuts3_id" swaggertype:"string"`
	// NUTS3LinkSource kennzeichnet, woher NUTS3ID stammt: 'declared' (aus
	// Quelldaten) oder 'spatial_estimate' (räumlich ermittelt, keine
	// amtliche Zuordnung) - siehe lau_integrated.sql.
	NUTS3LinkSource       sql.NullString  `json:"nuts3_link_source,omitempty" db:"nuts3_link_source" swaggertype:"string"`
	LAUNatCode            sql.NullString  `json:"lau_nat_code,omitempty" db:"lau_nat_code" swaggertype:"string"`
	PopulationCount       sql.NullInt64   `json:"population_count,omitempty" db:"population_count" swaggertype:"integer"`
	Country               sql.NullString  `json:"country,omitempty" db:"country" swaggertype:"string"`
	HasValidPopulation    sql.NullBool    `json:"has_valid_population,omitempty" db:"has_valid_population" swaggertype:"boolean"`
	MissingPopulationData bool            `json:"missing_population_data" db:"missing_population_data" example:"false"`
	YearMatch             bool            `json:"year_match" db:"year_match" example:"true"`
	Datasets              []RegionDataset `json:"datasets,omitempty" db:"-"`
}

// LAURegionFilter für Query-Parameter
type LAURegionFilter struct {
	CountryCode         string `form:"country_code" example:"DE"`
	NutsParentID        string `form:"nuts_parent_id" example:"DE1"`
	GeodataNameContains string `form:"geodata_name_contains" example:"stuttgart"`
	Point               string `form:"point" example:"13.404954,52.520008"`
	Year                *int   `form:"year" example:"2024"`
	FromYear            *int   `form:"from_year" example:"2022"`
	ToYear              *int   `form:"to_year" example:"2024"`
	HasPopulation       *bool  `form:"has_population" example:"true"`
	Limit               int    `form:"limit" binding:"omitempty,min=1,max=1000" example:"100"`
	Offset              int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// GeoRegionSummary ist ein einheitlicher Listen-Eintrag für NUTS- und LAU-Regionen.
type GeoRegionSummary struct {
	GeoID            string          `json:"geo_id" example:"DE111"`
	GeoType          string          `json:"geo_type" example:"NUTS"`
	RegionName       string          `json:"region_name" example:"Stuttgart, Stadtkreis"`
	NutsLevel        *int            `json:"nuts_level,omitempty" example:"3"`
	NutsLevelCode    *string         `json:"nuts_level_code,omitempty" example:"NUTS-3"`
	CountryCode      *string         `json:"country_code,omitempty" example:"DE"`
	Year             int             `json:"vintage_year" example:"2024"`
	GeometryWGS84    json.RawMessage `json:"geometry_wgs84,omitempty" swaggertype:"object"`
	HasValidGeometry *bool           `json:"has_valid_geometry,omitempty" example:"true"`
	Datasets         []RegionDataset `json:"datasets,omitempty"`
}

// GeoRegionFilter für Query-Parameter am allgemeinen Geo-Listenendpunkt.
type GeoRegionFilter struct {
	GeoType             string `form:"geo_type"`
	Level               *int   `form:"level" binding:"omitempty,min=0,max=3"`
	CountryCode         string `form:"country_code"`
	NutsParentID        string `form:"nuts_parent_id"`
	GeodataNameContains string `form:"geodata_name_contains"`
	Point               string `form:"point"`
	IncludeGeometry     bool   `form:"include_geometry"`
	GeometryYear        *int   `form:"geometry_year"`
	GeometryStrategy    string `form:"geometry_strategy"`
	Year                *int   `form:"year"`
	FromYear            *int   `form:"from_year"`
	ToYear              *int   `form:"to_year"`
	Limit               int    `form:"limit" binding:"omitempty,min=1,max=1000"`
	Offset              int    `form:"offset" binding:"omitempty,min=0"`
}

// GeoPeriod repräsentiert einen Gültigkeitszeitraum einer Geometrie-Version.
// ValidTo ist nil, wenn die Version noch aktuell ist.
type GeoPeriod struct {
	ValidFrom int  `json:"valid_from" db:"valid_from" example:"2016"`
	ValidTo   *int `json:"valid_to"   db:"valid_to"   example:"2023" swaggertype:"integer"`
}

// GeoRegionVersions enthält die Geometrie-Perioden einer Geo-Region (NUTS oder LAU).
// Jede Periode repräsentiert eine Version der Geometrie; ValidTo=null bedeutet noch aktuell.
type GeoRegionVersions struct {
	GeoID         string      `json:"geo_id"                    example:"DE111"`
	GeoType       string      `json:"geo_type"                  example:"NUTS"`
	NutsLevel     *int        `json:"nuts_level,omitempty"      example:"3"`
	NutsLevelCode *string     `json:"nuts_level_code,omitempty" example:"NUTS-3"`
	Periods       []GeoPeriod `json:"periods"`
}

// GeoSpatialQueryRequest beschreibt eine räumliche Abfrage mit GeoJSON-Geometrie.
type GeoSpatialQueryRequest struct {
	GeoType          string          `json:"geo_type" example:"nuts"`
	Operation        string          `json:"operation" example:"intersects"`
	Geometry         json.RawMessage `json:"geometry" swaggertype:"object"`
	IncludeGeometry  bool            `json:"include_geometry" example:"false"`
	IncludeDatasets  bool            `json:"include_datasets,omitempty" example:"false"`
	GeometryStrategy string          `json:"geometry_strategy" example:"latest"`
	GeometryYear     *int            `json:"geometry_year,omitempty" example:"2024"`
	Year             *int            `json:"year,omitempty" example:"2024"`
	FromYear         *int            `json:"from_year,omitempty" example:"2020"`
	ToYear           *int            `json:"to_year,omitempty" example:"2024"`
	Limit            int             `json:"limit,omitempty" example:"100"`
	Offset           int             `json:"offset,omitempty" example:"0"`
}
