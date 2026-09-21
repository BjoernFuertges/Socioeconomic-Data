// Response-Shapes der Go-API (backend/internal/models). Nur die Felder, die
// das Frontend tatsächlich verwendet.

export interface ApiResponse<T> {
  success: boolean;
  data: T;
  error?: string;
  meta?: { total?: number; offset?: number };
}

// database/sql.Null*-Struct-Shape, die die Go-API für einige nullable Felder
// serialisiert, statt eines schlichten Skalars/`null`.
export interface SqlNull {
  Valid: boolean;
  Int64?: number;
  String?: string;
  Bool?: boolean;
  Float64?: number;
}

export type Nullable<T> = T | SqlNull | null | undefined;

export interface NutsRegionSummary {
  nuts_id: string;
  region_name: string;
  nuts_level: number;
  nuts_level_code?: string;
  ars?: Nullable<string>;
}

export interface NutsRegionMetadata {
  nuts_id: string;
  region_name: string;
  nuts_level: number;
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
}

export interface LauRegion {
  gisco_id: string;
  lau_name: string;
  year: number;
  country_code?: string;
  country?: Nullable<string>;
  population_count?: Nullable<number>;
  nuts_3_code?: Nullable<string>;
  // nuts3_id ist die kanonische LAU->NUTS-3-Verlinkung (deklariert oder
  // räumlich ermittelt) - robuster als nuts_3_code, das oft NULL ist.
  nuts3_id?: Nullable<string>;
  // 'declared' (aus Quelldaten) oder 'spatial_estimate' (räumlich
  // ermittelt, keine amtliche Zuordnung) - siehe backend lau_integrated.sql.
  nuts3_link_source?: Nullable<string>;
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
}

// Agenturbezirke sind eine eigenständige Gebietsgliederung der
// Bundesagentur für Arbeit - nicht deckungsgleich mit NUTS-3/LAU/ARS.
export interface AgenturbezirkSummary {
  ba_schl: string;
  name: string;
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
  valid_from?: string | null;
  valid_to?: string | null;
}

export interface AgenturbezirkArbeitslosigkeitPoint {
  year: number;
  month: number;
  observation_date: string;
  arbeitslose?: Nullable<number>;
  unterbeschaeftigung?: Nullable<number>;
}

// Ein Agenturbezirk mit Geometrie und dem jeweils neuesten Berichtsmonat -
// für die Kartendarstellung (GET /agenturbezirke/latest).
export interface AgenturbezirkLatestValue {
  ba_schl: string;
  name: string;
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
  year: number;
  month: number;
  observation_date: string;
  arbeitslose?: Nullable<number>;
  unterbeschaeftigung?: Nullable<number>;
}

// Eine verfügbare (Jahr, Monat)-Kombination, für die Agenturbezirk-
// Arbeitslosenzahlen vorliegen (GET /agenturbezirke/periods) - für die
// Jahr/Monat-Auswahl in der Karte, analog zur Jahres-/Versionsauswahl bei
// NUTS/LAU.
export interface AgenturbezirkPeriod {
  year: number;
  month: number;
  observation_date: string;
}

// n:m-Zuordnung Agenturbezirk <-> NUTS-3-Region mit Flächen-/
// Bevölkerungsanteil in beide Richtungen (GET /agenturbezirke/{ba_schl}/nuts3).
// Räumliche Schätzung, keine amtliche Zuordnung - Agenturbezirke bestehen
// i.d.R. aus einem oder mehreren ganzen Kreisen
export interface AgenturNuts3Share {
  nuts3_id: string;
  region_name?: string | null;
  // Anteil der Agenturbezirksfläche, der auf diese NUTS-3-Region entfällt
  // (Zeilen eines Agenturbezirks summieren sich näherungsweise auf 1.0).
  area_share_of_agentur: number;
  // Anteil der NUTS-3-Fläche, der auf diesen Agenturbezirk entfällt (Zeilen
  // einer NUTS-3-Region summieren sich näherungsweise auf 1.0).
  area_share_of_nuts3: number;
}

// Ein Jahr der aggregierten sozioökonomischen Indikatoren einer NUTS-Region
// (alle Level 0-3, siehe mart_nuts0/1/2/3_summary) - GET /nuts/{nuts_id}?all=true.
export interface NutsSocioeconomicPoint {
  year: number;
  bevoelkerung?: Nullable<number>;
  schuldnerquote_prozent?: Nullable<number>;
  arbeitslose?: Nullable<number>;
  yoy_change_percent?: Nullable<number>;
}

export interface NutsRegionTimeSeries {
  metadata?: NutsRegionMetadata;
  socioeconomic?: NutsSocioeconomicPoint[];
}

// Ein Jahr der auf NUTS-3-Ebene hochaggregierten LAU-Werte (Gemeinden) -
// Ergänzung/Cross-Check zu NutsSocioeconomicPoint.bevoelkerung, siehe
// mart_nuts3_lau_aggregate. GET /nuts/{nuts_id}/lau-aggregate (nur NUTS-3).
export interface NutsLauAggregatePoint {
  year: number;
  lau_population_sum?: Nullable<number>;
  lau_region_count: number;
  lau_region_count_with_population: number;
  lau_beschaeftigungsquote_weighted_avg?: Nullable<number>;
}

export interface NutsLauAggregateResponse {
  nuts_id: string;
  series: NutsLauAggregatePoint[];
}

export interface GeoRegionSummary {
  geo_id: string;
  geo_type: "nuts" | "lau" | "agentur";
  region_name: string;
  nuts_level?: number;
  vintage_year?: number;
  geometry_wgs84?: GeoJSON.Geometry | null;
}

export interface LauTimeseriesPoint extends TimeseriesPoint {
  gisco_id: string;
  dataset_slug: string;
  lau_version: number;
}

export interface DatasetCatalogEntry {
  route_slug: string;
  available_years: number[];
}

export interface DatasetCatalogResponse {
  datasets: DatasetCatalogEntry[];
}

export interface TimeseriesPoint {
  year: number;
  value: number;
  is_interpolated: boolean;
  // true = Wert ist eine Aggregation über mehrere Regionen (siehe
  // NutsDatasetAggregatePoint), keine an einer einzelnen Quelle abgelesene
  // Einzelbeobachtung - TimeseriesChart.vue/DataTable.vue zeigen dafür den
  // dritten Status "berechnet" statt "beobachtet"/"interpoliert". Fehlt bei
  // allen anderen Zeitreihenpunkten (NUTS-3/LAU-Kreis-grain), dort gilt die
  // bisherige Zweiteilung.
  is_computed?: boolean;
  value_linear?: number | null;
  value_peer_trend?: number | null;
  n_peer_regions?: number | null;
}

export interface NutsTimeseriesPoint extends TimeseriesPoint {
  nuts_id: string;
  dataset_slug: string;
  nuts_version: number;
}

// Zeitreihenpunkt von GET /nuts/{nuts_id}/dataset-aggregate/{slug} - das
// Pendant zu NutsTimeseriesPoint für NUTS-0/1/2 (siehe dortigen
// Backend-Kommentar): is_interpolated ist hier immer false, dafür gibt es
// source_region_count (Anzahl der Kreise, die zu diesem Jahr beigetragen haben).
export interface NutsDatasetAggregatePoint extends TimeseriesPoint {
  nuts_id: string;
  dataset_slug: string;
  source_region_count: number;
}

// Zeitreihenpunkt von GET /agenturbezirke/{ba_schl}/dataset-aggregate/{slug} -
// das Agentur-Pendant zu NutsDatasetAggregatePoint, aber flächenanteils-
// statt bevölkerungsgewichtet (siehe mart_agentur_dataset_aggregate).
export interface AgenturDatasetAggregatePoint extends TimeseriesPoint {
  ba_schl: string;
  dataset_slug: string;
  source_region_count: number;
}

// Zeile von GET /agenturbezirke/dataset-aggregate - eine (Datensatz, Jahr)-
// Kombination, für die aggregierte Agenturbezirk-Werte vorliegen.
export interface AgenturDatasetAggregateYear {
  dataset_slug: string;
  year: number;
}

// Metadaten-Katalogeintrag von GET /datasets (backend models.DatasetCatalogEintrag) -
// Herkunft/Lizenz/Beschreibung
export interface DatasetMetadataEntry {
  dataset_slug: string;
  dataset_uri: string;
  name?: string | null;
  description?: string | null;
  description_long?: string | null;
  unit?: string | null;
  provenance?: string | null;
  license: string;
  license_url?: string | null;
  algorithm?: string | null;
  number_type?: string | null;
  geodata_level?: string | null;
  geodata_year?: number | null;
  min_year?: number | null;
  max_year?: number | null;
  total_records?: number | null;
  updated_at?: string | null;
  license_tracked_since?: string | null;
  license_backfilled: boolean;
}

export interface InventoryEntry {
  dataset: string;
  year: number;
  geodata_level: string;
  records: number;
  distinct_geodata_nuts: number;
  distinct_geodata_lau: number;
  distinct_geodata_ars: number;
}

// Rohzeile eines generischen Datensatz-Endpoints (/<slug>), Struktur variiert
// je Datensatz -> nur die für den Fallback-Tabellen-Renderer relevanten Keys
// sind bekannt, alle weiteren werden generisch durchgereicht.
export type GenericDatasetRow = Record<string, unknown>;
