<script setup lang="ts">
import { ref, watch } from "vue";
import { apiGet } from "../api/client";
import type {
  DatasetCatalogEntry,
  DatasetMetadataEntry,
  GenericDatasetRow,
  NutsTimeseriesPoint,
} from "../types/api";
import { formatDatasetLabel } from "../format";
import TimeseriesChart from "./TimeseriesChart.vue";
import DataTable from "./DataTable.vue";
import DatasetInfo from "./DatasetInfo.vue";

const props = defineProps<{
  datasets: DatasetCatalogEntry[];
  nutsCode: string;
  // Slug, der beim ersten Anzeigen vorausgewählt sein soll (z.B. beim Sprung
  // aus dem Agenturbezirk-Panel zum passenden INKAR-Datensatz), statt des
  // sonst üblichen ersten Eintrags. Ignoriert, falls kein Datensatz mit
  // diesem Slug verfügbar ist.
  initialSlug?: string;
}>();

const initialDataset = props.datasets.find((ds) => ds.route_slug === props.initialSlug);
const selectedSlug = ref(initialDataset?.route_slug ?? props.datasets[0]?.route_slug ?? "");
const loading = ref(false);
const error = ref<string | null>(null);
const result = ref<{ kind: "timeseries"; points: NutsTimeseriesPoint[] } | { kind: "raw"; rows: GenericDatasetRow[] } | null>(
  null,
);

// Herkunft/Lizenz/Beschreibung - separat vom Zeitreihen-/Rohdaten-Request geladen, damit ein 404 im Katalog die eigentliche Datenanzeige nicht blockiert.
const metadata = ref<DatasetMetadataEntry | null>(null);

async function loadMetadata() {
  const slug = selectedSlug.value;
  metadata.value = null;
  if (!slug) return;
  try {
    const body = await apiGet<DatasetMetadataEntry>(`/datasets/${slug}`);
    metadata.value = body.data ?? null;
  } catch {
    // Kein Katalogeintrag für diesen Slug
  }
}

// Probiert zuerst den lückenlosen /{slug}/timeseries-Endpoint (einheitliches
// Schema, Interpolationsmetadaten inklusive); fällt für die wenigen
// Datensätze ohne Timeseries-Unterstützung (z.B. beschaeftigungsquote-
// gemeinden) auf die rohe Datensatz-Route zurück.
async function load() {
  const slug = selectedSlug.value;
  if (!slug) return;
  loading.value = true;
  error.value = null;
  result.value = null;
  try {
    const body = await apiGet<NutsTimeseriesPoint[]>(`/${slug}/timeseries`, {
      geodata_id_nuts: props.nutsCode,
      limit: "null",
    });
    result.value = { kind: "timeseries", points: body.data ?? [] };
  } catch {
    try {
      const body = await apiGet<GenericDatasetRow[]>(`/${slug}`, {
        geodata_id_nuts: props.nutsCode,
        limit: "null",
      });
      result.value = { kind: "raw", rows: body.data ?? [] };
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err);
    }
  } finally {
    loading.value = false;
  }
}

watch(selectedSlug, load, { immediate: true });
watch(selectedSlug, loadMetadata, { immediate: true });
watch(
  () => props.nutsCode,
  () => load(),
);
</script>

<template>
  <div class="dataset-picker">
    <label for="dataset-select">Datensatz</label>
    <select id="dataset-select" v-model="selectedSlug">
      <option v-for="ds in datasets" :key="ds.route_slug" :value="ds.route_slug">
        {{ formatDatasetLabel(ds.route_slug) }} ({{ ds.available_years.length }} Jahre)
      </option>
    </select>
  </div>

  <DatasetInfo v-if="metadata" :metadata="metadata" />

  <div>
    <p v-if="loading" class="hint">Lade Daten…</p>
    <p v-else-if="error" class="error-text">Fehler beim Laden: {{ error }}</p>
    <template v-else-if="result">
      <p v-if="result.kind === 'timeseries' && !result.points.length" class="hint">
        Keine Werte für diesen Datensatz und diese Region vorhanden.
      </p>
      <template v-else-if="result.kind === 'timeseries'">
        <TimeseriesChart :points="result.points" />
        <DataTable :points="result.points" />
      </template>
      <p v-else-if="result.kind === 'raw' && !result.rows.length" class="hint">
        Keine Werte für diesen Datensatz und diese Region vorhanden.
      </p>
      <DataTable v-else-if="result.kind === 'raw'" :rows="result.rows" />
    </template>
  </div>
</template>
