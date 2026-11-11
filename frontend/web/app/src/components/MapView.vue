<script setup lang="ts">
import { computed, onActivated, onMounted, onUnmounted, ref, watch } from "vue";
import type { RouteLocationRaw } from "vue-router";
import { useRoute, useRouter } from "vue-router";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { apiGet, unwrapNull } from "../api/client";
import AgenturAggregationNote from "./AgenturAggregationNote.vue";
import type {
  AgenturbezirkLatestValue,
  AgenturDatasetAggregatePoint,
  AgenturDatasetAggregateYear,
  AgenturbezirkPeriod,
  DatasetCatalogEntry,
  GeoRegionSummary,
  InventoryEntry,
  LauTimeseriesPoint,
  NutsDatasetAggregatePoint,
  NutsTimeseriesPoint,
  TimeseriesPoint,
} from "../types/api";
import { formatDatasetLabel, formatMonthLabel, formatNumber } from "../format";
import { AGENTUR_DATASET_SLUG } from "../constants";
import {
  MAP_ONLY_FILL_DARK,
  MAP_ONLY_FILL_LIGHT,
  NO_DATA_FILL_DARK,
  NO_DATA_FILL_LIGHT,
  sequentialColor,
  sequentialSteps,
} from "../mapColor";

const route = useRoute();
const router = useRouter();

// No external tile provider: instead of tiles from an API, Bundesland borders are drawn from our own historized NUTS geometry
// (NUTS-0 = national border, NUTS-1 = Bundesländer in Germany), no place names or other map labels
const BOUNDARY_COLOR_LIGHT = "#1a1a1a";
const BOUNDARY_COLOR_DARK = "#f5f5f5";

// Supported NUTS-3 target versions - must match supportedNutsVersions in
// backend/internal/handlers/kreis_generic.go. Corresponds to the vintage years
// present in stg_nuts_geometry; mart_nuts_version_crosswalk computes a direct
// (1-hop) mapping for every pair of them.
const NUTS_VERSIONS = [2003, 2006, 2010, 2013, 2016, 2021, 2024] as const;

// Supported LAU target versions - must match supportedLauVersions in
// backend/internal/handlers/lau_generic.go. Corresponds to the vintage years
// present in mart_lau_geometry_historized; mart_lau_version_crosswalk computes a
// mapping for every pair of them.
const LAU_VERSIONS = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024] as const;

type GeoType = "nuts" | "lau" | "agentur";

// NUTS levels for the level switcher of the interactive map layer
type NutsLevel = 0 | 1 | 2 | 3;
const NUTS_LEVELS: readonly NutsLevel[] = [0, 1, 2, 3];
const NUTS_LEVEL_LABELS: Record<NutsLevel, string> = {
  0: "Land",
  1: "Bundesland",
  2: "Regierungsbezirk",
  3: "Kreis",
};

// Unlike NUTS/LAU, Agenturbezirke have no time-series/interpolation mart, no
// dataset slug and no geometry versions (only one current boundary vintage) - instead
// of dataset+version one directly chooses between the two available metrics and
// year+month (monthly reporting data, see mart_agenturbezirk_sociodata.sql).
type AgenturMetric = "arbeitslose" | "unterbeschaeftigung";
const AGENTUR_METRIC_LABELS: Record<AgenturMetric, string> = {
  arbeitslose: "Arbeitslose",
  unterbeschaeftigung: "Unterbeschäftigung",
};
const selectedAgenturMetric = ref<AgenturMetric>("arbeitslose");
// Raw data from /agenturbezirke/period (geometry + both metrics for the chosen
// reporting month) - on a metric switch only valuesByRegion is re-derived from it,
// no new request needed.
const agenturLatestRaw = ref<AgenturbezirkLatestValue[]>([]);

// As an alternative to the BA monthly values (arbeitslose/unterbeschaeftigung), the
// Kreis datasets aggregated to Agenturbezirke can be colored too
// (GET /agenturbezirke/dataset-aggregate[/{slug}], area-share-weighted via
// mart_agentur_dataset_aggregate). "" = BA monthly values (metric + year/month),
// otherwise the slug of the aggregated dataset (then year only, no month).
// In both cases the geometry comes from /agenturbezirke/period.
const selectedAgenturDataset = ref<string>("");
const selectedAgenturAggYear = ref<number | null>(null);
// dataset slug -> available years (descending)
const agenturAggYears = ref<Map<string, number[]>>(new Map());
const agenturAggDatasetOptions = computed(() => Array.from(agenturAggYears.value.keys()).sort());
const agenturAggError = ref<string | null>(null);
const agenturAggYearOptions = computed(() => agenturAggYears.value.get(selectedAgenturDataset.value) ?? []);

// Available reporting months (from /agenturbezirke/periods, newest first) - fills
// the year/month selection for Agenturbezirke, analogous to the year/version
// selection for NUTS/LAU. Agenturbezirke only carry one current boundary vintage
// (see mart_agenturbezirk_sociodata.sql), so unlike the NUTS/LAU version there is
// no version selection here.
const agenturPeriodsError = ref<string | null>(null);
const agenturPeriods = ref<AgenturbezirkPeriod[]>([]);
const selectedAgenturYear = ref<number | null>(null);
const selectedAgenturMonth = ref<number | null>(null);
let agenturPeriodsLoaded = false;
// Prevents the initial population of selectedAgenturYear/-Month (below) from also
// triggering the watch([selectedAgenturYear, selectedAgenturMonth]) handler on top
// of the loadGeometries() call that is running anyway (and has just set these
// values via loadAgenturPeriods()) - otherwise the period request would run twice
// on the first switch to "agentur".
let suppressAgenturPeriodWatch = false;
// Analogous to suppressAgenturPeriodWatch: prevents initFromRoute() (onMounted/
// onActivated) from triggering a redundant second reload via
// watch(selectedGeoType)/watch(selectedNutsLevel) when the respective caller already
// triggers the reload explicitly because of a cross-link jump (see there) -
// otherwise the late, asynchronous watcher reload could overwrite a fitBounds() set
// earlier by focusOnRegion() with a buildLayer() over the full set of regions.
let suppressGeoTypeReload = false;
let suppressNutsLevelReload = false;

const agenturYearOptions = computed(() =>
  Array.from(new Set(agenturPeriods.value.map((p) => p.year))).sort((a, b) => b - a),
);
const agenturMonthOptions = computed(() =>
  agenturPeriods.value
    .filter((p) => p.year === selectedAgenturYear.value)
    .map((p) => p.month)
    .sort((a, b) => a - b),
);

async function loadAgenturPeriods() {
  if (agenturPeriodsLoaded) return;
  agenturPeriodsLoaded = true;
  agenturPeriodsError.value = null;
  try {
    const body = await apiGet<AgenturbezirkPeriod[]>("/agenturbezirke/periods");
    agenturPeriods.value = (body.data ?? []).slice().sort((a, b) => b.year - a.year || b.month - a.month);
    const latest = agenturPeriods.value[0];
    if (latest) {
      suppressAgenturPeriodWatch = true;
      selectedAgenturYear.value = latest.year;
      selectedAgenturMonth.value = latest.month;
    }
  } catch (err) {
    agenturPeriodsError.value = err instanceof Error ? err.message : String(err);
  }
}

// Minimal subset of GeoRegionSummary that buildLayer()/styleForFeature() actually
// need - GeoRegionSummary[] (NUTS/LAU, from /geo) is structurally compatible and
// thus still directly assignable.
type MapRegion = { geo_id: string; region_name: string; geometry_wgs84?: GeoJSON.Geometry | null };

const mapContainer = ref<HTMLDivElement | null>(null);
let map: L.Map | null = null;
let geoJsonLayer: L.GeoJSON | null = null;
let boundaryLayer: L.GeoJSON | null = null;
let lastBounds: L.LatLngBounds | null = null;

const isDark = ref(window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);

const geoError = ref<string | null>(null);
const regions = ref<MapRegion[]>([]);

const boundaryError = ref<string | null>(null);
const boundaries = ref<GeoRegionSummary[]>([]);

const inventoryError = ref<string | null>(null);
// dataset slug -> available years (descending, newest first), separate per geo
// level (/socioeconomic/inventory returns NUTS_3 and LAU entries separately, and
// /{slug}/timeseries expects nuts_version or lau_version depending on the level).
const datasetYearsNuts = ref<Map<string, number[]>>(new Map());
const datasetYearsLau = ref<Map<string, number[]>>(new Map());
// dataset slug -> available years for NUTS-0/1/2, loaded once (not reloaded when
// switching between Land/Bundesland/Regierungsbezirk). The catalog comes from the
// DE root instead of per level/region
const datasetYearsNuts012 = ref<Map<string, number[]>>(new Map());
// "" = no dataset selected -> map shows only the borders, no values.
const selectedDataset = ref<string>("");
const selectedYear = ref<number | null>(null);
const selectedNutsVersion = ref<number>(2024);
const selectedLauVersion = ref<number>(2024);
const selectedGeoType = ref<GeoType>("nuts");
const selectedNutsLevel = ref<NutsLevel>(3);

const valuesLoading = ref(false);
const valuesError = ref<string | null>(null);
// source_region_count is only set for aggregated Agentur datasets.
type MapValue = TimeseriesPoint & {
  source_region_count?: number;
  aggregation_method?: string;
  value_kind?: string;
};
const valuesByRegion = ref<Map<string, MapValue>>(new Map());

// NUTS-0/1/2 use a catalog of their own (datasetYearsNuts012) instead of the Kreis
// catalog (datasetYearsNuts) - the two sets differ, hence a separate case instead of just nuts/lau.
const datasetYears = computed(() => {
  if (selectedGeoType.value === "lau") return datasetYearsLau.value;
  if (usesNutsAggregate.value) return datasetYearsNuts012.value;
  return datasetYearsNuts.value;
});
const datasetOptions = computed(() => Array.from(datasetYears.value.keys()).sort());
const yearOptions = computed(() => datasetYears.value.get(selectedDataset.value) ?? []);

// NUTS-0/1/2 have no bulk endpoint across a whole level, only the single-region
// endpoint GET /nuts/{nuts_id}/dataset-aggregate/{slug} (see loadNuts012Values()) -
// controls which of the two load/display paths loadValues()/datasetYears above take.
const usesNutsAggregate = computed(() => selectedGeoType.value === "nuts" && selectedNutsLevel.value !== 3);

const valueDomain = computed<[number, number] | null>(() => {
  const values = Array.from(valuesByRegion.value.values()).map((p) => p.value);
  if (!values.length) return null;
  return [Math.min(...values), Math.max(...values)];
});

function colorForValue(value: number): string {
  const domain = valueDomain.value;
  if (!domain) return isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT;
  const [min, max] = domain;
  const t = max === min ? 0.5 : (value - min) / (max - min);
  return sequentialColor(t, isDark.value);
}

// Cross-link "Regionen -> Karte" (RegionBrowser.vue: viewOnMap()): region ID from
// ?region=, to fly to it on arrival (see focusOnRegion()) and - as long as nothing
// is hovered/clicked - show it in the sidebar box (see sidebarRegionId).
const highlightedRegionId = computed(() => (route.query.region as string | undefined) || null);
const HIGHLIGHT_COLOR = "#e8a33d";

// Instead of a Leaflet hover tooltip (see comment at buildLayer()), a sidebar box
// shows the data of the "active" region: live while hovering, otherwise the last
// clicked ("pinned") region, otherwise - right after a Regionen -> Karte cross-link
// jump - the region reached via ?region=. This keeps the box stable for careful
// reading/linking instead of vanishing as soon as the mouse moves away.
const hoveredRegionId = ref<string | null>(null);
const pinnedRegionId = ref<string | null>(null);
const sidebarRegionId = computed(() => hoveredRegionId.value ?? pinnedRegionId.value ?? highlightedRegionId.value);

function styleForFeature(feature?: GeoJSON.Feature): L.PathOptions {
  const geoId = feature?.properties?.geo_id as string | undefined;
  const point = geoId ? valuesByRegion.value.get(geoId) : undefined;
  const borderColor = isDark.value ? "#0d0d0d" : "#ffffff";
  const isHighlighted = geoId !== undefined && geoId === sidebarRegionId.value;

  if (selectedGeoType.value !== "agentur" && !selectedDataset.value) {
    // "Kein Datensatz (nur Karte)" or LAU view: own high-contrast style instead of the
    // subtle "no data" gray further below - that would be barely visible on the dark
    // map background if literally every region gets it (see MAP_ONLY_FILL_* in
    // mapColor.ts).
    // Reset dashArray explicitly: Leaflet's setStyle() only merges options
    // (setOptions copies existing keys via for...in) instead of replacing them -
    // without this line the dashed border of previously interpolated regions (from
    // an earlier dataset) would stick around.
    return {
      fillColor: isDark.value ? MAP_ONLY_FILL_DARK : MAP_ONLY_FILL_LIGHT,
      fillOpacity: 0.75,
      color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
      weight: isHighlighted ? 3 : 1,
      dashArray: undefined,
    };
  }

  if (!point) {
    return {
      fillColor: isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT,
      fillOpacity: 0.6,
      color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
      weight: isHighlighted ? 3 : 1,
      dashArray: undefined,
    };
  }
  return {
    fillColor: colorForValue(point.value),
    fillOpacity: 0.85,
    color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
    weight: isHighlighted ? 3 : point.is_interpolated ? 1 : 1.5,
    dashArray: point.is_interpolated ? "3 2" : undefined,
  };
}

function restyleLayer() {
  geoJsonLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature };
    l.setStyle(styleForFeature(l.feature));
  });
}

// "3D" lift effect for the active region (see sidebarRegionId): two stacked
// drop-shadows (tight+dark, wide+soft) make the area look like a raised card -
// illustrates both the hover preview and the jump via a cross-link. L.PathOptions
// has no CSS filter, hence direct access to the <path> element created by
// Leaflet's SVG renderer (_path is internal, but - like other Leaflet internals in
// this file, see the _tryAnimatedZoom comments - the only way to do it).
const LIFT_SHADOW_LIGHT = "drop-shadow(0 4px 4px rgba(0,0,0,0.3)) drop-shadow(0 10px 14px rgba(0,0,0,0.2))";
const LIFT_SHADOW_DARK = "drop-shadow(0 4px 4px rgba(0,0,0,0.6)) drop-shadow(0 10px 14px rgba(0,0,0,0.4))";

function applyLiftEffect() {
  const activeId = sidebarRegionId.value;
  geoJsonLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature; _path?: SVGPathElement };
    const isActive = activeId !== null && l.feature?.properties?.geo_id === activeId;
    if (l._path) l._path.style.filter = isActive ? (isDark.value ? LIFT_SHADOW_DARK : LIFT_SHADOW_LIGHT) : "";
    if (isActive) l.bringToFront();
  });
  boundaryLayer?.bringToFront();
}

// Data for the sidebar box - the same valuesByRegion map as for the coloring, so
// identical for NUTS/LAU (selected dataset) and Agentur (selected metric).
const sidebarRegion = computed(() => {
  const id = sidebarRegionId.value;
  if (!id) return null;
  const region = regions.value.find((r) => r.geo_id === id);
  if (!region) return null;
  return { id, name: region.region_name, point: valuesByRegion.value.get(id) ?? null };
});

const sidebarValueLabel = computed(() => {
  if (selectedGeoType.value !== "agentur") return "Wert";
  return selectedAgenturDataset.value
    ? formatDatasetLabel(selectedAgenturDataset.value)
    : AGENTUR_METRIC_LABELS[selectedAgenturMetric.value];
});

// Legend label for when no coloring exists yet because no dataset is selected.
const mapOnlyLabel = computed(() => (selectedGeoType.value === "lau" ? "LAU-Gemeinden" : "Kein Datensatz ausgewählt"));

// Cross-link "Karte -> Regionen" (reverse of RegionBrowser.vue: viewOnMap()) -
// jumps to the detail view of the region shown in the sidebar box, with the dataset
// currently selected on the map preselected (see RegionBrowser.vue:
// nutsSelectedSlugs/lauSelectedSlugs via ?dataset=). Agenturbezirke have no
// dataset picker (see there).
const sidebarLinkTarget = computed<RouteLocationRaw | null>(() => {
  const id = sidebarRegionId.value;
  if (!id) return null;
  if (selectedGeoType.value === "agentur") {
    // Analogous to dataset= below - carries over the metric currently selected on
    // the map, so RegionBrowser.vue (agenturSelectedMetrics) shows the same view
    // instead of falling back to its default selection (both metrics).
    return {
      name: "regionen",
      params: { regionType: "agentur", regionId: id },
      query: selectedAgenturDataset.value ? {} : { metrics: selectedAgenturMetric.value },
    };
  }
  return {
    name: "regionen",
    params: { regionType: selectedGeoType.value, regionId: id },
    query: selectedDataset.value ? { dataset: selectedDataset.value } : {},
  };
});

function styleForBoundary(feature?: GeoJSON.Feature): L.PathOptions {
  const level = feature?.properties?.nuts_level as number | undefined;
  return {
    fill: false,
    color: isDark.value ? BOUNDARY_COLOR_DARK : BOUNDARY_COLOR_LIGHT,
    weight: level === 0 ? 2.5 : 1.25,
    opacity: level === 0 ? 0.9 : 0.6,
  };
}

function restyleBoundaryLayer() {
  boundaryLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature };
    l.setStyle(styleForBoundary(l.feature));
  });
}

// Load country (NUTS-0) and Bundesland (NUTS-1) borders - independent of the
// selected level (NUTS/LAU), so the map offers geographic orientation without
// place names in the LAU view too.
async function loadBoundaries() {
  boundaryError.value = null;
  try {
    const [countries, states] = await Promise.all(
      [0, 1].map((level) =>
        apiGet<GeoRegionSummary[]>("/geo", {
          geo_type: "nuts",
          level,
          nuts_parent_id: "DE",
          include_geometry: true,
          geometry_strategy: "exact",
          geometry_year: selectedNutsVersion.value,
          limit: "null",
        }),
      ),
    );
    boundaries.value = [...(countries.data ?? []), ...(states.data ?? [])].filter((r) => r.geometry_wgs84);
  } catch (err) {
    boundaryError.value = err instanceof Error ? err.message : String(err);
  }
}

function buildBoundaryLayer() {
  if (!map) return;
  if (boundaryLayer) {
    map.removeLayer(boundaryLayer);
    boundaryLayer = null;
  }
  if (!boundaries.value.length) return;
  const featureCollection: GeoJSON.FeatureCollection = {
    type: "FeatureCollection",
    features: boundaries.value.map((r) => ({
      type: "Feature",
      properties: { nuts_level: r.nuts_level },
      geometry: r.geometry_wgs84 as GeoJSON.Geometry,
    })),
  };
  boundaryLayer = L.geoJSON(featureCollection, {
    style: (feature) => styleForBoundary(feature),
    interactive: false,
  }).addTo(map);
  // Must sit above the (filled) region layer, otherwise the border lines vanish
  // under its fill color.
  boundaryLayer.bringToFront();
}

async function loadGeometries() {
  geoError.value = null;
  try {
    if (selectedGeoType.value === "agentur") {
      // The year/month selection is only possible after the available reporting
      // months have loaded (see loadAgenturPeriods) - so wait here before querying
      // the chosen (or default: latest) point in time.
      await loadAgenturPeriods();
      if (selectedAgenturYear.value === null || selectedAgenturMonth.value === null) {
        agenturLatestRaw.value = [];
        regions.value = [];
        return;
      }
      // One request delivers geometry AND the value of the chosen reporting month in
      // one step (unlike the separate /geo + /{dataset}/timeseries path for NUTS/LAU).
      const body = await apiGet<AgenturbezirkLatestValue[]>("/agenturbezirke/period", {
        year: selectedAgenturYear.value,
        month: selectedAgenturMonth.value,
      });
      agenturLatestRaw.value = (body.data ?? []).filter((a) => a.geometry_wgs84);
      regions.value = agenturLatestRaw.value.map((a) => ({
        geo_id: a.ba_schl,
        region_name: a.name,
        geometry_wgs84: a.geometry_wgs84,
      }));
      return;
    }

    if (selectedGeoType.value === "lau") {
      // geometry_year pins the boundaries of the chosen LAU version -
      // /geo?geo_type=lau interprets geometry_year as a year filter on
      // lau_integrated (see backend/internal/handlers/geo.go).
      // country_code restricts to Germany.
      const body = await apiGet<GeoRegionSummary[]>("/geo", {
        geo_type: "lau",
        country_code: "DE",
        include_geometry: true,
        geometry_year: selectedLauVersion.value,
        limit: "null",
      });
      regions.value = (body.data ?? []).filter((r) => r.geometry_wgs84);
      return;
    }

    // Per the API, country_code is only evaluated for geo_type=lau; for NUTS,
    // nuts_parent_id (prefix filter) restricts to Germany.
    // geometry_strategy=exact+geometry_year pins the boundaries of the chosen NUTS
    // version (see mart_nuts_geometry_historized) - so /geo returns exactly the
    // regions of the chosen level that were valid in that version (level selectable
    // via the level switcher).
    const body = await apiGet<GeoRegionSummary[]>("/geo", {
      geo_type: "nuts",
      level: selectedNutsLevel.value,
      nuts_parent_id: "DE",
      include_geometry: true,
      geometry_strategy: "exact",
      geometry_year: selectedNutsVersion.value,
      limit: "null",
    });
    regions.value = (body.data ?? []).filter((r) => r.geometry_wgs84);
  } catch (err) {
    geoError.value = err instanceof Error ? err.message : String(err);
  }
}

// Loads the dataset overview for a geo level and preselects the first available
// dataset (if nothing is selected yet or the previous selection doesn't exist in
// this level).
async function loadInventoryFor(geoType: GeoType) {
  const geodataLevel = geoType === "lau" ? "LAU" : "NUTS_3";
  const target = geoType === "lau" ? datasetYearsLau : datasetYearsNuts;
  const body = await apiGet<InventoryEntry[]>("/socioeconomic/inventory", { geodata_level: geodataLevel });
  const byDataset = new Map<string, Set<number>>();
  for (const entry of body.data ?? []) {
    if (!byDataset.has(entry.dataset)) byDataset.set(entry.dataset, new Set());
    byDataset.get(entry.dataset)!.add(entry.year);
  }
  const sorted = new Map<string, number[]>();
  for (const [slug, years] of byDataset) {
    sorted.set(slug, Array.from(years).sort((a, b) => b - a));
  }
  target.value = sorted;
}

async function loadInventory() {
  try {
    await Promise.all([loadInventoryFor("nuts"), loadInventoryFor("lau")]);
  } catch (err) {
    inventoryError.value = err instanceof Error ? err.message : String(err);
  }
}

// After all catalogs (Kreis/LAU/NUTS-0-2) have loaded: replace an unknown dataset
// with the first available one and a missing/invalid year (e.g. deep link with
// ?dataset= but without ?year=) with the latest available - otherwise the map
// would stay uncolored despite a dataset.
function reconcileDatasetSelection() {
  if (selectedGeoType.value === "agentur") return;
  if (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value)) {
    const firstDataset = datasetOptions.value[0];
    selectedDataset.value = firstDataset ?? "";
    selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
    return;
  }
  const years = datasetYears.value.get(selectedDataset.value) ?? [];
  if (selectedYear.value === null || !years.includes(selectedYear.value)) {
    selectedYear.value = years[0] ?? null;
  }
}

// Dataset catalog for NUTS-0/1/2 - see the datasetYearsNuts012 comment above
// (loaded once from the DE root, not reloaded per level).
async function loadNuts012Inventory() {
  try {
    const body = await apiGet<{ datasets: DatasetCatalogEntry[] }>("/nuts/DE/datasets");
    const sorted = new Map<string, number[]>();
    for (const ds of body.data?.datasets ?? []) {
      sorted.set(ds.route_slug, [...ds.available_years].sort((a, b) => b - a));
    }
    datasetYearsNuts012.value = sorted;
  } catch (err) {
    inventoryError.value = err instanceof Error ? err.message : String(err);
  }
}

// Derives valuesByRegion from the cached /agenturbezirke/latest response and the
// chosen metric - no network request, so a mere metric switch reacts instantly.
function applyAgenturValues() {
  valuesByRegion.value = new Map(
    agenturLatestRaw.value.flatMap((a) => {
      const raw = selectedAgenturMetric.value === "arbeitslose" ? a.arbeitslose : a.unterbeschaeftigung;
      const value = unwrapNull(raw);
      if (value === null) return [];
      // is_interpolated is always false for Agenturbezirke: unlike NUTS/LAU there
      // is no gap-filling/interpolation mart yet.
      const point: TimeseriesPoint = { year: a.year, value, is_interpolated: false };
      return [[a.ba_schl, point]] as [string, TimeseriesPoint][];
    }),
  );
  restyleLayer();
}

// Available aggregated datasets/years for Agenturbezirke; preselects a valid year
// (if a dataset was given via the route).
async function loadAgenturAggInventory() {
  const body = await apiGet<AgenturDatasetAggregateYear[]>("/agenturbezirke/dataset-aggregate");
  const byDataset = new Map<string, number[]>();
  for (const row of body.data ?? []) {
    if (!byDataset.has(row.dataset_slug)) byDataset.set(row.dataset_slug, []);
    byDataset.get(row.dataset_slug)!.push(row.year);
  }
  for (const years of byDataset.values()) years.sort((a, b) => b - a);
  agenturAggYears.value = byDataset;
  ensureAgenturAggSelection();
}

// Discards an unknown dataset and replaces a missing/invalid year with the latest
// available one.
function ensureAgenturAggSelection() {
  const slug = selectedAgenturDataset.value;
  if (!slug) return;
  const years = agenturAggYears.value.get(slug);
  if (!years) {
    if (agenturAggYears.value.size > 0) selectedAgenturDataset.value = "";
    return;
  }
  if (selectedAgenturAggYear.value === null || !years.includes(selectedAgenturAggYear.value)) {
    selectedAgenturAggYear.value = years[0] ?? null;
  }
}

async function loadAgenturAggValues() {
  const slug = selectedAgenturDataset.value;
  const year = selectedAgenturAggYear.value;
  if (!slug || year === null) {
    valuesByRegion.value = new Map();
    restyleLayer();
    return;
  }
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    const body = await apiGet<AgenturDatasetAggregatePoint[]>(`/agenturbezirke/dataset-aggregate/${slug}`, { year });
    // Response may have been superseded by quick selection changes.
    if (slug !== selectedAgenturDataset.value || year !== selectedAgenturAggYear.value) return;
    valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.ba_schl, p]));
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

// NUTS-0/1/2: no bulk endpoint across a whole level (unlike /{slug}/timeseries for
// Kreise/LAU) - instead query once per visible region in parallel (manageable at
// Land/Bundesland/Regierungsbezirk: 1/16/~30 regions). Each response already
// contains all years of that region (the endpoint has no year filter); a mere year
// switch simply re-queries anyway rather than caching the raw data separately -
// negligible at this region count.
async function loadNuts012Values() {
  const slug = selectedDataset.value;
  if (!slug) {
    valuesError.value = null;
    if (valuesByRegion.value.size > 0) {
      valuesByRegion.value = new Map();
      restyleLayer();
    }
    return;
  }
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    const results = await Promise.all(
      regions.value.map(async (r) => {
        const body = await apiGet<NutsDatasetAggregatePoint[]>(`/nuts/${r.geo_id}/dataset-aggregate/${slug}`);
        return [r.geo_id, body.data ?? []] as const;
      }),
    );
    // Response may have been superseded by a quick dataset/level switch.
    if (slug !== selectedDataset.value) return;
    const year = selectedYear.value;
    valuesByRegion.value = new Map(
      results.flatMap(([geoId, points]) => {
        const point = year === null ? points[0] : points.find((p) => p.year === year);
        return point ? ([[geoId, point]] as [string, MapValue][]) : [];
      }),
    );
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

// Drops responses of superseded requests (fast year/dataset changes).
let valuesSeq = 0;

async function loadValues() {
  if (selectedGeoType.value === "agentur") {
    if (selectedAgenturDataset.value) await loadAgenturAggValues();
    else applyAgenturValues();
    return;
  }
  if (usesNutsAggregate.value) {
    await loadNuts012Values();
    return;
  }
  if (!selectedDataset.value || selectedYear.value === null) {
    // No dataset selected -> map stays uncolored; discard any values left over
    // from the previously selected dataset.
    valuesError.value = null;
    if (valuesByRegion.value.size > 0) {
      valuesByRegion.value = new Map();
      restyleLayer();
    }
    return;
  }
  const seq = ++valuesSeq;
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    if (selectedGeoType.value === "lau") {
      const body = await apiGet<LauTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        lau_version: selectedLauVersion.value,
        limit: "null",
      });
      if (seq !== valuesSeq) return;
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.gisco_id, p]));
    } else {
      const body = await apiGet<NutsTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        nuts_version: selectedNutsVersion.value,
        limit: "null",
      });
      if (seq !== valuesSeq) return;
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.nuts_id, p]));
    }
    restyleLayer();
  } catch (err) {
    if (seq === valuesSeq) valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    if (seq === valuesSeq) valuesLoading.value = false;
  }
}

function buildLayer() {
  if (!map) return;
  // Always remove the old layer first, even if the new region list is empty -
  // otherwise a version/level switch without matches would wrongly leave the
  // previous map layer (borders + coloring) in place.
  if (geoJsonLayer) {
    map.removeLayer(geoJsonLayer);
    geoJsonLayer = null;
  }
  if (!regions.value.length) return;
  const featureCollection: GeoJSON.FeatureCollection = {
    type: "FeatureCollection",
    features: regions.value.map((r) => ({
      type: "Feature",
      properties: { geo_id: r.geo_id, region_name: r.region_name },
      geometry: r.geometry_wgs84 as GeoJSON.Geometry,
    })),
  };

  geoJsonLayer = L.geoJSON(featureCollection, {
    style: (feature) => styleForFeature(feature),
    // Instead of a Leaflet tooltip (see sidebarRegionId comment above), hover/click
    // now only drive the sidebar box; a click "pins" the region so the box stays
    // stable while reading/linking instead of vanishing when leaving the area.
    onEachFeature: (feature, layer) => {
      const geoId = feature.properties?.geo_id as string | undefined;
      layer.on({
        mouseover: () => {
          hoveredRegionId.value = geoId ?? null;
        },
        mouseout: () => {
          hoveredRegionId.value = null;
        },
        click: () => {
          pinnedRegionId.value = geoId ?? null;
        },
      });
    },
  }).addTo(map);
  applyLiftEffect();

  const bounds = geoJsonLayer.getBounds();
  if (bounds.isValid()) {
    lastBounds = bounds;
    // The map tab may still be hidden via v-show (container size 0x0) - fitBounds()
    // would then compute with a zoom level that permanently corrupts Leaflet's
    // internal map projection (SVG layers get stuck at "M0 0", later
    // invalidateSize() calls throw). The ResizeObserver below catches up on the
    // fitBounds once the container becomes visible.
    const size = map.getSize();
    if (size.x > 0 && size.y > 0) {
      // animate: false - see comment in focusOnRegion(): if an animated zoom
      // transition from a previous fitBounds() is still running (e.g. quick
      // level/version switches in a row), Leaflet (_tryAnimatedZoom:
      // `if (this._animatingZoom) return true`) would silently drop this call
      // without ever applying the new view.
      map.fitBounds(bounds, { padding: [8, 8], animate: false });
    }
  }
  boundaryLayer?.bringToFront();
}

let darkModeQuery: MediaQueryList | null = null;
function handleDarkModeChange(evt: MediaQueryListEvent) {
  isDark.value = evt.matches;
}

// The map tab is shown/hidden via v-show (not v-if) so its state survives tab
// switches - so the container div may still have size 0 (display:none) at Leaflet
// init. A ResizeObserver triggers invalidateSize() as soon as the container gets
// visible dimensions (e.g. on the first switch to the map tab).
let resizeObserver: ResizeObserver | null = null;

// Copies level/dataset/year/version/metric from the route into the local refs when
// a query parameter is present (otherwise the current value is left untouched) -
// for the initial deep link (onMounted) AND for a repeated cross-link jump to the
// already mounted map (via KeepAlive in App.vue, onActivated). The actual reload
// reactions run through the existing watch(selected...) handlers below as soon as a
// ref actually changes as a result.
function initFromRoute() {
  const q = route.query;
  const geo = q.geo as GeoType | undefined;
  if (geo === "nuts" || geo === "lau" || geo === "agentur") selectedGeoType.value = geo;

  if (selectedGeoType.value === "agentur") {
    const metric = q.metric as AgenturMetric | undefined;
    if (metric === "arbeitslose" || metric === "unterbeschaeftigung") selectedAgenturMetric.value = metric;
    if (typeof q.dataset === "string") selectedAgenturDataset.value = q.dataset;
    if (q.year) {
      const y = Number(q.year);
      // For an aggregated dataset ?year= denotes the data year, otherwise the reporting year.
      if (!Number.isNaN(y)) {
        if (typeof q.dataset === "string" && q.dataset) selectedAgenturAggYear.value = y;
        else selectedAgenturYear.value = y;
      }
    }
    if (q.month) {
      const m = Number(q.month);
      if (!Number.isNaN(m)) selectedAgenturMonth.value = m;
    }
  } else {
    if (typeof q.dataset === "string") selectedDataset.value = q.dataset;
    if (q.year) {
      const y = Number(q.year);
      if (!Number.isNaN(y)) selectedYear.value = y;
    }
  }
  if (q.nutsVersion) {
    const v = Number(q.nutsVersion);
    if ((NUTS_VERSIONS as readonly number[]).includes(v)) selectedNutsVersion.value = v;
  }
  if (q.nutsLevel) {
    const l = Number(q.nutsLevel);
    if ((NUTS_LEVELS as readonly number[]).includes(l)) selectedNutsLevel.value = l as NutsLevel;
  }
  if (q.lauVersion) {
    const v = Number(q.lauVersion);
    if ((LAU_VERSIONS as readonly number[]).includes(v)) selectedLauVersion.value = v;
  }
}

// Centers the map on a region reached via ?region= (cross-link "Regionen -> Karte")
// and opens its tooltip - the highlighting itself is handled by
// styleForFeature()/highlightedRegionId further up.
function focusOnRegion(id: string) {
  if (!map || !geoJsonLayer) return;
  // Container may still be invisible/size 0x0 (e.g. on the very first KeepAlive
  // activated right after mount, before Vue has painted the layout) - a fitBounds()
  // on it would permanently corrupt Leaflet's internal projection (see
  // buildLayer()). The ResizeObserver catches up on the focus once the container
  // has a real size.
  const size = map.getSize();
  if (size.x === 0 || size.y === 0) return;
  geoJsonLayer.eachLayer((layer) => {
    const l = layer as L.Polygon & { feature?: GeoJSON.Feature };
    if (l.feature?.properties?.geo_id !== id) return;
    const bounds = l.getBounds?.();
    if (bounds && bounds.isValid()) {
      // Update lastBounds here too, otherwise the ResizeObserver above (which
      // re-fits to lastBounds on a later size change) would reset the focus on this
      // region back to the full map extent.
      lastBounds = bounds;
      // animate: false - on a big zoom jump (e.g. from the Germany overview to a
      // single Kreis) Leaflet's animated zoom (_tryAnimatedZoom) occasionally gets
      // stuck here (zoom/center never finally set); an immediate, unanimated jump
      // suits this programmatic focusing better than an animation anyway.
      map!.fitBounds(bounds, { padding: [40, 40], maxZoom: 10, animate: false });
    }
  });
}

watch(highlightedRegionId, (id) => {
  if (id) focusOnRegion(id);
});

// Hover/pin/cross-link focus change which region counts as "active" (see
// sidebarRegionId) - reapply border highlight and 3D lift effect without changing
// the map view (unlike focusOnRegion() above) - mere hovering must not zoom away.
watch(sidebarRegionId, () => {
  restyleLayer();
  applyLiftEffect();
});

// Mirrors the current selection state back into the route (router.replace, so
// pure selection changes create no history entries) - makes the map view
// deep-linkable/shareable, see F12.1.
const currentQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = { geo: selectedGeoType.value };
  if (selectedGeoType.value === "agentur") {
    if (selectedAgenturDataset.value) {
      q.dataset = selectedAgenturDataset.value;
      if (selectedAgenturAggYear.value !== null) q.year = String(selectedAgenturAggYear.value);
    } else {
      q.metric = selectedAgenturMetric.value;
      if (selectedAgenturYear.value !== null) q.year = String(selectedAgenturYear.value);
      if (selectedAgenturMonth.value !== null) q.month = String(selectedAgenturMonth.value);
    }
  } else {
    if (selectedDataset.value) q.dataset = selectedDataset.value;
    if (selectedYear.value !== null) q.year = String(selectedYear.value);
    if (selectedGeoType.value === "nuts") {
      q.nutsVersion = String(selectedNutsVersion.value);
      q.nutsLevel = String(selectedNutsLevel.value);
    }
    if (selectedGeoType.value === "lau") q.lauVersion = String(selectedLauVersion.value);
  }
  if (highlightedRegionId.value) q.region = highlightedRegionId.value;
  return q;
});

watch(currentQuery, (q) => {
  // App.vue keeps MapView alive via <KeepAlive> even outside the "karte" route, so
  // this watcher keeps running e.g. while the user is on "regionen"/"katalog".
  // route (useRoute()) is the globally current route object, not "the route this
  // component was last active with" - without this check a query key that happens
  // to collide with another view would navigate back here (see the same bug in
  // RegionBrowser.vue: applyRouteSelection()).
  if (route.name !== "karte") return;
  router.replace({ name: "karte", query: q });
});

onMounted(async () => {
  const prevGeoType = selectedGeoType.value;
  const prevNutsLevel = selectedNutsLevel.value;
  initFromRoute();
  // A deep link (?geo=/?nutsLevel=) can change selectedGeoType/selectedNutsLevel from
  // their default - the reload this requires is handled by the explicit
  // Promise.all(...)/buildLayer() block below, so skip the corresponding watch(...)
  // handlers once (see suppressGeoTypeReload/suppressNutsLevelReload comment above).
  if (selectedGeoType.value !== prevGeoType) suppressGeoTypeReload = true;
  if (selectedGeoType.value === "nuts" && selectedNutsLevel.value !== prevNutsLevel) suppressNutsLevelReload = true;

  if (mapContainer.value) {
    map = L.map(mapContainer.value, { center: [51.2, 10.4], zoom: 6, scrollWheelZoom: false });

    resizeObserver = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      if (width > 0 && height > 0) {
        map?.invalidateSize();
        if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
        else if (lastBounds) map?.fitBounds(lastBounds, { padding: [8, 8], animate: false });
      }
    });
    resizeObserver.observe(mapContainer.value);
  }

  darkModeQuery = window.matchMedia?.("(prefers-color-scheme: dark)") ?? null;
  darkModeQuery?.addEventListener("change", handleDarkModeChange);

  await Promise.all([loadGeometries(), loadBoundaries()]);
  buildLayer();
  buildBoundaryLayer();
  await Promise.all([loadInventory(), loadNuts012Inventory()]);
  reconcileDatasetSelection();
  try {
    await loadAgenturAggInventory();
  } catch (err) {
    agenturAggError.value = err instanceof Error ? err.message : String(err);
  }
  // Landed directly via deep link on an aggregated Agentur/NUTS-0-1-2 dataset: load
  // the values only now (after year validation or after the geometry load that
  // loadNuts012Values() needs for the region list) - the reactive
  // watch([selectedDataset, selectedYear], loadValues) below fires too early here
  // (or not at all for a pure dataset deep link without ?year=, see the comment there).
  if ((selectedGeoType.value === "agentur" && selectedAgenturDataset.value) || usesNutsAggregate.value) {
    await loadValues();
  }
  if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
});

onUnmounted(() => {
  darkModeQuery?.removeEventListener("change", handleDarkModeChange);
  resizeObserver?.disconnect();
  map?.remove();
});

// App.vue keeps the map view mounted via <KeepAlive> - another jump here (e.g. via
// cross-link from Regionen) doesn't remount the component but only reactivates it.
// initFromRoute() picks up new query parameters from the cross-link; then the map
// is re-measured via invalidateSize() (the container may have had size 0 while
// deactivated) and focused on a region passed along.
onActivated(async () => {
  const prevGeoType = selectedGeoType.value;
  const prevNutsLevel = selectedNutsLevel.value;
  initFromRoute();
  // On the very first activation (right at the initial mount, before the first
  // layout) the container may still have size 0x0 - then do nothing here and let
  // the ResizeObserver (see above) take over, instead of permanently corrupting
  // Leaflet's internal projection with invalidateSize()/fitBounds() on a 0x0 size
  // (see buildLayer() comment).
  const size = map?.getSize();
  if (!map || !size || size.x === 0 || size.y === 0) return;
  map.invalidateSize();
  // A cross-link jump (e.g. Regionen -> Karte to a Bundesland) may have changed
  // selectedGeoType/selectedNutsLevel - reload geometry/values for it HERE (and let
  // the corresponding watch(...) handlers skip once, see
  // suppressGeoTypeReload/suppressNutsLevelReload) before focusOnRegion() runs
  // below. Otherwise focusOnRegion() would search the old geoJsonLayer (reloaded
  // asynchronously only later) and not find the region.
  const typeChanged = selectedGeoType.value !== prevGeoType;
  const levelChanged = selectedGeoType.value === "nuts" && selectedNutsLevel.value !== prevNutsLevel;
  if (typeChanged || levelChanged) {
    if (typeChanged) suppressGeoTypeReload = true;
    if (levelChanged) suppressNutsLevelReload = true;
    await loadGeometries();
    buildLayer();
    await loadValues();
  }
  if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
  else if (lastBounds) map.fitBounds(lastBounds, { padding: [8, 8], animate: false });
});

watch([selectedDataset, selectedYear], loadValues);
watch(selectedAgenturMetric, () => {
  if (selectedGeoType.value === "agentur" && !selectedAgenturDataset.value) applyAgenturValues();
});
// Dataset switch (BA monthly values <-> aggregated Kreis dataset): ensure a valid
// year and reload values; geometry stays unchanged.
watch(selectedAgenturDataset, () => {
  ensureAgenturAggSelection();
  if (selectedGeoType.value === "agentur") loadValues();
});
watch(selectedAgenturAggYear, () => {
  if (selectedGeoType.value === "agentur" && selectedAgenturDataset.value) loadValues();
});
// If the chosen month doesn't exist in the new year (different years may have
// covered different reporting months), jump to the latest available month of the
// new year.
watch(selectedAgenturYear, () => {
  const months = agenturMonthOptions.value;
  if (selectedAgenturMonth.value !== null && !months.includes(selectedAgenturMonth.value)) {
    selectedAgenturMonth.value = months[months.length - 1] ?? null;
  }
});
// On reporting-month change: load new geometry+values for this point in time
// (analogous to the version switch for NUTS/LAU).
watch([selectedAgenturYear, selectedAgenturMonth], async () => {
  if (suppressAgenturPeriodWatch) {
    suppressAgenturPeriodWatch = false;
    return;
  }
  if (selectedGeoType.value !== "agentur") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
watch(isDark, () => {
  restyleLayer();
  restyleBoundaryLayer();
  applyLiftEffect();
});
// On a NUTS version switch both the boundaries (different vintage period,
// possibly different Kreis IDs) and the values (different target_version in the
// harmonized mart) must be reloaded. The country/Bundesland borders belong to the
// same version and are updated too, independent of the chosen level (NUTS/LAU).
watch(selectedNutsVersion, async () => {
  await loadBoundaries();
  buildBoundaryLayer();
  if (selectedGeoType.value !== "nuts") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// Same for the LAU version (different target_version in the LAU-harmonized mart).
watch(selectedLauVersion, async () => {
  if (selectedGeoType.value !== "lau") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// On a level switch: if the previously chosen dataset doesn't exist in the new
// level (NUTS-3 and LAU datasets are separate sets), switch to the first available
// dataset of the new level (or "" without dataset selection in this level).
watch(selectedGeoType, async () => {
  if (suppressGeoTypeReload) {
    suppressGeoTypeReload = false;
    return;
  }
  if (selectedGeoType.value !== "agentur" && (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value))) {
    const firstDataset = datasetOptions.value[0];
    selectedDataset.value = firstDataset ?? "";
    selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
  }
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// On a NUTS level switch (Land/Bundesland/Regierungsbezirk/Kreis): load new
// geometry (different regions per level) and - analogous to the level switch
// above (Kreis catalog vs. NUTS-0/1/2 catalog are different sets, see
// datasetYears) - switch a no longer valid dataset selection.
watch(selectedNutsLevel, async () => {
  if (suppressNutsLevelReload) {
    suppressNutsLevelReload = false;
    return;
  }
  if (selectedGeoType.value !== "nuts") return;
  if (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value)) {
    const firstDataset = datasetOptions.value[0];
    selectedDataset.value = firstDataset ?? "";
    selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
  }
  await loadGeometries();
  buildLayer();
  await loadValues();
});
</script>

<template>
  <div class="map-view">
    <p v-if="geoError" class="error-text">Geometrien konnten nicht geladen werden: {{ geoError }}</p>
    <p v-if="boundaryError" class="error-text">Staats-/Bundeslandgrenzen konnten nicht geladen werden: {{ boundaryError }}</p>
    <p v-if="inventoryError" class="error-text">Datensatz-Übersicht konnte nicht geladen werden: {{ inventoryError }}</p>
    <p v-if="agenturAggError" class="error-text">Aggregierte Agenturbezirk-Datensätze konnten nicht geladen werden: {{ agenturAggError }}</p>
    <p v-if="agenturPeriodsError" class="error-text">Berichtsmonate der Agenturbezirke konnten nicht geladen werden: {{ agenturPeriodsError }}</p>

    <div class="dataset-picker">
      <label for="map-geo-type-select">Ebene</label>
      <select id="map-geo-type-select" v-model="selectedGeoType">
        <option value="nuts">NUTS (Land–Kreis)</option>
        <option value="lau">LAU (Gemeinden)</option>
        <option value="agentur">Agenturbezirke (BA)</option>
      </select>

      <template v-if="selectedGeoType === 'nuts'">
        <label for="map-nuts-level-select">NUTS-Ebene</label>
        <select id="map-nuts-level-select" v-model.number="selectedNutsLevel">
          <option v-for="level in NUTS_LEVELS" :key="level" :value="level">{{ NUTS_LEVEL_LABELS[level] }}</option>
        </select>
      </template>

      <template v-if="selectedGeoType === 'agentur'">
        <label for="map-agentur-dataset-select">Datensatz</label>
        <select id="map-agentur-dataset-select" v-model="selectedAgenturDataset">
          <option value="">BA-Monatswerte (Arbeitslose / Unterbeschäftigung)</option>
          <option v-for="slug in agenturAggDatasetOptions" :key="slug" :value="slug">
            {{ formatDatasetLabel(slug) }} (Kreise aggregiert)
          </option>
        </select>

        <template v-if="selectedAgenturDataset">
          <label for="map-agentur-agg-year-select">Jahr</label>
          <select id="map-agentur-agg-year-select" v-model.number="selectedAgenturAggYear">
            <option v-for="year in agenturAggYearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: selectedAgenturDataset } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
        <template v-else>
          <label for="map-agentur-metric-select">Metrik</label>
          <select id="map-agentur-metric-select" v-model="selectedAgenturMetric">
            <option value="arbeitslose">Arbeitslose</option>
            <option value="unterbeschaeftigung">Unterbeschäftigung</option>
          </select>

          <label for="map-agentur-year-select">Jahr</label>
          <select id="map-agentur-year-select" v-model.number="selectedAgenturYear">
            <option v-for="year in agenturYearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <label for="map-agentur-month-select">Monat</label>
          <select id="map-agentur-month-select" v-model.number="selectedAgenturMonth">
            <option v-for="month in agenturMonthOptions" :key="month" :value="month">{{ String(month).padStart(2, "0") }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: AGENTUR_DATASET_SLUG } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
      </template>
      <template v-else>
        <label for="map-dataset-select">Datensatz</label>
        <select id="map-dataset-select" v-model="selectedDataset">
          <option value="">Kein Datensatz (nur Karte)</option>
          <option v-for="slug in datasetOptions" :key="slug" :value="slug">{{ formatDatasetLabel(slug) }}</option>
        </select>

        <template v-if="selectedDataset">
          <label for="map-year-select">Jahr</label>
          <select id="map-year-select" v-model.number="selectedYear">
            <option v-for="year in yearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: selectedDataset } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
      </template>

      <template v-if="selectedGeoType === 'nuts'">
        <label for="map-nuts-version-select">NUTS-Version</label>
        <select id="map-nuts-version-select" v-model.number="selectedNutsVersion">
          <option v-for="version in NUTS_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>
      <template v-else-if="selectedGeoType === 'lau'">
        <label for="map-lau-version-select">LAU-Version</label>
        <select id="map-lau-version-select" v-model.number="selectedLauVersion">
          <option v-for="version in LAU_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>

      <span v-if="valuesLoading" class="hint">Lade Werte…</span>
      <span v-if="valuesError" class="error-text">Fehler: {{ valuesError }}</span>
    </div>

    <div class="map-layout">
      <div class="map-wrap">
        <div ref="mapContainer" class="map-container"></div>
        <div class="map-legend">
          <template v-if="selectedGeoType === 'agentur'">
            <p v-if="selectedAgenturDataset" class="map-legend-title">
              {{ formatDatasetLabel(selectedAgenturDataset) }}
              <template v-if="selectedAgenturAggYear"> · {{ selectedAgenturAggYear }}</template>
            </p>
            <p v-else class="map-legend-title">
              {{ AGENTUR_METRIC_LABELS[selectedAgenturMetric] }}
              <template v-if="selectedAgenturYear && selectedAgenturMonth">
                · {{ formatMonthLabel(selectedAgenturYear, selectedAgenturMonth) }}
              </template>
            </p>
            <div class="map-legend-scale">
              <span v-for="step in sequentialSteps(isDark)" :key="step" class="map-legend-swatch" :style="{ background: step }"></span>
            </div>
            <div class="map-legend-labels">
              <span>{{ valueDomain ? formatNumber(valueDomain[0]) : "–" }}</span>
              <span>{{ valueDomain ? formatNumber(valueDomain[1]) : "–" }}</span>
            </div>
            <p class="map-legend-note">
              <span class="map-legend-swatch map-legend-swatch--nodata"></span>keine Daten
            </p>
            <AgenturAggregationNote v-if="selectedAgenturDataset" :points="Array.from(valuesByRegion.values())" />
          </template>
          <template v-else-if="selectedDataset">
            <p class="map-legend-title">{{ formatDatasetLabel(selectedDataset) }}<template v-if="selectedYear"> · {{ selectedYear }}</template></p>
            <div class="map-legend-scale">
              <span v-for="step in sequentialSteps(isDark)" :key="step" class="map-legend-swatch" :style="{ background: step }"></span>
            </div>
            <div class="map-legend-labels">
              <span>{{ valueDomain ? formatNumber(valueDomain[0]) : "–" }}</span>
              <span>{{ valueDomain ? formatNumber(valueDomain[1]) : "–" }}</span>
            </div>
            <p class="map-legend-note">
              <span class="map-legend-swatch map-legend-swatch--nodata"></span>keine Daten
              <template v-if="!usesNutsAggregate">
                <span class="map-legend-swatch map-legend-swatch--interpolated"></span>interpoliert (gestrichelter Rand)
              </template>
            </p>
            <p v-if="usesNutsAggregate" class="hint aggregation-note">
              <strong>Aggregation:</strong> Werte je {{ NUTS_LEVEL_LABELS[selectedNutsLevel] }} sind aus den
              zugehörigen Kreis-Werten hochgerechnet (Summe bzw. bevölkerungsgewichteter Durchschnitt, je nach
              Datensatz) - eine Näherung, keine eigenständige Erhebung auf dieser Ebene.
            </p>
          </template>
          <p v-else class="map-legend-title">
            <span
              class="map-legend-swatch"
              :style="{
                background: isDark ? MAP_ONLY_FILL_DARK : MAP_ONLY_FILL_LIGHT,
                display: 'inline-block',
                width: '10px',
                height: '10px',
                borderRadius: '2px',
                marginRight: '0.3rem',
                verticalAlign: 'middle',
              }"
            ></span>
            {{ mapOnlyLabel }}
          </p>
        </div>
      </div>

      <aside class="map-sidebar">
        <template v-if="sidebarRegion">
          <div class="map-sidebar-header">
            <h3>{{ sidebarRegion.name }}</h3>
            <span class="region-code">{{ sidebarRegion.id }}</span>
          </div>
          <dl class="map-sidebar-data">
            <template v-if="!sidebarRegion.point">
              <dt>{{ sidebarValueLabel }}</dt>
              <dd>keine Daten</dd>
            </template>
            <template v-else>
              <dt>Jahr</dt>
              <dd>{{ sidebarRegion.point.year }}</dd>
              <dt>{{ sidebarValueLabel }}</dt>
              <dd>{{ formatNumber(sidebarRegion.point.value) }}</dd>
              <!-- source_region_count is only set for aggregated values (Agentur
                   Kreis aggregate or NUTS-0/1/2 Kreis aggregate, see MapValue above) -
                   both get the same "aggregiert aus..." row instead of the usual
                   observed/interpolated distinction, which doesn't apply to aggregates. -->
              <template v-if="sidebarRegion.point.source_region_count !== undefined">
                <dt>Status</dt>
                <dd>
                  aggregiert aus {{ sidebarRegion.point.source_region_count }} Kreis(en)
                  <template v-if="sidebarRegion.point.aggregation_method === 'sum'">(Summe)</template>
                  <template v-else-if="sidebarRegion.point.value_kind === 'median'">(Näherung, nur Schätzung)</template>
                  <template v-else-if="selectedGeoType === 'agentur'">(Näherung: gewichteter Durchschnitt)</template>
                </dd>
              </template>
              <template v-else-if="selectedGeoType !== 'agentur'">
                <dt>Status</dt>
                <dd>{{ sidebarRegion.point.is_interpolated ? "interpoliert" : "beobachtet" }}</dd>
              </template>
            </template>
          </dl>
          <RouterLink v-if="sidebarLinkTarget" :to="sidebarLinkTarget" class="link-button">
            Datensatz für diese Region ansehen ↗
          </RouterLink>
          <button
            v-if="pinnedRegionId && !hoveredRegionId"
            type="button"
            class="link-button map-sidebar-clear"
            @click="pinnedRegionId = null"
          >
            Auswahl aufheben
          </button>
        </template>
        <p v-else class="hint">Über eine Region auf der Karte fahren oder klicken, um Details zu sehen.</p>
      </aside>
    </div>
  </div>
</template>
