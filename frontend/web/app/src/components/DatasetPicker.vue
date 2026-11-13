<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { apiGet, apiPost } from "../api/client";
import type {
  AgenturDatasetAggregatePoint,
  DatasetCatalogEntry,
  DatasetMetadataEntry,
  GenericDatasetRow,
  NutsDatasetAggregatePoint,
  TimeseriesPoint,
} from "../types/api";
import type { CustomRegionAggregateRow } from "../types/api";
import type { RegionGeometry } from "../customRegion";
import { formatDatasetLabel, formatMonthLabel } from "../format";
import CustomRegionNote from "./CustomRegionNote.vue";
import TimeseriesChart from "./TimeseriesChart.vue";
import DataTable from "./DataTable.vue";
import DatasetInfo from "./DatasetInfo.vue";
import AgenturAggregationNote from "./AgenturAggregationNote.vue";

// Caps multi-select at the number of validated categorical color slots
// (see SERIES_COLORS in TimeseriesChart.vue) - more series in the combined
// view could no longer be told apart reliably.
const MAX_SELECTED = 4;

type LoadResult =
  | { kind: "loading" }
  | { kind: "timeseries"; points: (TimeseriesPoint & { month?: number })[] }
  | { kind: "raw"; rows: GenericDatasetRow[] }
  | { kind: "error"; message: string };

const props = defineProps<{
  datasets: DatasetCatalogEntry[];
  // Despite the name, for kind="agentur" this is the region's ba_schl, not a NUTS
  // code (see kind below) - prop name left unchanged for compatibility, to keep
  // the diff to the existing NUTS/LAU callers small.
  nutsCode: string;
  selectedSlugs: string[];
  // NUTS level of the current region (0-3). Default 3 (Kreis) - the previous
  // behavior. For 0-2 (Land/Bundesland/Regierungsbezirk) there are no Kreis-grain
  // rows under this code, so loadSlug() uses the endpoint aggregated up to that
  // level instead of /{slug}/timeseries (see RegionBrowser.vue,
  // mart_nuts012_dataset_aggregate). Ignored for kind="agentur".
  nutsLevel?: number;
  // "nuts" (default) loads via /{slug}/timeseries or
  // /nuts/{code}/dataset-aggregate/{slug}. "agentur" always loads via
  // /agenturbezirke/{ba_schl}/dataset-aggregate/{slug} (area-share- instead of
  // population-weighted, see mart_agentur_dataset_aggregate) - there is no
  // Kreis-grain raw data for Agenturbezirke, only the aggregation.
  kind?: "nuts" | "agentur" | "custom";
  // Only kind="custom": the region (GeoJSON Polygon/MultiPolygon) aggregated over via
  // POST /custom-regions/dataset-aggregate. route_slug is then the dataset slug or
  // "slug::indikator" for datasets with several indicators (see
  // CustomRegionView.vue). Monthly series carry `month`.
  geometry?: RegionGeometry;
  // Only kind="agentur": the region's BA monthly series (e.g. Arbeitslose) that may
  // appear in the combined chart next to the yearly Kreis aggregates. Points carry
  // `month` (see IndexedSeries in TimeseriesChart.vue).
  monthlySeries?: { key: string; label: string; points: (TimeseriesPoint & { month: number })[] }[];
}>();

const emit = defineEmits<{
  "update:selectedSlugs": [slugs: string[]];
}>();

// kind="custom": route_slug = "slug" oder "slug::indikator".
function baseSlug(routeSlug: string): string {
  return routeSlug.split("::")[0];
}
function labelOf(routeSlug: string): string {
  const [slug, indicator] = routeSlug.split("::");
  return indicator ? `${formatDatasetLabel(slug)} – ${indicator}` : formatDatasetLabel(slug);
}

// Monthly series (Agenturbezirke, kind="custom" only) have points with `month`.
function isMonthly(points: (TimeseriesPoint & { month?: number })[]): boolean {
  return points.some((p) => p.month != null);
}
function periodSeriesOf(slug: string, points: (TimeseriesPoint & { month?: number })[]) {
  return [{ key: slug, label: labelOf(slug), points: points.map((p) => ({ label: formatMonthLabel(p.year, p.month ?? 1), value: p.value })) }];
}
function periodRowsOf(points: (TimeseriesPoint & { month?: number })[]) {
  return points.map((p) => ({ label: formatMonthLabel(p.year, p.month ?? 1), arbeitslose: p.value, unterbeschaeftigung: null }));
}
const VALUE_COLUMN = [{ key: "arbeitslose" as const, label: "Wert" }];

// Checkbox order follows the order in `datasets`, so selectedDatasets (below)
// has a stable order for chart series/tabs.
// BA monthly series (kind="agentur" only) are part of the combined chart and also
// occupy color slots there - so correspondingly fewer remain for datasets.
const monthlyCount = computed(() => props.monthlySeries?.length ?? 0);
const maxSelected = computed(() => Math.max(1, MAX_SELECTED - monthlyCount.value));
// Agentur: default is "no dataset" (slot then shows the plain BA values),
// NUTS/LAU need at least one dataset.
const minSelected = computed(() => (props.kind === "agentur" ? 0 : 1));
// No manual toggle: as soon as more than one series is involved (second dataset,
// or a dataset next to the BA monthly series), the view automatically shows the
// combined, indexed chart - otherwise the single view.
const isCombined = computed(() => props.selectedSlugs.length > 0 && props.selectedSlugs.length + monthlyCount.value > 1);

// Points of a loaded dataset (for AgenturAggregationNote); empty until loaded.
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
    // At least one dataset must stay selected - otherwise the selection would be
    // "empty" while RegionBrowser.vue (nutsSelectedSlugs/lauSelectedSlugs) and the
    // route still show the last valid state: a contradictory state (checkbox
    // unchecked, but chart/URL still show the old dataset).
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
  // Bound to the Map of this region: a late response after a region switch
  // must not land in the fresh Map.
  const store = results.value;
  store.set(slug, { kind: "loading" });
  try {
    const body = await apiGet<DatasetMetadataEntry>(`/datasets/${baseSlug(slug)}`);
    metadataBySlug.value.set(slug, body.data ?? null);
  } catch {
    metadataBySlug.value.set(slug, null);
  }
  if (props.kind === "custom") {
    try {
      const [base, indicator] = slug.split("::");
      const body = await apiPost<CustomRegionAggregateRow[]>("/custom-regions/dataset-aggregate", {
        geometry: props.geometry,
        datasets: [base],
      });
      const rows = (body.data ?? []).filter((r) => !indicator || r.indicator === indicator);
      store.set(slug, {
        kind: "timeseries",
        points: rows.map((r) => ({
          year: r.year,
          month: r.month,
          value: r.value,
          is_interpolated: false,
          // Exactly matching region with a direct value: the unit's own values,
          // nothing computed (legend: "beobachtet").
          is_computed: !r.matched_unit || !!r.is_crosswalked,
          aggregation_method: r.aggregation_method,
          value_kind: r.value_kind,
          level_used: r.level_used,
          coverage: r.coverage,
          source_region_count: r.source_region_count,
          matched_unit: r.matched_unit,
          is_crosswalked: r.is_crosswalked,
          employee_slug: r.employee_slug,
        })),
      });
    } catch (err) {
      store.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
    return;
  }
  if (props.kind === "agentur") {
    try {
      const body = await apiGet<AgenturDatasetAggregatePoint[]>(`/agenturbezirke/${props.nutsCode}/dataset-aggregate/${slug}`);
      store.set(slug, { kind: "timeseries", points: body.data ?? [] });
    } catch (err) {
      store.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
    return;
  }
  if ((props.nutsLevel ?? 3) !== 3) {
    // NUTS-0/1/2: /{slug}/timeseries only knows Kreis-grain codes (see comment
    // above) - load the time series aggregated up to this level instead. All
    // datasets listed for this level are Kreis-grain/harmonized (see
    // mart_nuts012_dataset_aggregate), so no raw-data fallback is needed here.
    try {
      const body = await apiGet<NutsDatasetAggregatePoint[]>(`/nuts/${props.nutsCode}/dataset-aggregate/${slug}`);
      store.set(slug, { kind: "timeseries", points: body.data ?? [] });
    } catch (err) {
      store.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
    return;
  }

  // First try the gap-free /{slug}/timeseries endpoint (uniform schema, including
  // interpolation metadata); falls back to the raw dataset route for the few
  // datasets without timeseries support (e.g. beschaeftigungsquote-gemeinden).
  try {
    const body = await apiGet<TimeseriesPoint[]>(`/${slug}/timeseries`, {
      geodata_id_nuts: props.nutsCode,
      limit: "null",
    });
    store.set(slug, { kind: "timeseries", points: body.data ?? [] });
  } catch {
    try {
      const body = await apiGet<GenericDatasetRow[]>(`/${slug}`, {
        geodata_id_nuts: props.nutsCode,
        limit: "null",
      });
      store.set(slug, { kind: "raw", rows: body.data ?? [] });
    } catch (err) {
      store.set(slug, { kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }
}

function ensureLoaded(slugs: string[]) {
  for (const slug of slugs) {
    if (!results.value.has(slug)) loadSlug(slug);
  }
}

watch(() => props.selectedSlugs, ensureLoaded, { immediate: true });
// Region changed (same selection, different geodata_id_nuts) - all already
// loaded results are invalid for the new region.
watch(
  () => props.nutsCode,
  () => {
    results.value = new Map();
    ensureLoaded(props.selectedSlugs);
  },
);

// Only datasets with timeseries support can be overlaid in the combined view
// (see drawIndexedChart in TimeseriesChart.vue) - the generic raw-data fallback
// table has no uniform year/value schema.
const combinableSeries = computed(() =>
  selectedDatasets.value.flatMap((ds) => {
    const result = results.value.get(ds.route_slug);
    if (!result || result.kind !== "timeseries" || !result.points.length) return [];
    return [{ key: ds.route_slug, label: labelOf(ds.route_slug), points: result.points }];
  }),
);
// Monthly series first; capped to the validated color slots.
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

// Agentur: the BA monthly values (table/info come via the "ba-tab" slot from the
// caller) are a regular tab next to the Kreis datasets and the first/active one
// by default.
const BA_TAB = "__ba";
const hasBaTab = computed(() => props.kind === "agentur" && monthlyCount.value > 0);

const activeTabSlug = ref<string | null>(null);
watch(
  [selectedDatasets, hasBaTab],
  ([list, baTab]) => {
    const valid = (baTab && activeTabSlug.value === BA_TAB) || list.some((ds) => ds.route_slug === activeTabSlug.value);
    if (!valid) activeTabSlug.value = baTab ? BA_TAB : (list[0]?.route_slug ?? null);
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
        {{ labelOf(ds.route_slug) }}<template v-if="ds.available_years.length"> ({{ ds.available_years.length }} Jahre)</template>
      </label>
      <p v-if="atMax" class="hint">Maximal {{ maxSelected }} Datensätze gleichzeitig.</p>
    </fieldset>
  </div>

  <!-- Agentur without dataset: plain BA view (actual values) from the caller's slot. -->
  <slot v-if="!selectedDatasets.length && kind === 'agentur'" />
  <p v-else-if="!selectedDatasets.length" class="hint">Mindestens einen Datensatz oben auswählen.</p>

  <template v-else-if="isCombined">
    <p v-if="nonCombinableSlugs.length" class="hint">
      {{ nonCombinableSlugs.map(labelOf).join(", ") }} unterstützt keine Zeitreihe und wird nur in der
      Tabelle unten gezeigt, nicht im kombinierten Chart.
    </p>
    <p v-if="chartTruncated" class="hint">Es werden höchstens {{ MAX_SELECTED }} Reihen gleichzeitig gezeigt.</p>
    <p v-if="!chartSeries.length" class="hint">Noch keine Zeitreihendaten zum Kombinieren geladen…</p>
    <TimeseriesChart v-else :indexed-series="chartSeries" />

    <div class="dataset-tabs" role="tablist" aria-label="Datensatz für Tabelle/Info">
      <button
        v-if="hasBaTab"
        type="button"
        class="nav-tab"
        role="tab"
        :class="{ 'is-active': activeTabSlug === BA_TAB }"
        :aria-selected="activeTabSlug === BA_TAB"
        @click="activeTabSlug = BA_TAB"
      >
        BA-Monatswerte
      </button>
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
        {{ labelOf(ds.route_slug) }}
      </button>
    </div>
    <div v-if="hasBaTab && activeTabSlug === BA_TAB">
      <slot name="ba-tab" />
    </div>
    <template v-for="ds in selectedDatasets" :key="ds.route_slug">
      <div v-if="activeTabSlug === ds.route_slug">
        <DatasetInfo v-if="metadataBySlug.get(ds.route_slug)" :metadata="metadataBySlug.get(ds.route_slug)!" />
        <AgenturAggregationNote v-if="kind === 'agentur'" :points="pointsOf(ds.route_slug)" :dataset-slug="baseSlug(ds.route_slug)" />
        <CustomRegionNote v-else-if="kind === 'custom'" :points="pointsOf(ds.route_slug) as any" />
        <p v-if="results.get(ds.route_slug)?.kind === 'loading'" class="hint">Lade Daten…</p>
        <p v-else-if="results.get(ds.route_slug)?.kind === 'error'" class="error-text">
          Fehler beim Laden: {{ (results.get(ds.route_slug) as { message: string }).message }}
        </p>
        <template v-else-if="results.get(ds.route_slug)?.kind === 'timeseries'">
          <p v-if="!(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points.length" class="hint">
            Keine Werte für diesen Datensatz und diese Region vorhanden.
          </p>
          <DataTable
            v-else-if="isMonthly((results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points)"
            :period-rows="periodRowsOf((results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points)"
            :period-columns="VALUE_COLUMN"
          />
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
      <h3 v-if="selectedDatasets.length > 1" class="dataset-block-title">{{ labelOf(ds.route_slug) }}</h3>
      <DatasetInfo v-if="metadataBySlug.get(ds.route_slug)" :metadata="metadataBySlug.get(ds.route_slug)!" />
      <AgenturAggregationNote v-if="kind === 'agentur'" :points="pointsOf(ds.route_slug)" :dataset-slug="baseSlug(ds.route_slug)" />
        <CustomRegionNote v-else-if="kind === 'custom'" :points="pointsOf(ds.route_slug) as any" />
      <p v-if="results.get(ds.route_slug)?.kind === 'loading'" class="hint">Lade Daten…</p>
      <p v-else-if="results.get(ds.route_slug)?.kind === 'error'" class="error-text">
        Fehler beim Laden: {{ (results.get(ds.route_slug) as { message: string }).message }}
      </p>
      <template v-else-if="results.get(ds.route_slug)?.kind === 'timeseries'">
        <p v-if="!(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points.length" class="hint">
          Keine Werte für diesen Datensatz und diese Region vorhanden.
        </p>
        <template v-else>
          <template v-if="isMonthly((results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points)">
            <TimeseriesChart :period-series="periodSeriesOf(ds.route_slug, (results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points)" />
            <DataTable
              :period-rows="periodRowsOf((results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points)"
              :period-columns="VALUE_COLUMN"
            />
          </template>
          <template v-else>
            <TimeseriesChart :points="(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points" />
            <DataTable :points="(results.get(ds.route_slug) as { points: TimeseriesPoint[] }).points" />
          </template>
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
