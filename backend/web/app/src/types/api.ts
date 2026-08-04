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
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
}

export interface GeoRegionSummary {
  geo_id: string;
  geo_type: "nuts" | "lau";
  region_name: string;
  nuts_level?: number;
  vintage_year?: number;
  geometry_wgs84?: GeoJSON.Geometry | null;
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
