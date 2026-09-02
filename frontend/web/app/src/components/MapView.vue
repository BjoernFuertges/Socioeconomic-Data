<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { apiGet } from "../api/client";
import type { GeoRegionSummary, InventoryEntry, LauTimeseriesPoint, NutsTimeseriesPoint, TimeseriesPoint } from "../types/api";
import { formatDatasetLabel, formatNumber } from "../format";
import {
  MAP_ONLY_FILL_DARK,
  MAP_ONLY_FILL_LIGHT,
  NO_DATA_FILL_DARK,
  NO_DATA_FILL_LIGHT,
  sequentialColor,
  sequentialSteps,
} from "../mapColor";

const LIGHT_TILES = "https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png";
const DARK_TILES = "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png";
const TILE_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>';

// Unterstützte NUTS-3-Zielversionen - muss mit supportedNutsVersions in
// backend/internal/handlers/kreis_generic.go übereinstimmen. Entspricht den
// in stg_nuts_geometry vorhandenen Vintage-Jahren; mart_nuts_version_crosswalk
// berechnet für jedes Paar davon eine direkte (1-Hop) Abbildung.
const NUTS_VERSIONS = [2003, 2006, 2010, 2013, 2016, 2021, 2024] as const;

// Unterstützte LAU-Zielversionen - muss mit supportedLauVersions in
// backend/internal/handlers/lau_generic.go übereinstimmen. Entspricht den
// in mart_lau_geometry_historized vorhandenen Vintage-Jahren;
// mart_lau_version_crosswalk berechnet für jedes Paar davon eine Abbildung.
const LAU_VERSIONS = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024] as const;

type GeoType = "nuts" | "lau";

const mapContainer = ref<HTMLDivElement | null>(null);
let map: L.Map | null = null;
let tileLayer: L.TileLayer | null = null;
let geoJsonLayer: L.GeoJSON | null = null;
let lastBounds: L.LatLngBounds | null = null;

const isDark = ref(window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);

const geoError = ref<string | null>(null);
const regions = ref<GeoRegionSummary[]>([]);

const inventoryError = ref<string | null>(null);
// dataset-slug -> verfügbare Jahre (absteigend, neuestes zuerst), getrennt je
// Geo-Ebene (/socioeconomic/inventory liefert NUTS_3- und LAU-Einträge
// getrennt, und /{slug}/timeseries erwartet je nach Ebene nuts_version oder
// lau_version).
const datasetYearsNuts = ref<Map<string, number[]>>(new Map());
const datasetYearsLau = ref<Map<string, number[]>>(new Map());
// "" = kein Datensatz gewählt -> Karte zeigt nur den Grenzverlauf, keine Werte.
const selectedDataset = ref<string>("");
const selectedYear = ref<number | null>(null);
const selectedNutsVersion = ref<number>(2024);
const selectedLauVersion = ref<number>(2024);
const selectedGeoType = ref<GeoType>("nuts");

const valuesLoading = ref(false);
const valuesError = ref<string | null>(null);
const valuesByRegion = ref<Map<string, TimeseriesPoint>>(new Map());

const datasetYears = computed(() => (selectedGeoType.value === "lau" ? datasetYearsLau.value : datasetYearsNuts.value));
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

  if (!selectedDataset.value) {
    // "Kein Datensatz (nur Karte)" bzw. LAU-Ansicht: eigener, kontrastreicher
    // Stil statt des dezenten "keine Daten"-Grautons weiter unten - der wäre
    // auf dem dunklen Kartenhintergrund kaum sichtbar, wenn ihn ausnahmslos
    // jede Region bekommt (siehe MAP_ONLY_FILL_* in mapColor.ts).
    // dashArray explizit zurücksetzen: Leaflet's setStyle() merged Optionen nur
    // (setOptions kopiert per for...in nur vorhandene Keys) statt sie zu
    // ersetzen - ohne diese Zeile bliebe der gestrichelte Rand von zuvor
    // interpolierten Regionen (aus einem vorher gewählten Datensatz) hängen.
    return {
      fillColor: isDark.value ? MAP_ONLY_FILL_DARK : MAP_ONLY_FILL_LIGHT,
      fillOpacity: 0.75,
      color: borderColor,
      weight: 1,
      dashArray: undefined,
    };
  }

  if (!point) {
    return {
      fillColor: isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT,
      fillOpacity: 0.6,
      color: borderColor,
      weight: 1,
      dashArray: undefined,
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
  geoError.value = null;
  try {
    if (selectedGeoType.value === "lau") {
      // geometry_year pinnt auf den Grenzverlauf der gewählten LAU-Version -
      // /geo?geo_type=lau interpretiert geometry_year dabei als Jahresfilter
      // auf lau_integrated (siehe backend/internal/handlers/geo.go).
      // country_code grenzt auf Deutschland ein.
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

    // country_code wird laut API nur für geo_type=lau ausgewertet; für NUTS
    // grenzt nuts_parent_id (Präfix-Filter) auf Deutschland ein.
    // geometry_strategy=exact+geometry_year pinnt auf den Grenzverlauf der
    // gewählten NUTS-Version (siehe mart_nuts_geometry_historized) - damit
    // liefert /geo genau die Kreise, die in dieser Version gültig waren.
    const body = await apiGet<GeoRegionSummary[]>("/geo", {
      geo_type: "nuts",
      level: 3,
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

// Lädt die Datensatz-Übersicht für eine Geo-Ebene und wählt (falls noch
// nichts gewählt ist oder die bisherige Auswahl in dieser Ebene nicht
// existiert) den ersten verfügbaren Datensatz vor.
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
    if (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value)) {
      const firstDataset = Array.from(datasetYears.value.keys()).sort()[0];
      selectedDataset.value = firstDataset ?? "";
      selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
    }
  } catch (err) {
    inventoryError.value = err instanceof Error ? err.message : String(err);
  }
}

async function loadValues() {
  if (!selectedDataset.value || selectedYear.value === null) {
    // Kein Datensatz gewählt -> Karte bleibt ungefärbt, eventuell noch
    // vorhandene Werte des zuvor gewählten Datensatzes verwerfen.
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
    if (selectedGeoType.value === "lau") {
      const body = await apiGet<LauTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        lau_version: selectedLauVersion.value,
        limit: "null",
      });
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.gisco_id, p]));
    } else {
      const body = await apiGet<NutsTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        nuts_version: selectedNutsVersion.value,
        limit: "null",
      });
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.nuts_id, p]));
    }
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

function buildLayer() {
  if (!map) return;
  // Alte Ebene immer zuerst entfernen, auch wenn die neue Region-Liste leer
  // ist - sonst bliebe bei einem Versions-/Ebenenwechsel ohne Treffer die
  // vorherige Kartenebene (Grenzen + Einfärbung) fälschlich stehen.
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
// Beim Wechsel der NUTS-Version müssen sowohl der Grenzverlauf (andere
// Vintage-Periode, ggf. andere Kreis-IDs) als auch die Werte (andere
// target_version im harmonisierten Mart) neu geladen werden.
watch(selectedNutsVersion, async () => {
  if (selectedGeoType.value !== "nuts") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// Analog für die LAU-Version (andere target_version im LAU-harmonisierten Mart).
watch(selectedLauVersion, async () => {
  if (selectedGeoType.value !== "lau") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// Beim Wechsel der Ebene: falls der bisher gewählte Datensatz in der neuen
// Ebene nicht existiert (NUTS-3- und LAU-Datensätze sind getrennte Mengen),
// auf den ersten verfügbaren Datensatz der neuen Ebene umschalten (oder
// "" ohne Datensatz-Auswahl in dieser Ebene).
watch(selectedGeoType, async () => {
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
    <p v-if="inventoryError" class="error-text">Datensatz-Übersicht konnte nicht geladen werden: {{ inventoryError }}</p>

    <div class="dataset-picker">
      <label for="map-geo-type-select">Ebene</label>
      <select id="map-geo-type-select" v-model="selectedGeoType">
        <option value="nuts">NUTS-3 (Kreise)</option>
        <option value="lau">LAU (Gemeinden)</option>
      </select>

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
      </template>

      <template v-if="selectedGeoType === 'nuts'">
        <label for="map-nuts-version-select">NUTS-Version</label>
        <select id="map-nuts-version-select" v-model.number="selectedNutsVersion">
          <option v-for="version in NUTS_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>
      <template v-else>
        <label for="map-lau-version-select">LAU-Version</label>
        <select id="map-lau-version-select" v-model.number="selectedLauVersion">
          <option v-for="version in LAU_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>

      <span v-if="valuesLoading" class="hint">Lade Werte…</span>
      <span v-if="valuesError" class="error-text">Fehler: {{ valuesError }}</span>
    </div>

    <div class="map-wrap">
      <div ref="mapContainer" class="map-container"></div>
      <div class="map-legend">
        <template v-if="selectedDataset">
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
          {{ selectedGeoType === "lau" ? "LAU-Gemeinden" : "Kein Datensatz ausgewählt" }}
        </p>
      </div>
    </div>
  </div>
</template>
