<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { API_BASE, apiGet } from "../api/client";
import { MAX_GEOJSON_BYTES, parseRegionGeoJSON, type RegionGeometry } from "../customRegion";
import type { CustomRegionDataset, DatasetCatalogEntry, GeoRegionSummary } from "../types/api";
import CombineRegionsPicker from "./CombineRegionsPicker.vue";
import DatasetPicker from "./DatasetPicker.vue";

// "Eigene Region": the user pastes or uploads a GeoJSON (Polygon/MultiPolygon) and
// gets the time series of the selected datasets aggregated over that region
// (POST /custom-regions/dataset-aggregate). The region is only sent per request
// and not stored; the browser remembers the last used GeoJSON locally
// (localStorage), purely as a convenience.

const STORAGE_KEY = "customRegion.geojson";

type InputMode = "geojson" | "combine";
const mode = ref<InputMode>("geojson");
const geojsonGeometry = ref<RegionGeometry | null>(null);
const combinedGeometry = ref<RegionGeometry | null>(null);
const combinePicker = ref<InstanceType<typeof CombineRegionsPicker> | null>(null);

// Both input modes yield the same geometry; the result UI is shared.
const geometry = computed(() => (mode.value === "combine" ? combinedGeometry.value : geojsonGeometry.value));

const geojsonText = ref("");
const parseError = ref<string | null>(null);

const datasets = ref<CustomRegionDataset[]>([]);
const selected = ref<string[]>([]);
const loadingDatasets = ref(true);

const requestError = ref<string | null>(null);

const mapContainer = ref<HTMLElement | null>(null);
let map: L.Map | null = null;
let regionLayer: L.GeoJSON | null = null;

// For the DatasetPicker: one entry per dataset, "slug::indikator" per indicator
// when there are several (Agenturbezirke).
const pickerDatasets = computed<DatasetCatalogEntry[]>(() => {
  const perSlug = new Map<string, string[]>();
  for (const d of datasets.value) (perSlug.get(d.dataset_slug) ?? perSlug.set(d.dataset_slug, []).get(d.dataset_slug)!).push(d.indicator);
  return [...perSlug.entries()].flatMap(([slug, indicators]) =>
    indicators.length > 1
      ? indicators.map((i) => ({ route_slug: `${slug}::${i}`, available_years: [] }))
      : [{ route_slug: slug, available_years: [] }],
  );
});
const selectedBaseSlugs = computed(() => [...new Set(selected.value.map((s) => s.split("::")[0]))]);

function applyText(text: string) {
  geojsonText.value = text;
  const parsed = parseRegionGeoJSON(text);
  if ("error" in parsed) {
    geojsonGeometry.value = null;
    parseError.value = text.trim() ? parsed.error : null;
    return;
  }
  parseError.value = null;
  geojsonGeometry.value = parsed.geometry;
  try {
    localStorage.setItem(STORAGE_KEY, text);
  } catch {
    // localStorage may be blocked (private window) - convenience only.
  }
}

async function onFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  applyText(await file.text());
}

// Country and Bundesland borders for orientation (the map has no tile layer).
// If the preview fails, so be it.
async function loadBoundaries() {
  if (!map) return;
  try {
    const responses = await Promise.all(
      [0, 1].map((level) =>
        apiGet<GeoRegionSummary[]>("/geo", {
          geo_type: "nuts",
          level,
          nuts_parent_id: "DE",
          include_geometry: true,
          limit: "null",
        }),
      ),
    );
    const dark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
    L.geoJSON(
      {
        type: "FeatureCollection",
        features: responses
          .flatMap((r) => r.data ?? [])
          .filter((r) => r.geometry_wgs84)
          .map((r) => ({
            type: "Feature" as const,
            properties: { nuts_level: r.nuts_level },
            geometry: r.geometry_wgs84 as GeoJSON.Geometry,
          })),
      } as GeoJSON.FeatureCollection,
      {
        interactive: false,
        style: (feature) => ({
          fill: false,
          color: dark ? "#9aa0a6" : "#5f6368",
          weight: feature?.properties?.nuts_level === 0 ? 2 : 1,
          opacity: feature?.properties?.nuts_level === 0 ? 0.9 : 0.55,
        }),
      },
    ).addTo(map);
    regionLayer?.bringToFront();
  } catch {
    // orientation aid only
  }
}

function drawRegion() {
  if (!map) return;
  if (regionLayer) {
    regionLayer.remove();
    regionLayer = null;
  }
  if (!geometry.value) return;
  regionLayer = L.geoJSON(geometry.value as any, { style: { color: "#2a6fdb", weight: 2, fillOpacity: 0.25 } }).addTo(map);
  // Don't fit at size 0 (hidden tab) - the zoom would be invalid; the mode
  // watcher catches up on switching back.
  const size = map.getSize();
  if (size.x && size.y) map.fitBounds(regionLayer.getBounds(), { padding: [16, 16], animate: false });
  regionLayer.bringToFront();
}

watch(geometry, drawRegion);
watch(mode, () => {
  queryGeometry.value = null;
  requestError.value = null;
  nextTick(() => {
    map?.invalidateSize();
    drawRegion();
    combinePicker.value?.invalidate();
  });
});

// The query only runs after a click (not on every selection change on the
// map); the DatasetPicker then loads each selected dataset itself.
const queryGeometry = ref<RegionGeometry | null>(null);
const queryId = ref(0);
const canRun = computed(() => !!geometry.value && pickerDatasets.value.length > 0);

function run() {
  if (!geometry.value) return;
  if (new Blob([JSON.stringify(geometry.value)]).size > MAX_GEOJSON_BYTES) {
    requestError.value = "Die Region ist zu groß (mehr als 5 MB Geometrie) – bitte weniger oder gröbere Einheiten wählen.";
    return;
  }
  requestError.value = null;
  if (!selected.value.length) selected.value = [pickerDatasets.value[0].route_slug];
  queryGeometry.value = geometry.value;
  queryId.value++;
}

const curlSnippet = computed(() => {
  if (!queryGeometry.value) return "";
  const body = JSON.stringify({ geometry: queryGeometry.value, datasets: selectedBaseSlugs.value });
  const base = API_BASE.startsWith("http") ? API_BASE : `${window.location.origin}${API_BASE}`;
  return `curl -X POST '${base}/custom-regions/dataset-aggregate?format=csv' \\\n  -H 'Content-Type: application/json' \\\n  -d '${body.replace(/'/g, "'\\''")}'`;
});

async function downloadCsv() {
  if (!queryGeometry.value) return;
  requestError.value = null;
  try {
    const res = await fetch(`${API_BASE}/custom-regions/dataset-aggregate?format=csv&download=1`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ geometry: queryGeometry.value, datasets: selectedBaseSlugs.value }),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const url = URL.createObjectURL(await res.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = "eigene-region.csv";
    a.click();
    URL.revokeObjectURL(url);
  } catch (err) {
    requestError.value = `Download fehlgeschlagen: ${err instanceof Error ? err.message : String(err)}`;
  }
}

onMounted(async () => {
  if (mapContainer.value) {
    map = L.map(mapContainer.value, { center: [51.2, 10.4], zoom: 5, scrollWheelZoom: false });
  }

  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) applyText(saved);
  } catch {
    // see above
  }

  try {
    const list = await apiGet<CustomRegionDataset[]>("/custom-regions/datasets");
    datasets.value = list.data ?? [];
  } catch (err) {
    requestError.value = `Datensätze konnten nicht geladen werden: ${err instanceof Error ? err.message : String(err)}`;
  } finally {
    loadingDatasets.value = false;
  }
  drawRegion();
  loadBoundaries();
});

onBeforeUnmount(() => {
  map?.remove();
  map = null;
});
</script>

<template>
  <section class="custom-region">
    <div class="panel">
      <h2>Eigene Region</h2>
      <div class="sub-nav" role="tablist" aria-label="Eingabeart">
        <button type="button" role="tab" :aria-selected="mode === 'geojson'" class="nav-tab" :class="{ 'is-active': mode === 'geojson' }" @click="mode = 'geojson'">
          GeoJSON einfügen
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'combine'"
          class="nav-tab"
          :class="{ 'is-active': mode === 'combine' }"
          @click="mode = 'combine'"
        >
          Regionen kombinieren
        </button>
      </div>

      <div v-show="mode === 'geojson'">
      <p class="hint">
        GeoJSON (Polygon oder MultiPolygon, WGS84, Koordinaten als [lon, lat]) einfügen oder hochladen. Die Region wird
        nur für die Abfrage an die API gesendet und nicht gespeichert.
      </p>

      <label class="visually-hidden" for="custom-region-geojson">GeoJSON</label>
      <textarea
        id="custom-region-geojson"
        class="custom-region-input"
        rows="8"
        spellcheck="false"
        placeholder='{"type":"Polygon","coordinates":[[[13.3,52.4],[13.5,52.4],[13.5,52.6],[13.3,52.6],[13.3,52.4]]]}'
        :value="geojsonText"
        @input="applyText(($event.target as HTMLTextAreaElement).value)"
      />
      <p>
        <label class="link-button">
          Datei auswählen …
          <input type="file" accept=".geojson,.json,application/geo+json,application/json" class="visually-hidden" @change="onFileChange" />
        </label>
      </p>
      <p v-if="parseError" class="error-text" role="alert">{{ parseError }}</p>
      <p v-else-if="geometry" class="hint">✓ {{ geometry.type }} erkannt.</p>

      <div ref="mapContainer" class="map-container custom-region-map" aria-label="Vorschau der Region"></div>
      </div>
      <CombineRegionsPicker v-show="mode === 'combine'" ref="combinePicker" @update:geometry="combinedGeometry = $event" />
    </div>

    <div class="panel">
      <p v-if="loadingDatasets" class="hint">Lade Datensätze …</p>
      <p>
        <button type="button" :disabled="!canRun || !geometry" @click="run">Daten für diese Region abfragen</button>
      </p>
      <p v-if="requestError" class="error-text" role="alert">{{ requestError }}</p>
    </div>

    <div v-if="queryGeometry" class="panel">
      <h3>Ergebnis</h3>
      <DatasetPicker
        :key="queryId"
        kind="custom"
        :datasets="pickerDatasets"
        nuts-code=""
        :geometry="queryGeometry"
        :selected-slugs="selected"
        @update:selected-slugs="(s) => (selected = s)"
      />
      <p>
        <button type="button" @click="downloadCsv">CSV herunterladen</button>
      </p>
      <details>
        <summary>API-Aufruf (curl)</summary>
        <pre class="api-snippet">{{ curlSnippet }}</pre>
      </details>
    </div>
  </section>
</template>

<style scoped>
.sub-nav {
  display: flex;
  gap: 0.25rem;
  margin: 0.5rem 0 0.75rem;
}

.custom-region {
  display: grid;
  gap: 1rem;
  max-width: 60rem;
  margin: 0 auto;
}

.custom-region-input {
  width: 100%;
  font-family: monospace;
  font-size: 0.8rem;
  padding: 0.5rem 0.65rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--page-plane);
  color: var(--text-primary);
}

.custom-region-map {
  height: 320px;
  min-height: 0;
  border: 1px solid var(--border);
  border-radius: 6px;
  margin-top: 0.5rem;
}

.api-snippet {
  overflow-x: auto;
  font-size: 0.8rem;
}
</style>
