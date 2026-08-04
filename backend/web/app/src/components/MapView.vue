<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { apiGet } from "../api/client";
import type { GeoRegionSummary, InventoryEntry, NutsTimeseriesPoint } from "../types/api";
import { formatDatasetLabel, formatNumber } from "../format";
import { NO_DATA_FILL_DARK, NO_DATA_FILL_LIGHT, sequentialColor, sequentialSteps } from "../mapColor";

const LIGHT_TILES = "https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png";
const DARK_TILES = "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png";
const TILE_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>';

const mapContainer = ref<HTMLDivElement | null>(null);
let map: L.Map | null = null;
let tileLayer: L.TileLayer | null = null;
let geoJsonLayer: L.GeoJSON | null = null;
let lastBounds: L.LatLngBounds | null = null;

const isDark = ref(window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);

const geoError = ref<string | null>(null);
const regions = ref<GeoRegionSummary[]>([]);

const inventoryError = ref<string | null>(null);
// dataset-slug -> verfügbare Jahre (absteigend, neuestes zuerst)
const datasetYears = ref<Map<string, number[]>>(new Map());
const selectedDataset = ref<string>("");
const selectedYear = ref<number | null>(null);

const valuesLoading = ref(false);
const valuesError = ref<string | null>(null);
const valuesByRegion = ref<Map<string, NutsTimeseriesPoint>>(new Map());

const datasetOptions = computed(() => Array.from(datasetYears.value.keys()).sort());
const yearOptions = computed(() => datasetYears.value.get(selectedDataset.value) ?? []);

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

function styleForFeature(feature?: GeoJSON.Feature): L.PathOptions {
  const geoId = feature?.properties?.geo_id as string | undefined;
  const point = geoId ? valuesByRegion.value.get(geoId) : undefined;
  const borderColor = isDark.value ? "#0d0d0d" : "#ffffff";
  if (!point) {
    return {
      fillColor: isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT,
      fillOpacity: 0.6,
      color: borderColor,
      weight: 1,
    };
  }
  return {
    fillColor: colorForValue(point.value),
    fillOpacity: 0.85,
    color: borderColor,
    weight: point.is_interpolated ? 1 : 1.5,
    dashArray: point.is_interpolated ? "3 2" : undefined,
  };
}

function tooltipForFeature(feature?: GeoJSON.Feature): string {
  const geoId = feature?.properties?.geo_id as string | undefined;
  const name = (feature?.properties?.region_name as string | undefined) ?? geoId ?? "";
  const point = geoId ? valuesByRegion.value.get(geoId) : undefined;
  const rows: string[] = [`<dt>Region</dt><dd>${name}</dd>`];
  if (!point) {
    rows.push(`<dt>Wert</dt><dd>keine Daten</dd>`);
  } else {
    rows.push(`<dt>Jahr</dt><dd>${point.year}</dd>`);
    rows.push(`<dt>Wert</dt><dd>${formatNumber(point.value)}</dd>`);
    rows.push(`<dt>Status</dt><dd>${point.is_interpolated ? "interpoliert" : "beobachtet"}</dd>`);
  }
  return `<dl class="map-tooltip-dl">${rows.join("")}</dl>`;
}

function restyleLayer() {
  geoJsonLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature };
    l.setStyle(styleForFeature(l.feature));
    l.setTooltipContent(tooltipForFeature(l.feature));
  });
}

function swapTileLayer() {
  if (!map) return;
  if (tileLayer) map.removeLayer(tileLayer);
  tileLayer = L.tileLayer(isDark.value ? DARK_TILES : LIGHT_TILES, {
    attribution: TILE_ATTRIBUTION,
    subdomains: "abcd",
    maxZoom: 19,
  });
  tileLayer.addTo(map);
}

async function loadGeometries() {
  try {
    // country_code wird laut API nur für geo_type=lau ausgewertet; für NUTS
    // grenzt nuts_parent_id (Präfix-Filter) auf Deutschland ein.
    const body = await apiGet<GeoRegionSummary[]>("/geo", {
      geo_type: "nuts",
      level: 3,
      nuts_parent_id: "DE",
      include_geometry: true,
      limit: "null",
    });
    const all = (body.data ?? []).filter((r) => r.geometry_wgs84);
    // /geo liefert bei geometry_strategy=latest (Default) auch stillgelegte
    // NUTS-Codes aus früheren Gebietsreformen mit älterem vintage_year: nur
    // der aktuellste Vintage entspricht den ~400 Kreisen, für die
    // mart_nuts3_timeseries_interpolated tatsächlich Werte führt.
    const maxVintage = Math.max(...all.map((r) => r.vintage_year ?? 0));
    regions.value = all.filter((r) => r.vintage_year === maxVintage);
  } catch (err) {
    geoError.value = err instanceof Error ? err.message : String(err);
  }
}

async function loadInventory() {
  try {
    const body = await apiGet<InventoryEntry[]>("/socioeconomic/inventory", { geodata_level: "NUTS_3" });
    const byDataset = new Map<string, Set<number>>();
    for (const entry of body.data ?? []) {
      if (!byDataset.has(entry.dataset)) byDataset.set(entry.dataset, new Set());
      byDataset.get(entry.dataset)!.add(entry.year);
    }
    const sorted = new Map<string, number[]>();
    for (const [slug, years] of byDataset) {
      sorted.set(slug, Array.from(years).sort((a, b) => b - a));
    }
    datasetYears.value = sorted;
    const firstDataset = Array.from(sorted.keys()).sort()[0];
    if (firstDataset) {
      selectedDataset.value = firstDataset;
      selectedYear.value = sorted.get(firstDataset)?.[0] ?? null;
    }
  } catch (err) {
    inventoryError.value = err instanceof Error ? err.message : String(err);
  }
}

async function loadValues() {
  if (!selectedDataset.value || selectedYear.value === null) return;
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    const body = await apiGet<NutsTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
      year: selectedYear.value,
      limit: "null",
    });
    valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.nuts_id, p]));
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

function buildLayer() {
  if (!map || !regions.value.length) return;
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
    onEachFeature: (feature, layer) => {
      layer.bindTooltip(tooltipForFeature(feature), { sticky: true, className: "map-tooltip" });
    },
  }).addTo(map);

  const bounds = geoJsonLayer.getBounds();
  if (bounds.isValid()) {
    lastBounds = bounds;
    map.fitBounds(bounds, { padding: [8, 8] });
  }
}

let darkModeQuery: MediaQueryList | null = null;
function handleDarkModeChange(evt: MediaQueryListEvent) {
  isDark.value = evt.matches;
}

// Der Karten-Tab wird per v-show (nicht v-if) ein-/ausgeblendet, damit sein
// Zustand beim Tab-Wechsel erhalten bleibt - das Container-Div hat beim
// Leaflet-Init also ggf. noch Größe 0 (display:none). Ein ResizeObserver
// stößt invalidateSize() an, sobald der Container tatsächlich sichtbare
// Ausmaße bekommt (z.B. beim ersten Wechsel auf den Karte-Tab).
let resizeObserver: ResizeObserver | null = null;

onMounted(async () => {
  if (mapContainer.value) {
    map = L.map(mapContainer.value, { center: [51.2, 10.4], zoom: 6, scrollWheelZoom: false });
    swapTileLayer();

    resizeObserver = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      if (width > 0 && height > 0) {
        map?.invalidateSize();
        if (lastBounds) map?.fitBounds(lastBounds, { padding: [8, 8] });
      }
    });
    resizeObserver.observe(mapContainer.value);
  }

  darkModeQuery = window.matchMedia?.("(prefers-color-scheme: dark)") ?? null;
  darkModeQuery?.addEventListener("change", handleDarkModeChange);

  await loadGeometries();
  buildLayer();
  await loadInventory();
});

onUnmounted(() => {
  darkModeQuery?.removeEventListener("change", handleDarkModeChange);
  resizeObserver?.disconnect();
  map?.remove();
});

watch([selectedDataset, selectedYear], loadValues);
watch(isDark, () => {
  swapTileLayer();
  restyleLayer();
});
</script>

<template>
  <div class="map-view">
    <p v-if="geoError" class="error-text">Geometrien konnten nicht geladen werden: {{ geoError }}</p>
    <p v-if="inventoryError" class="error-text">Datensatz-Übersicht konnte nicht geladen werden: {{ inventoryError }}</p>

    <div class="dataset-picker">
      <label for="map-dataset-select">Datensatz</label>
      <select id="map-dataset-select" v-model="selectedDataset">
        <option v-for="slug in datasetOptions" :key="slug" :value="slug">{{ formatDatasetLabel(slug) }}</option>
      </select>

      <label for="map-year-select">Jahr</label>
      <select id="map-year-select" v-model.number="selectedYear">
        <option v-for="year in yearOptions" :key="year" :value="year">{{ year }}</option>
      </select>

      <span v-if="valuesLoading" class="hint">Lade Werte…</span>
      <span v-if="valuesError" class="error-text">Fehler: {{ valuesError }}</span>
    </div>

    <div class="map-wrap">
      <div ref="mapContainer" class="map-container"></div>
      <div class="map-legend">
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
          <span class="map-legend-swatch map-legend-swatch--interpolated"></span>interpoliert (gestrichelter Rand)
        </p>
      </div>
    </div>
  </div>
</template>
