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
  // Räumliche Schätzung (größte Schnittfläche), keine amtliche Zuordnung -
  // siehe int_agentur_nuts3_spatial_link.sql.
  nuts3_id?: string | null;
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
  nuts3_id?: string | null;
  year: number;
  month: number;
  observation_date: string;
  arbeitslose?: Nullable<number>;
  unterbeschaeftigung?: Nullable<number>;
}

export interface GeoRegionSummary {
  geo_id: string;
  geo_type: "nuts" | "lau";
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
  value_linear?: number | null;
  value_peer_trend?: number | null;
  n_peer_regions?: number | null;
}

export interface NutsTimeseriesPoint extends TimeseriesPoint {
  nuts_id: string;
  dataset_slug: string;
  nuts_version: number;
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
