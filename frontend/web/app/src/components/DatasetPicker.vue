<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { apiGet } from "../api/client";
import type {
  AgenturDatasetAggregatePoint,
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
import AgenturAggregationNote from "./AgenturAggregationNote.vue";

// Deckelt die Mehrfachauswahl auf die Anzahl validierter kategorialer
// Farbslots (siehe SERIES_COLORS in TimeseriesChart.vue) - mehr Serien in
// der kombinierten Ansicht würden nicht mehr sicher unterscheidbar bleiben.
const MAX_SELECTED = 4;

type LoadResult =
  | { kind: "loading" }
  | { kind: "timeseries"; points: TimeseriesPoint[] }
  | { kind: "raw"; rows: GenericDatasetRow[] }
  | { kind: "error"; message: string };

const props = defineProps<{
  datasets: DatasetCatalogEntry[];
  // Trotz des Namens bei kind="agentur" der ba_schl der Region, nicht ein
  // NUTS-Code (siehe kind unten) - Prop-Name aus Kompatibilitätsgründen
  // unverändert gelassen, um den Diff zu den bestehenden NUTS/LAU-Aufrufen
  // klein zu halten.
  nutsCode: string;
  selectedSlugs: string[];
  // NUTS-Ebene der aktuellen Region (0-3). Default 3 (Kreis) - bisheriges
  // Verhalten. Bei 0-2 (Land/Bundesland/Regierungsbezirk) gibt es keine
  // Kreis-grain-Zeilen unter diesem Code, deshalb lädt loadSlug() dann über
  // den auf diese Ebene hochaggregierten Endpoint statt /{slug}/timeseries
  // (siehe RegionBrowser.vue, mart_nuts012_dataset_aggregate). Ignoriert,
  // wenn kind="agentur".
  nutsLevel?: number;
  // "nuts" (Standard) lädt wie bisher über /{slug}/timeseries bzw.
  // /nuts/{code}/dataset-aggregate/{slug}. "agentur" lädt stattdessen immer
  // über /agenturbezirke/{ba_schl}/dataset-aggregate/{slug} (flächenanteils-
  // statt bevölkerungsgewichtet, siehe mart_agentur_dataset_aggregate) - es
  // gibt für Agenturbezirke keine Kreis-grain-Rohdaten, nur die Aggregation.
  kind?: "nuts" | "agentur";
  // Nur kind="agentur": BA-Monatsreihen der Region (z.B. Arbeitslose), die im
  // kombinierten Diagramm neben den jährlichen Kreis-Aggregaten erscheinen
  // können. Punkte tragen `month` (siehe IndexedSeries in TimeseriesChart.vue).
  monthlySeries?: { key: string; label: string; points: (TimeseriesPoint & { month: number })[] }[];
}>();

const emit = defineEmits<{
  "update:selectedSlugs": [slugs: string[]];
}>();

// Reihenfolge der Checkboxen folgt der Reihenfolge in `datasets`, damit
// selectedDatasets (unten) für Chart-Serien/Tabs eine stabile Reihenfolge hat.
// BA-Monatsreihen (nur kind="agentur") laufen im kombinierten Diagramm mit und
// belegen dort ebenfalls Farbslots - deshalb bleiben für Datensätze entsprechend weniger übrig.
const monthlyCount = computed(() => props.monthlySeries?.length ?? 0);
const maxSelected = computed(() => Math.max(1, MAX_SELECTED - monthlyCount.value));
// Agentur: Standard ist "kein Datensatz" (Slot zeigt dann die reinen BA-Werte),
// NUTS/LAU brauchen mindestens einen Datensatz.
const minSelected = computed(() => (props.kind === "agentur" ? 0 : 1));
// Keine manuelle Umschaltung: sobald mehr als eine Reihe im Spiel ist (zweiter
// Datensatz, oder ein Datensatz neben den BA-Monatsreihen), zeigt die Ansicht
// automatisch das kombinierte, indexierte Diagramm - sonst die Einzelansicht.
const isCombined = computed(() => props.selectedSlugs.length > 0 && props.selectedSlugs.length + monthlyCount.value > 1);

// Punkte eines geladenen Datensatzes (für AgenturAggregationNote); leer, solange nicht geladen.
function pointsOf(slug: string): { aggregation_method?: string; value_kind?: string }[] {
  const result = results.value.get(slug);
  return result?.kind === "timeseries" ? (result.points as AgenturDatasetAggregatePoint[]) : [];
}

const selectedDatasets = computed(() => props.datasets.filter((ds) => props.selectedSlugs.includes(ds.route_slug)));
const atMax = computed(() => props.selectedSlugs.length >= maxSelected.value);

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
    if (props.selectedSlugs.length <= minSelected.value) return;
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
  if (props.kind === "agentur") {
    try {
      const body = await apiGet<AgenturDatasetAggregatePoint[]>(`/agenturbezirke/${props.nutsCode}/dataset-aggregate/${slug}`);
      results.value.set(slug, { kind: "timeseries", points: body.data ?? [] });
    } catch (err) {
      results.value.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
    return;
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
// Monatsreihen zuerst; auf die validierten Farbslots gekappt.
const chartSeries = computed(() => [...(props.monthlySeries ?? []), ...combinableSeries.value].slice(0, MAX_SELECTED));
const chartTruncated = computed(() => monthlyCount.value + combinableSeries.value.length > MAX_SELECTED);
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
            (atMax && !selectedSlugs.includes(ds.route_slug)) || (selectedSlugs.length <= minSelected && selectedSlugs.includes(ds.route_slug))
          "
          @change="toggleSlug(ds.route_slug, ($event.target as HTMLInputElement).checked)"
        />
        {{ formatDatasetLabel(ds.route_slug) }} ({{ ds.available_years.length }} Jahre)
      </label>
      <p v-if="atMax" class="hint">Maximal {{ maxSelected }} Datensätze gleichzeitig.</p>
    </fieldset>
  </div>

  <!-- Agentur ohne Datensatz: reine BA-Ansicht (echte Werte) aus dem Slot des Aufrufers. -->
  <slot v-if="!selectedDatasets.length && kind === 'agentur'" />
  <p v-else-if="!selectedDatasets.length" class="hint">Mindestens einen Datensatz oben auswählen.</p>

  <template v-else-if="isCombined">
    <p v-if="nonCombinableSlugs.length" class="hint">
      {{ nonCombinableSlugs.map(formatDatasetLabel).join(", ") }} unterstützt keine Zeitreihe und wird nur in der
      Tabelle unten gezeigt, nicht im kombinierten Chart.
    </p>
    <p v-if="chartTruncated" class="hint">Es werden höchstens {{ MAX_SELECTED }} Reihen gleichzeitig gezeigt.</p>
    <p v-if="!chartSeries.length" class="hint">Noch keine Zeitreihendaten zum Kombinieren geladen…</p>
    <TimeseriesChart v-else :indexed-series="chartSeries" />

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
        <AgenturAggregationNote v-if="kind === 'agentur'" :points="pointsOf(ds.route_slug)" />
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
      <AgenturAggregationNote v-if="kind === 'agentur'" :points="pointsOf(ds.route_slug)" />
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
