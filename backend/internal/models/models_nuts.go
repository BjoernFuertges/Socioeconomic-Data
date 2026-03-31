package models

import "encoding/json"

// NutsRegionData enthält alle sozioökonomischen Indikatoren einer NUTS-Region (Level 0–3) für ein Jahr.
// Bei Level 0–2 entspricht region_name mangels Namenstabelle dem NUTS-Code selbst.
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
}

// NutsRegionTimeSeries enthält Metadaten und die Zeitreihe sozioökonomischer Daten.
type NutsRegionTimeSeries struct {
	Metadata      *NutsRegionMetadata     `json:"metadata,omitempty"`
	Socioeconomic []NutsSocioeconomicData `json:"socioeconomic,omitempty"`
}

// NutsRegionSummary ist ein einzelner Eintrag in der NUTS-Regionenliste.
type NutsRegionSummary struct {
	NutsID        string  `db:"nuts_id"        json:"nuts_id"         example:"DE111"`
	RegionName    string  `db:"region_name"    json:"region_name"     example:"Stuttgart, Stadtkreis"`
	NutsLevel     int     `db:"-"              json:"nuts_level"      example:"3"`
	NutsLevelCode string  `db:"-"              json:"nuts_level_code" example:"NUTS-3"`
	ARS           *string `db:"geodata_id_ars" json:"ars"             example:"08111000"`
}

// NutsRegionVersions enthält verfügbare Versionsstände (Jahre) für eine NUTS-Region.
type NutsRegionVersions struct {
	NutsID        string `json:"nuts_id"         example:"DE111"`
	NutsLevel     int    `json:"nuts_level"      example:"3"`
	NutsLevelCode string `json:"nuts_level_code" example:"NUTS-3"`
	Versions      []int  `json:"versions"        example:"2024,2023,2022"`
}
