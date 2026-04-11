package models

//go:generate go run ../../cmd/gen-models/main.go

// SozioBaseData enthält die gemeinsamen Felder aller Kreis-Datensätze.
// Sie entsprechen den Basis-Spalten in den dbt-Staging-Modellen.
type SozioBaseData struct {
	OriginalID1     string `json:"original_id_1" db:"original_id_1" example:"01001"`
	OriginalName    string `json:"original_name" db:"original_name" example:"Flensburg, Stadt"`
	GeodataName     string `json:"geodata_name" db:"geodata_name" example:"Flensburg"`
	GeodataIDNUTS   string `json:"geodata_id_nuts" db:"geodata_id_nuts" example:"DEF01"`
	GeodataIDLAU    string `json:"geodata_id_lau" db:"geodata_id_lau" example:"01001000"`
	GeodataIDARS    string `json:"geodata_id_ars" db:"geodata_id_ars" example:"01001"`
	Mapper          string `json:"mapper" db:"mapper" example:"exact_id"`
	Parameter       string `json:"parameter" db:"parameter" example:"id_ars"`
	GeodataLevel    string `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	GeodataYear     int    `json:"geodata_year" db:"geodata_year" example:"2024"`
	Year            int    `json:"year" db:"year" example:"2020"`
	ObservationDate string `json:"observation_date" db:"observation_date" example:"2020-01-01"`
	HasValidValue   bool   `json:"has_valid_value" db:"has_valid_value" example:"true"`
}

// SozioKreisFilter sind die einheitlichen Query-Parameter für alle Kreis-Datensätze.
type SozioKreisFilter struct {
	GeodataIDNUTS       string `form:"geodata_id_nuts" example:"DEF01"`
	NutsParentID        string `form:"nuts_parent_id" example:"DE1"`
	GeodataNameContains string `form:"geodata_name_contains" example:"stuttgart"`
	Year                *int   `form:"year" example:"2020"`
	FromYear            *int   `form:"from_year" example:"2018"`
	ToYear              *int   `form:"to_year" example:"2022"`
	Point               string `form:"point" example:"13.404954,52.520008"`
	Limit               int    `form:"limit" binding:"omitempty,min=1,max=10000" example:"100"`
	Offset              int    `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// KreisDatasetConfig beschreibt einen Kreis-Datensatz für das generische Query-System.
// Instanzen werden von cmd/gen-models aus den dbt-Schema-YAML-Dateien generiert.
type KreisDatasetConfig struct {
	Schema       string
	Table        string
	ValueColumns []string // datensatzspezifische DB-Spaltennamen (zusätzlich zu SozioBaseData)
	DefaultLimit int
	OrderBy      string
	RouteSlug    string
}

// SoziooekonomischesInventarEintrag beschreibt die Datenverfügbarkeit je Datensatz, Jahr und Geodatenebene.
type SoziooekonomischesInventarEintrag struct {
	Dataset             string `json:"dataset" db:"dataset" example:"arbeitslose"`
	Year                int    `json:"year" db:"year" example:"2022"`
	GeodataLevel        string `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	Records             int    `json:"records" db:"records" example:"400"`
	DistinctGeodataNUTS int    `json:"distinct_geodata_nuts" db:"distinct_geodata_nuts" example:"400"`
	DistinctGeodataLAU  int    `json:"distinct_geodata_lau" db:"distinct_geodata_lau" example:"0"`
	DistinctGeodataARS  int    `json:"distinct_geodata_ars" db:"distinct_geodata_ars" example:"400"`
}

// RegionDataset beschreibt einen verfügbaren Soziodata-Datensatz für eine Region.
type RegionDataset struct {
	Dataset        string `json:"dataset" example:"bevoelkerung"`
	RouteSlug      string `json:"route_slug" example:"bevoelkerung"`
	AvailableYears []int  `json:"available_years" example:"2020,2021,2022"`
}

// RegionDatasets enthält alle verfügbaren Datensätze für eine Geo-Region.
type RegionDatasets struct {
	GeoID    string          `json:"geo_id" example:"DEF01"`
	GeoType  string          `json:"geo_type" example:"NUTS"`
	Datasets []RegionDataset `json:"datasets"`
}

// AllSoziodataConfigs liefert alle bekannten Soziodata-Datensatzkonfigurationen.
func AllSoziodataConfigs() []KreisDatasetConfig {
	return []KreisDatasetConfig{
		ArbeitsloseConfig,
		ArbeitsvolumenConfig,
		AuzubildendeConfig,
		BeschaeftigungsquoteGemeindenConfig,
		BeschaeftigungsquoteNuts3Config,
		BevoelkerungConfig,
		BruttoverdienstConfig,
		DurchschnittsalterConfig,
		ErwerbstaetigePro1000EinwohnerConfig,
		KinderarmutConfig,
		MedianeinkommenConfig,
		SchuldnerquoteConfig,
		SgbIiQuoteConfig,
		StudierendeConfig,
		WohngeldhaushalteConfig,
	}
}

// DatasetConfigBySlug gibt die KreisDatasetConfig für einen gegebenen RouteSlug zurück.
func DatasetConfigBySlug(slug string) (KreisDatasetConfig, bool) {
	for _, cfg := range AllSoziodataConfigs() {
		if cfg.RouteSlug == slug {
			return cfg, true
		}
	}
	return KreisDatasetConfig{}, false
}
