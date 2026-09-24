<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { API_BASE, apiGet, apiPost } from "../api/client";
import { parseRegionGeoJSON, type RegionGeometry } from "../customRegion";
import { formatMonthLabel, formatNumber, formatPercent } from "../format";
import type {
  CustomRegionAggregateRow,
  CustomRegionDataset,
  DatasetMetadataEntry,
  GeoRegionSummary,
} from "../types/api";
import TimeseriesChart from "./TimeseriesChart.vue";

// "Eigene Region": Nutzer fügt ein GeoJSON (Polygon/MultiPolygon) ein oder lädt
// es hoch und bekommt die Zeitreihen der gewählten Datensätze aggregiert über
// diese Region (POST /custom-regions/dataset-aggregate). Die Region wird nur pro
// Anfrage gesendet und nicht gespeichert; zuletzt genutztes GeoJSON merkt sich
// der Browser lokal (localStorage), rein als Komfort.

const STORAGE_KEY = "customRegion.geojson";

const geojsonText = ref("");
const parseError = ref<string | null>(null);
const geometry = ref<RegionGeometry | null>(null);

const datasets = ref<CustomRegionDataset[]>([]);
const datasetNames = ref<Record<string, string>>({});
const selected = ref<string[]>([]);
const loadingDatasets = ref(true);

const running = ref(false);
const requestError = ref<string | null>(null);
const rows = ref<CustomRegionAggregateRow[]>([]);
const hasResult = ref(false);

const mapContainer = ref<HTMLElement | null>(null);
let map: L.Map | null = null;
let regionLayer: L.GeoJSON | null = null;

const LEVEL_LABEL: Record<string, string> = { lau: "Gemeinden (LAU)", nuts3: "Kreise (NUTS-3)", agentur: "Agenturbezirke" };

// Ein Eintrag je Datensatz-Slug (Agentur-Slug hat zwei Indikatoren).
const selectableSlugs = computed(() => [...new Set(datasets.value.map((d) => d.dataset_slug))]);

function labelFor(slug: string): string {
  return datasetNames.value[slug] || slug;
}

function levelFor(slug: string): string {
  const levels = [...new Set(datasets.value.filter((d) => d.dataset_slug === slug).map((d) => d.level))];
  return levels.map((l) => LEVEL_LABEL[l] ?? l).join(", ");
}

function applyText(text: string) {
  geojsonText.value = text;
  const parsed = parseRegionGeoJSON(text);
  if ("error" in parsed) {
    geometry.value = null;
    parseError.value = text.trim() ? parsed.error : null;
    return;
  }
  parseError.value = null;
  geometry.value = parsed.geometry;
  try {
    localStorage.setItem(STORAGE_KEY, text);
  } catch {
    // localStorage kann gesperrt sein (privates Fenster) - nur Komfortfunktion.
  }
}

async function onFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  applyText(await file.text());
}

// Staats- und Bundeslandgrenzen als Orientierung (die Karte hat keine
// Kachel-Ebene. Sollte die Vorschau nicht funktionieren, ist das halt so
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
    // nur Orientierungshilfe
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
  map.fitBounds(regionLayer.getBounds(), { padding: [16, 16] });
  regionLayer.bringToFront();
}

watch(geometry, drawRegion);

const canRun = computed(() => !!geometry.value && selected.value.length > 0 && !running.value);

async function run() {
  if (!geometry.value) return;
  running.value = true;
  requestError.value = null;
  try {
    const body = await apiPost<CustomRegionAggregateRow[]>("/custom-regions/dataset-aggregate", {
      geometry: geometry.value,
      datasets: selected.value,
    });
    rows.value = body.data ?? [];
    hasResult.value = true;
  } catch (err) {
    rows.value = [];
    hasResult.value = false;
    requestError.value = err instanceof Error ? err.message : String(err);
  } finally {
    running.value = false;
  }
}

// Ergebnis gruppiert nach (Datensatz, Indikator).
const groups = computed(() => {
  const map = new Map<string, CustomRegionAggregateRow[]>();
  for (const r of rows.value) {
    const key = `${r.dataset_slug}::${r.indicator}`;
    (map.get(key) ?? map.set(key, []).get(key)!).push(r);
  }
  return [...map.entries()].map(([key, groupRows]) => {
    const first = groupRows[0];
    const monthly = groupRows.some((r) => r.month != null);
    const title =
      first.indicator === first.dataset_slug
        ? labelFor(first.dataset_slug)
        : `${labelFor(first.dataset_slug)} – ${first.indicator}`;
    return {
      key,
      title,
      rows: groupRows,
      monthly,
      // Diagramm nur für Jahreswerte; Monatsreihen (Agenturbezirke) nur als Tabelle.
      points: monthly
        ? []
        : groupRows.map((r) => ({ year: r.year, value: r.value, is_interpolated: false, is_computed: true })),
    };
  });
});

function methodNote(r: CustomRegionAggregateRow): string {
  switch (r.aggregation_method) {
    case "sum":
      return "Summe (flächenanteilig)";
    case "weighted_avg_population":
      return "Bevölkerungsgewichtet";
    case "weighted_avg_area":
      return "Flächengewichtet";
    default:
      return r.aggregation_method;
  }
}

function notesFor(groupRows: CustomRegionAggregateRow[]): string[] {
  const out: string[] = [];
  const methods = new Set(groupRows.map((r) => r.aggregation_method));
  if (methods.has("weighted_avg_population")) {
    out.push("Quoten/Durchschnitte sind bevölkerungsgewichtete Mittel der Einheitenwerte – eine Näherung.");
  }
  if (methods.has("weighted_avg_area")) {
    out.push("Teilweise flächengewichtet (für mindestens eine Einheit fehlt die Bevölkerung) – Näherung.");
  }
  if (groupRows.some((r) => r.value_kind === "median")) {
    out.push("Mediane lassen sich nicht aus Einheitenwerten zusammensetzen – der Wert ist nur eine Schätzung.");
  }
  if (groupRows.some((r) => r.coverage < 0.95)) {
    out.push("Für einen Teil der Fläche liegen keine Daten vor (Abdeckung < 95 %) – der Wert gilt nur für den abgedeckten Teil.");
  }
  return out;
}

const curlSnippet = computed(() => {
  if (!geometry.value) return "";
  const body = JSON.stringify({ geometry: geometry.value, datasets: selected.value });
  const base = API_BASE.startsWith("http") ? API_BASE : `${window.location.origin}${API_BASE}`;
  return `curl -X POST '${base}/custom-regions/dataset-aggregate?format=csv' \\\n  -H 'Content-Type: application/json' \\\n  -d '${body.replace(/'/g, "'\\''")}'`;
});

async function downloadCsv() {
  if (!geometry.value) return;
  requestError.value = null;
  try {
    const res = await fetch(`${API_BASE}/custom-regions/dataset-aggregate?format=csv&download=1`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ geometry: geometry.value, datasets: selected.value }),
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
    // s. o.
  }

  try {
    const [list, meta] = await Promise.all([
      apiGet<CustomRegionDataset[]>("/custom-regions/datasets"),
      apiGet<DatasetMetadataEntry[]>("/datasets").catch(() => null),
    ]);
    datasets.value = list.data ?? [];
    datasetNames.value = Object.fromEntries(
      (meta?.data ?? []).filter((m) => m.name).map((m) => [m.dataset_slug, m.name as string]),
    );
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

    <div class="panel">
      <h3>Datensätze</h3>
      <p v-if="loadingDatasets" class="hint">Lade Datensätze …</p>
      <ul v-else class="custom-region-datasets">
        <li v-for="slug in selectableSlugs" :key="slug">
          <label>
            <input v-model="selected" type="checkbox" :value="slug" />
            {{ labelFor(slug) }}
            <span class="hint">({{ levelFor(slug) }})</span>
          </label>
        </li>
      </ul>
      <p>
        <button type="button" :disabled="!canRun" @click="run">
          {{ running ? "Berechne …" : "Daten abfragen" }}
        </button>
      </p>
      <p v-if="requestError" class="error-text" role="alert">{{ requestError }}</p>
    </div>

    <div v-if="hasResult" class="panel">
      <h3>Ergebnis</h3>
      <p v-if="!groups.length" class="hint">
        Für diese Region liegen zu den gewählten Datensätzen keine Werte vor (liegt sie außerhalb Deutschlands?).
      </p>

      <article v-for="g in groups" :key="g.key" class="custom-region-result">
        <h4>{{ g.title }} <span class="badge">{{ LEVEL_LABEL[g.rows[0].level_used] ?? g.rows[0].level_used }}</span></h4>
        <TimeseriesChart v-if="g.points.length > 1" :points="g.points" />
        <div class="data-table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ g.monthly ? "Monat" : "Jahr" }}</th>
                <th>Wert</th>
                <th>Methode</th>
                <th>Einheiten</th>
                <th>Abdeckung</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in g.rows" :key="`${r.year}-${r.month ?? 0}`" class="is-computed">
                <td>{{ r.month != null ? formatMonthLabel(r.year, r.month) : r.year }}</td>
                <td>{{ formatNumber(r.value) }}</td>
                <td>{{ methodNote(r) }}</td>
                <td>{{ r.source_region_count }}</td>
                <td>{{ formatPercent(r.coverage) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-for="note in notesFor(g.rows)" :key="note" class="hint">{{ note }}</p>
      </article>

      <p v-if="groups.length">
        <button type="button" @click="downloadCsv">CSV herunterladen</button>
      </p>
      <details v-if="groups.length">
        <summary>API-Aufruf (curl)</summary>
        <pre class="api-snippet">{{ curlSnippet }}</pre>
      </details>
    </div>
  </section>
</template>

<style scoped>
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

.custom-region-datasets {
  list-style: none;
  margin: 0 0 0.75rem;
  padding: 0;
  display: grid;
  gap: 0.25rem;
}

.custom-region-result {
  margin-top: 1rem;
}

.api-snippet {
  overflow-x: auto;
  font-size: 0.8rem;
}
</style>
