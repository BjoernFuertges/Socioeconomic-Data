package models

//go:generate go run ../../cmd/gen-models/main.go

// SozioBaseData enthält die gemeinsamen Felder aller Kreis-Datensätze.
// Sie entsprechen den Basis-Spalten in den dbt-Staging-Modellen.
type SozioBaseData struct {
	OriginalID1     string   `json:"original_id_1" db:"original_id_1" example:"01001"`
	OriginalName    string   `json:"original_name" db:"original_name" example:"Flensburg, Stadt"`
	GeodataName     string   `json:"geodata_name" db:"geodata_name" example:"Flensburg"`
	GeodataIDNUTS   string   `json:"geodata_id_nuts" db:"geodata_id_nuts" example:"DEF01"`
	GeodataIDLAU    string   `json:"geodata_id_lau" db:"geodata_id_lau" example:"01001000"`
	GeodataIDARS    string   `json:"geodata_id_ars" db:"geodata_id_ars" example:"01001"`
	Mapper          string   `json:"mapper" db:"mapper" example:"exact_id"`
	Parameter       string   `json:"parameter" db:"parameter" example:"id_ars"`
	GeodataLevel    string   `json:"geodata_level" db:"geodata_level" example:"NUTS_3"`
	GeodataYear     int      `json:"geodata_year" db:"geodata_year" example:"2024"`
	Year            int      `json:"year" db:"year" example:"2020"`
	ObservationDate string   `json:"observation_date" db:"observation_date" example:"2020-01-01"`
	HasValidValue   bool     `json:"has_valid_value" db:"has_valid_value" example:"true"`
	IsInterpolated  *bool    `json:"is_interpolated,omitempty" db:"is_interpolated" example:"true"`
	ValueLinear     *float64 `json:"value_linear,omitempty" db:"value_linear" example:"8.78"`
	ValuePeerTrend  *float64 `json:"value_peer_trend,omitempty" db:"value_peer_trend" example:"8.50"`
	NPeerRegions    *int     `json:"n_peer_regions,omitempty" db:"n_peer_regions" example:"11"`
	// NutsVersion ist die NUTS-3-Zielversion dieser Zeile. Nur bei fill_gaps=true
	// befüllt (siehe api_*_fill_gaps-Views), damit fill_gaps=true und
	// /timeseries?nuts_version= für denselben Datensatz konsistent dieselbe
	// Zielversion erkennbar zurückgeben.
	NutsVersion *int `json:"nuts_version,omitempty" db:"nuts_version" example:"2021"`
	// LauVersion ist das Pendant zu NutsVersion für LAU-Datensätze (z. B.
	// beschaeftigungsquote-gemeinden). Nur bei fill_gaps=true auf einem
	// IsHarmonizedLau-Datensatz befüllt (siehe api_*_fill_gaps-Views für LAU).
	LauVersion *int `json:"lau_version,omitempty" db:"lau_version" example:"2021"`
}

// SozioKreisFilter sind die einheitlichen Query-Parameter für alle Kreis-Datensätze.
type SozioKreisFilter struct {
	GeodataIDNUTS string `form:"geodata_id_nuts" example:"DEF01"`
	// GeodataIDLAU filtert Timeseries-Abfragen auf LAU-Datensätzen (siehe
	// getLauTimeseriesData) direkt nach gisco_id - das LAU-Pendant zu
	// GeodataIDNUTS, das auf Kreis-Datensätzen einen NUTS-3-Code erwartet.
	GeodataIDLAU        string `form:"geodata_id_lau" example:"DE_11000000"`
	NutsParentID        string `form:"nuts_parent_id" example:"DE1"`
	GeodataNameContains string `form:"geodata_name_contains" example:"stuttgart"`
	Year                *int   `form:"year" example:"2020"`
	FromYear            *int   `form:"from_year" example:"2018"`
	ToYear              *int   `form:"to_year" example:"2022"`
	Point               string `form:"point" example:"13.404954,52.520008"`
	// FillGaps schaltet auf die lückenlose, interpolierte Zeitreihe aus
	// mart_nuts3_timeseries_interpolated um (nur bei IsHarmonized-Datensätzen
	// unterstützt, siehe KreisDatasetConfig.IsHarmonized).
	FillGaps bool `form:"fill_gaps" example:"false"`
	// NutsVersion wählt die NUTS-3-Zielversion für die harmonisierte
	// Zeitreihe (getKreisTimeseriesEndpoint). Nur zusammen mit dem
	// /timeseries-Endpoint relevant; erlaubte Werte siehe supportedNutsVersions
	// in kreis_generic.go. Ohne Angabe: 2024 (bisheriges Verhalten).
	NutsVersion *int `form:"nuts_version" example:"2021"`
	// LauVersion wählt die LAU-Zielversion für die harmonisierte Zeitreihe
	// eines LAU-Datensatzes (getLauTimeseriesEndpoint). Analog NutsVersion,
	// aber für LAU-Datensätze (IsHarmonizedLau); erlaubte Werte siehe
	// supportedLauVersions in lau_generic.go. Ohne Angabe: aktuellste Version.
	LauVersion *int `form:"lau_version" example:"2021"`
	Limit      int  `form:"limit" binding:"omitempty,min=1,max=10000" example:"100"`
	Offset     int  `form:"offset" binding:"omitempty,min=0" example:"0"`
}

// KreisDatasetConfig beschreibt einen Kreis-Datensatz für das generische Query-System.
// Instanzen werden von cmd/gen-models aus den dbt-Schema-YAML-Dateien generiert.
type KreisDatasetConfig struct {
	Schema            string
	Table             string
	InterpolatedTable string
	ValueColumns      []string // datensatzspezifische DB-Spaltennamen (zusätzlich zu SozioBaseData)
	DefaultLimit      int
	OrderBy           string
	RouteSlug         string
	// IsHarmonized = true, wenn dieser Datensatz mit demselben Slug (RouteSlug)
	// auch in mart_nuts3_sociodata_harmonized / mart_nuts3_timeseries_interpolated
	// enthalten ist. Nur dann unterstützt der Endpoint den fill_gaps-Query-
	// Parameter (lückenlose Zeitreihe inkl. Interpolationskennzeichnung).
	IsHarmonized bool
	// IsHarmonizedLau ist das LAU-Pendant zu IsHarmonized: true, wenn dieser
	// Datensatz mit demselben Slug in mart_lau_sociodata_harmonized /
	// mart_lau_timeseries_interpolated enthalten ist. Steuert, ob der
	// generierte Endpoint fill_gaps mit lau_version (statt nuts_version)
	// unterstützt (siehe kreis_generic.go / lau_generic.go). Ein Datensatz
	// ist immer nur entweder IsHarmonized ODER IsHarmonizedLau, nie beides.
	IsHarmonizedLau bool
}

// NutsTimeseriesPoint repräsentiert einen Zeitreihenpunkt aus
// transformed_data.mart_nuts3_timeseries_interpolated - wird von allen
// harmonisierten Kreis-Endpoints zurückgegeben, wenn ?fill_gaps=true gesetzt
// ist. Enthält den amtlichen Ausgabewert (value) sowie - transparent für
// API-Konsumenten - beide alternativen Imputationsschätzungen für fehlende
// Jahre (Methode 1: lineare Interpolation, Methode 3: peer-basierte
// Fortschreibung).
type NutsTimeseriesPoint struct {
	NutsID      string `db:"nuts3_id"        json:"nuts_id"        example:"DE226"`
	DatasetSlug string `db:"dataset_slug"    json:"dataset_slug"   example:"wohngeldhaushalte"`
	// NutsVersion ist die NUTS-3-Zielversion dieses Zeitreihenpunkts (2016, 2021 oder 2024).
	NutsVersion int     `db:"target_version"  json:"nuts_version"   example:"2021"`
	Year        int     `db:"year"            json:"year"           example:"2021"`
	Value       float64 `db:"value"           json:"value"          example:"8.78"`
	// true = Wert wurde imputiert (kein Originalwert vorhanden für dieses Jahr).
	IsInterpolated bool `db:"is_interpolated" json:"is_interpolated" example:"true"`
	// Methode-1-Schätzung (lineare Interpolation). Nur bei is_interpolated = true befüllt.
	ValueLinear *float64 `db:"value_linear" json:"value_linear,omitempty" example:"8.78"`
	// Methode-3-Schätzung (Fortschreibung mit der durchschnittlichen relativen
	// Veränderung anderer Regionen derselben NUTS-2-Gruppe). Nur befüllt, wenn
	// is_interpolated = true und die Schätzung berechenbar war.
	ValuePeerTrend *float64 `db:"value_peer_trend" json:"value_peer_trend,omitempty" example:"8.50"`
	// Anzahl Peer-Regionen (gleiche NUTS-2-Gruppe), die zu value_peer_trend beigetragen haben.
	NPeerRegions *int `db:"n_peer_regions" json:"n_peer_regions,omitempty" example:"11"`
}

// LauTimeseriesPoint repräsentiert einen Zeitreihenpunkt aus
// transformed_data.mart_lau_timeseries_interpolated - das LAU-Pendant zu
// NutsTimeseriesPoint, wird von allen IsHarmonizedLau-Endpoints
// zurückgegeben, wenn ?fill_gaps=true gesetzt ist.
type LauTimeseriesPoint struct {
	GiscoID     string `db:"gisco_id"      json:"gisco_id"      example:"DE_11000000"`
	DatasetSlug string `db:"dataset_slug"  json:"dataset_slug"  example:"beschaeftigungsquote-gemeinden"`
	// LauVersion ist die LAU-Zielversion dieses Zeitreihenpunkts (2011-2024).
	LauVersion int     `db:"target_version" json:"lau_version"   example:"2021"`
	Year       int     `db:"year"          json:"year"          example:"2021"`
	Value      float64 `db:"value"         json:"value"         example:"58.3"`
	// true = Wert wurde imputiert (kein Originalwert vorhanden für dieses Jahr).
	IsInterpolated bool `db:"is_interpolated" json:"is_interpolated" example:"true"`
	// Methode-1-Schätzung (lineare Interpolation). Nur bei is_interpolated = true befüllt.
	ValueLinear *float64 `db:"value_linear" json:"value_linear,omitempty" example:"58.3"`
	// Methode-3-Schätzung (Fortschreibung mit der durchschnittlichen relativen
	// Veränderung anderer Gemeinden desselben Kreises). Nur befüllt, wenn
	// is_interpolated = true und die Schätzung berechenbar war.
	ValuePeerTrend *float64 `db:"value_peer_trend" json:"value_peer_trend,omitempty" example:"57.9"`
	// Anzahl Peer-Gemeinden (gleicher Kreis), die zu value_peer_trend beigetragen haben.
	NPeerRegions *int `db:"n_peer_regions" json:"n_peer_regions,omitempty" example:"8"`
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

// DatasetCatalogEintrag beschreibt Herkunft, Lizenz und Beschreibung eines Datensatzes
// Umfasst aktuell die meta.yaml-basierten Kreis-/Gemeinden-Datensätze; siehe
// mart_dataset_catalog.sql für die noch offene Migration der AfA-Arbeitslosenzahlen.
type DatasetCatalogEintrag struct {
	DatasetSlug string `json:"dataset_slug" db:"dataset_slug" example:"bevoelkerung"`
	// DatasetURI ist eine persistente, deterministisch aus DatasetSlug abgeleitete
	// Kennung (urn:soziodata:dataset:<slug>) - keine künstliche Zufalls-ID.
	DatasetURI      string  `json:"dataset_uri" db:"dataset_uri" example:"urn:soziodata:dataset:bevoelkerung"`
	Name            *string `json:"name,omitempty" db:"name" example:"Bevölkerung"`
	Description     *string `json:"description,omitempty" db:"description" example:"Zahl der Erwerbstätigen in 1000 Personen"`
	DescriptionLong *string `json:"description_long,omitempty" db:"description_long"`
	Unit            *string `json:"unit,omitempty" db:"unit" example:"Einwohner"`
	Provenance      *string `json:"provenance,omitempty" db:"provenance" example:"Laufende Raumbeobachtung des BBSR; Fortschreibung des Bevölkerungsstandes"`
	License         string  `json:"license" db:"license" example:"unbekannt"`
	LicenseURL      *string `json:"license_url,omitempty" db:"license_url"`
	Algorithm       *string `json:"algorithm,omitempty" db:"algorithm"`
	NumberType      *string `json:"number_type,omitempty" db:"number_type" example:"absolute"`
	GeodataLevel    *string `json:"geodata_level,omitempty" db:"geodata_level" example:"NUTS_3"`
	GeodataYear     *int    `json:"geodata_year,omitempty" db:"geodata_year" example:"2024"`
	MinYear         *int    `json:"min_year,omitempty" db:"min_year" example:"1995"`
	MaxYear         *int    `json:"max_year,omitempty" db:"max_year" example:"2023"`
	TotalRecords    *int64  `json:"total_records,omitempty" db:"total_records" example:"11600"`
	UpdatedAt       *string `json:"updated_at,omitempty" db:"updated_at"`
	// LicenseTrackedSince ist das Datum, seit dem die Lizenz dieses Datensatzes
	// per snap_dataset_license beobachtet wird; null falls noch kein dbt snapshot
	// gelaufen ist.
	LicenseTrackedSince *string `json:"license_tracked_since,omitempty" db:"license_tracked_since" example:"2026-09-11"`
	// LicenseBackfilled ist true, wenn der Datensatz Jahre vor LicenseTrackedSince
	// enthält - die aktuelle Lizenz wurde dann rückwirkend auf diese älteren Jahre
	// angewendet, ohne für sie unabhängig verifiziert zu sein.
	LicenseBackfilled bool `json:"license_backfilled" db:"license_backfilled" example:"true"`
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
		AuszubildendeConfig,
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
