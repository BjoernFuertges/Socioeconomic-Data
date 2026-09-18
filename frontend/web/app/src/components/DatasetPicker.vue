<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { apiGet } from "../api/client";
import type {
  DatasetCatalogEntry,
  DatasetMetadataEntry,
  GenericDatasetRow,
  NutsDatasetAggregatePoint,
  TimeseriesPoint,
} from "../types/api";
import { formatDatasetLabel } from "../format";
import TimeseriesChart from "./TimeseriesChart.vue";
import DataTable from "./DataTable.vue";
import DatasetInfo from "./DatasetInfo.vue";

// Deckelt die Mehrfachauswahl auf die Anzahl validierter kategorialer
// Farbslots (siehe SERIES_COLORS in TimeseriesChart.vue) - mehr Serien in
// der kombinierten Ansicht würden nicht mehr sicher unterscheidbar bleiben.
const MAX_SELECTED = 4;

type ViewMode = "separate" | "combined";
type LoadResult =
  | { kind: "loading" }
  | { kind: "timeseries"; points: TimeseriesPoint[] }
  | { kind: "raw"; rows: GenericDatasetRow[] }
  | { kind: "error"; message: string };

const props = defineProps<{
  datasets: DatasetCatalogEntry[];
  nutsCode: string;
  selectedSlugs: string[];
  viewMode: ViewMode;
  // NUTS-Ebene der aktuellen Region (0-3). Default 3 (Kreis) - bisheriges
  // Verhalten. Bei 0-2 (Land/Bundesland/Regierungsbezirk) gibt es keine
  // Kreis-grain-Zeilen unter diesem Code, deshalb lädt loadSlug() dann über
  // den auf diese Ebene hochaggregierten Endpoint statt /{slug}/timeseries
  // (siehe RegionBrowser.vue, mart_nuts012_dataset_aggregate).
  nutsLevel?: number;
}>();

const emit = defineEmits<{
  "update:selectedSlugs": [slugs: string[]];
  "update:viewMode": [mode: ViewMode];
}>();

// Reihenfolge der Checkboxen folgt der Reihenfolge in `datasets`, damit
// selectedDatasets (unten) für Chart-Serien/Tabs eine stabile Reihenfolge hat.
const selectedDatasets = computed(() => props.datasets.filter((ds) => props.selectedSlugs.includes(ds.route_slug)));
const atMax = computed(() => props.selectedSlugs.length >= MAX_SELECTED);

function toggleSlug(slug: string, checked: boolean) {
  if (checked) {
    if (atMax.value) return;
    emit("update:selectedSlugs", [...props.selectedSlugs, slug]);
  } else {
    // Mindestens ein Datensatz muss gewählt bleiben - sonst bliebe die
    // Auswahl "leer" während RegionBrowser.vue (nutsSelectedSlugs/
    // lauSelectedSlugs) und die Route weiterhin den zuletzt gültigen Stand
    // zeigen: ein widersprüchlicher Zustand (Checkbox unchecked, aber Chart/
    // URL zeigen noch den alten Datensatz).
    if (props.selectedSlugs.length <= 1) return;
    emit(
      "update:selectedSlugs",
      props.selectedSlugs.filter((s) => s !== slug),
    );
  }
}

const results = ref<Map<string, LoadResult>>(new Map());
const metadataBySlug = ref<Map<string, DatasetMetadataEntry | null>>(new Map());

async function loadSlug(slug: string) {
  results.value.set(slug, { kind: "loading" });
  try {
    const body = await apiGet<DatasetMetadataEntry>(`/datasets/${slug}`);
    metadataBySlug.value.set(slug, body.data ?? null);
  } catch {
    metadataBySlug.value.set(slug, null);
  }
  if ((props.nutsLevel ?? 3) !== 3) {
    // NUTS-0/1/2: /{slug}/timeseries kennt nur Kreis-grain-Codes (siehe
    // Kommentar oben) - stattdessen die auf diese Ebene hochaggregierte
    // Zeitreihe laden. Alle für diese Ebene gelisteten Datensätze sind
    // Kreis-grain/harmonisiert (siehe mart_nuts012_dataset_aggregate), ein
    // Rohdaten-Fallback ist hier also nicht nötig.
    try {
      const body = await apiGet<NutsDatasetAggregatePoint[]>(`/nuts/${props.nutsCode}/dataset-aggregate/${slug}`);
      results.value.set(slug, { kind: "timeseries", points: body.data ?? [] });
    } catch (err) {
      results.value.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
    return;
  }

  // Probiert zuerst den lückenlosen /{slug}/timeseries-Endpoint (einheitliches
  // Schema, Interpolationsmetadaten inklusive); fällt für die wenigen
  // Datensätze ohne Timeseries-Unterstützung (z.B. beschaeftigungsquote-
  // gemeinden) auf die rohe Datensatz-Route zurück.
  try {
    const body = await apiGet<TimeseriesPoint[]>(`/${slug}/timeseries`, {
      geodata_id_nuts: props.nutsCode,
      limit: "null",
    });
    results.value.set(slug, { kind: "timeseries", points: body.data ?? [] });
  } catch {
    try {
      const body = await apiGet<GenericDatasetRow[]>(`/${slug}`, {
        geodata_id_nuts: props.nutsCode,
        limit: "null",
      });
      results.value.set(slug, { kind: "raw", rows: body.data ?? [] });
    } catch (err) {
      results.value.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }
}

function ensureLoaded(slugs: string[]) {
  for (const slug of slugs) {
    if (!results.value.has(slug)) loadSlug(slug);
  }
}

watch(() => props.selectedSlugs, ensureLoaded, { immediate: true });
// Region hat sich geändert (gleiche Auswahl, andere geodata_id_nuts) - alle
// bereits geladenen Ergebnisse sind für die neue Region ungültig.
watch(
  () => props.nutsCode,
  () => {
    results.value = new Map();
    ensureLoaded(props.selectedSlugs);
  },
);

// Nur Datensätze mit Zeitreihen-Unterstützung können in der kombinierten
// Ansicht überlagert werden (siehe drawIndexedChart in TimeseriesChart.vue) -
// die generische Rohdaten-Fallback-Tabelle hat kein einheitliches
// Jahr/Wert-Schema.
const combinableSeries = computed(() =>
  selectedDatasets.value.flatMap((ds) => {
    const result = results.value.get(ds.route_slug);
    if (!result || result.kind !== "timeseries" || !result.points.length) return [];
    return [{ key: ds.route_slug, label: formatDatasetLabel(ds.route_slug), points: result.points }];
  }),
);
const nonCombinableSlugs = computed(() =>
  selectedDatasets.value
    .map((ds) => ds.route_slug)
    .filter((slug) => {
      const result = results.value.get(slug);
      return result && result.kind === "raw";
    }),
);

const activeTabSlug = ref<string | null>(null);
watch(
  selectedDatasets,
  (list) => {
    if (!list.some((ds) => ds.route_slug === activeTabSlug.value)) {
      activeTabSlug.value = list[0]?.route_slug ?? null;
    }
  },
  { immediate: true },
);
</script>

<template>
  <div class="dataset-picker">
    <fieldset class="dataset-multiselect">
      <legend class="visually-hidden">Datensätze auswählen</legend>
      <label v-for="ds in datasets" :key="ds.route_slug" class="dataset-checkbox">
        <input
          type="checkbox"
          :checked="selectedSlugs.includes(ds.route_slug)"
          :disabled="
            (atMax && !selectedSlugs.includes(ds.route_slug)) || (selectedSlugs.length <= 1 && selectedSlugs.includes(ds.route_slug))
          "
          @change="toggleSlug(ds.route_slug, ($event.target as HTMLInputElement).checked)"
        />
        {{ formatDatasetLabel(ds.route_slug) }} ({{ ds.available_years.length }} Jahre)
      </label>
      <p v-if="atMax" class="hint">Maximal {{ MAX_SELECTED }} Datensätze gleichzeitig.</p>
    </fieldset>

    <div v-if="selectedSlugs.length > 1" class="view-mode-toggle" role="radiogroup" aria-label="Ansicht">
      <button
        type="button"
        class="nav-tab"
        :class="{ 'is-active': viewMode === 'separate' }"
        role="radio"
        :aria-checked="viewMode === 'separate'"
        @click="emit('update:viewMode', 'separate')"
      >
        Getrennt
      </button>
      <button
        type="button"
        class="nav-tab"
        :class="{ 'is-active': viewMode === 'combined' }"
        role="radio"
        :aria-checked="viewMode === 'combined'"
        @click="emit('update:viewMode', 'combined')"
      >
        Kombiniert
      </button>
    </div>
  </div>

  <p v-if="!selectedDatasets.length" class="hint">Mindestens einen Datensatz oben auswählen.</p>

  <template v-else-if="viewMode === 'combined' && selectedSlugs.length > 1">
    <p v-if="nonCombinableSlugs.length" class="hint">
      {{ nonCombinableSlugs.map(formatDatasetLabel).join(", ") }} unterstützt keine Zeitreihe und wird nur in der
      Tabelle unten gezeigt, nicht im kombinierten Chart.
    </p>
    <p v-if="!combinableSeries.length" class="hint">Noch keine Zeitreihendaten zum Kombinieren geladen…</p>
    <TimeseriesChart v-else :indexed-series="combinableSeries" />

    <div class="dataset-tabs" role="tablist" aria-label="Datensatz für Tabelle/Info">
      <button
        v-for="ds in selectedDatasets"
        :key="ds.route_slug"
        type="button"
        class="nav-tab"
        role="tab"
        :class="{ 'is-active': activeTabSlug === ds.route_slug }"
        :aria-selected="activeTabSlug === ds.route_slug"
        @click="activeTabSlug = ds.route_slug"
      >
        {{ formatDatasetLabel(ds.route_slug) }}
      </button>
    </div>
    <template v-for="ds in selectedDatasets" :key="ds.route_slug">
      <div v-if="activeTabSlug === ds.route_slug">
        <DatasetInfo v-if="metadataBySlug.get(ds.route_slug)" :metadata="metadataBySlug.get(ds.route_slug)!" />
        <p v-if="results.get(ds.route_slug)?.kind === 'loading'" class="hint">Lade Daten…</p>
        <p v-else-if="results.get(ds.route_slug)?.kind === 'error'" class="error-text">
          Fehler beim Laden: {{ (results.get(ds.route_slug) as { message: string }).message }}
        </p>
        <template v-else-if="results.get(ds.route_slug)?.kind === 'timeseries'">
          <p v-if="!(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points.length" class="hint">
            Keine Werte für diesen Datensatz und diese Region vorhanden.
          </p>
          <DataTable v-else :points="(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points" />
        </template>
        <template v-else-if="results.get(ds.route_slug)?.kind === 'raw'">
          <p v-if="!(results.get(ds.route_slug) as { rows: GenericDatasetRow[] }).rows.length" class="hint">
            Keine Werte für diesen Datensatz und diese Region vorhanden.
          </p>
          <DataTable v-else :rows="(results.get(ds.route_slug) as { rows: GenericDatasetRow[] }).rows" />
        </template>
      </div>
    </template>
  </template>

  <template v-else>
    <section v-for="ds in selectedDatasets" :key="ds.route_slug" class="dataset-block">
      <h3 v-if="selectedDatasets.length > 1" class="dataset-block-title">{{ formatDatasetLabel(ds.route_slug) }}</h3>
      <DatasetInfo v-if="metadataBySlug.get(ds.route_slug)" :metadata="metadataBySlug.get(ds.route_slug)!" />
      <p v-if="results.get(ds.route_slug)?.kind === 'loading'" class="hint">Lade Daten…</p>
      <p v-else-if="results.get(ds.route_slug)?.kind === 'error'" class="error-text">
        Fehler beim Laden: {{ (results.get(ds.route_slug) as { message: string }).message }}
      </p>
      <template v-else-if="results.get(ds.route_slug)?.kind === 'timeseries'">
        <p v-if="!(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points.length" class="hint">
          Keine Werte für diesen Datensatz und diese Region vorhanden.
        </p>
        <template v-else>
          <TimeseriesChart :points="(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points" />
          <DataTable :points="(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points" />
        </template>
      </template>
      <template v-else-if="results.get(ds.route_slug)?.kind === 'raw'">
        <p v-if="!(results.get(ds.route_slug) as { rows: GenericDatasetRow[] }).rows.length" class="hint">
          Keine Werte für diesen Datensatz und diese Region vorhanden.
        </p>
        <DataTable v-else :rows="(results.get(ds.route_slug) as { rows: GenericDatasetRow[] }).rows" />
      </template>
    </section>
  </template>
</template>
