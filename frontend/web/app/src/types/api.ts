// Response shapes of the Go API (backend/internal/models). Only the fields the
// frontend actually uses.

export interface ApiResponse<T> {
  success: boolean;
  data: T;
  error?: string;
  meta?: { total?: number; offset?: number };
}

// database/sql.Null* struct shape the Go API serializes some nullable fields as,
// instead of a plain scalar/`null`.
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
  // nuts3_id is the canonical LAU->NUTS-3 link (declared or spatially derived) -
  // more robust than nuts_3_code, which is often NULL.
  nuts3_id?: Nullable<string>;
  // 'declared' (from source data) or 'spatial_estimate' (spatially derived, not an
  // official assignment) - see backend lau_integrated.sql.
  nuts3_link_source?: Nullable<string>;
  geometry_wgs84?: GeoJSON.Geometry | null;
  has_valid_geometry?: boolean;
}

// Agenturbezirke are a separate area division of the Bundesagentur für Arbeit -
// not congruent with NUTS-3/LAU/ARS.
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

// An Agenturbezirk with geometry and its latest reporting month -
// for the map display (GET /agenturbezirke/latest).
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

// An available (year, month) combination for which Agenturbezirk Arbeitslosenzahlen
// exist (GET /agenturbezirke/periods) - for the year/month selection on the map,
// analogous to the year/version selection for NUTS/LAU.
export interface AgenturbezirkPeriod {
  year: number;
  month: number;
  observation_date: string;
}

// n:m mapping Agenturbezirk <-> NUTS-3 region with area/population share in both
// directions (GET /agenturbezirke/{ba_schl}/nuts3). Spatial estimate, not an
// official assignment - Agenturbezirke usually consist of one or more whole Kreise.
export interface AgenturNuts3Share {
  nuts3_id: string;
  region_name?: string | null;
  // Share of the Agenturbezirk area that falls on this NUTS-3 region (rows of an
  // Agenturbezirk sum approximately to 1.0).
  area_share_of_agentur: number;
  // Share of the NUTS-3 area that falls on this Agenturbezirk (rows of a NUTS-3
  // region sum approximately to 1.0).
  area_share_of_nuts3: number;
}

// Placement of an Agenturbezirk in an overlapping NUTS-1/2/3 region
// (GET /agenturbezirke/{ba_schl}/nuts-hierarchy). is_parent: smallest level that
// fully contains the Agenturbezirk (absent if it extends across Bundesland
// borders). Spatial estimate, not an official assignment.
export interface AgenturNutsMembership {
  nuts_id: string;
  nuts_level: number;
  region_name?: string | null;
  area_share_of_agentur: number;
  is_fully_contained: boolean;
  is_parent: boolean;
}

// Agenturbezirk overlapping a NUTS region
// (GET /nuts/{nuts_id}/agenturbezirke). is_fully_contained=false: only partial
// overlap.
export interface NutsAgenturbezirk {
  ba_schl: string;
  name?: string | null;
  area_share_of_agentur: number;
  is_fully_contained: boolean;
}

// One year of the aggregated socioeconomic indicators of a NUTS region
// (all levels 0-3, see mart_nuts0/1/2/3_summary) - GET /nuts/{nuts_id}?all=true.
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

// One year of LAU values (Gemeinden) aggregated up to NUTS-3 level -
// supplement/cross-check to NutsSocioeconomicPoint.bevoelkerung, see
// mart_nuts3_lau_aggregate. GET /nuts/{nuts_id}/lau-aggregate (NUTS-3 only).
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

// GET /versions: supported nuts_version / lau_version values (ascending) and
// the default the API uses when the parameter is omitted (the newest one).
export interface TargetVersionList {
  versions: number[];
  default: number;
}

export interface TargetVersionsResponse {
  nuts: TargetVersionList;
  lau: TargetVersionList;
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
  // true = value is an aggregation over several regions (see
  // NutsDatasetAggregatePoint), not a single observation read from one source -
  // TimeseriesChart.vue/DataTable.vue show the third status "berechnet" for it
  // instead of "beobachtet"/"interpoliert". Absent on all other time-series points
  // (NUTS-3/LAU Kreis-grain), where the previous two-way split applies.
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

// Time-series point of GET /nuts/{nuts_id}/dataset-aggregate/{slug} - the
// counterpart to NutsTimeseriesPoint for NUTS-0/1/2 (see the backend comment
// there): is_interpolated is always false here; instead there is
// source_region_count (number of Kreise that contributed to this year).
export interface NutsDatasetAggregatePoint extends TimeseriesPoint {
  nuts_id: string;
  dataset_slug: string;
  source_region_count: number;
}

// Time-series point of GET /agenturbezirke/{ba_schl}/dataset-aggregate/{slug} -
// the Agentur counterpart to NutsDatasetAggregatePoint (sum or
// population-weighted average, see mart_agentur_dataset_aggregate).
export interface AgenturDatasetAggregatePoint extends TimeseriesPoint {
  ba_schl: string;
  dataset_slug: string;
  source_region_count: number;
  // "sum" = sum of the Kreis values; "weighted_avg_population" =
  // population-weighted average (approximation); "weighted_avg_employees" =
  // average weighted by employed persons (values per employed person,
  // approximation); "weighted_avg_area" = area-weighted fallback when the
  // weights are missing (approximation).
  aggregation_method: "sum" | "weighted_avg_population" | "weighted_avg_employees" | "weighted_avg_area";
  // "median" is only an estimate (medians can't be aggregated).
  value_kind: "count" | "rate" | "mean" | "median";
}

// Row of GET /agenturbezirke/dataset-aggregate - a (dataset, year) combination
// for which aggregated Agenturbezirk values exist.
export interface AgenturDatasetAggregateYear {
  dataset_slug: string;
  year: number;
}

// Metadata catalog entry of GET /datasets (backend models.DatasetCatalogEintrag) -
// origin/license/description
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

// Raw row of a generic dataset endpoint (/<slug>), structure varies per dataset
// -> only the keys relevant to the fallback table renderer are known, all others
// are passed through generically.
export type GenericDatasetRow = Record<string, unknown>;

// GET /custom-regions/datasets (mart_dataset_aggregation_meta)
export interface CustomRegionDataset {
  dataset_slug: string;
  indicator: string;
  level: "lau" | "nuts3" | "agentur";
  measure_type: "sum" | "weighted_avg";
  kind: "count" | "rate" | "mean" | "median";
  finer_slug?: string | null;
  employee_slug?: string | null;
}

// Row of POST /custom-regions/dataset-aggregate
export interface CustomRegionAggregateRow {
  dataset_slug: string;
  indicator: string;
  level_used: "lau" | "nuts3" | "agentur";
  year: number;
  month?: number;
  value: number;
  value_kind: string;
  aggregation_method: string;
  source_region_count: number;
  coverage: number;
  // Set when the polygon matches an existing unit exactly.
  matched_unit?: string;
  is_crosswalked?: boolean;
  // Preferred weighting count of weighted_avg_employees indicators (also set on fallback rows).
  employee_slug?: string | null;
}
