<script setup lang="ts">
// Download dialog for a region: assembles datasets, value kinds (raw/aggregated),
// format and year range and fetches them via the combined endpoint
// GET /regions/{type}/{id}/data. The same request is shown below as a
// curl command/link (ApiSnippet), so the download is reproducible outside the UI.
import { computed, ref, watch } from "vue";
import type { DatasetCatalogEntry } from "../types/api";
import {
  datasetMetadataApiUrl,
  datasetMetadataFilename,
  datasetMetadataUrl,
  regionApiUrl,
  regionDownloadUrl,
  regionFilename,
  triggerDownloads,
  type DownloadFormat,
  type DownloadScope,
  type RegionType,
} from "../api/download";
import { formatDatasetLabel } from "../format";
import { hasSourceData } from "../constants";
import ApiSnippet from "./ApiSnippet.vue";

// Additional, non-catalog series of a region (BA monthly values of an Agenturbezirk,
// Gemeinde rollups of a Kreis) - in the export's long format they carry their own
// dataset_slugs, see region_data.go.
export interface ExtraDownloadOption {
  slug: string;
  label: string;
  scope: Exclude<DownloadScope, "both">;
}

const props = defineProps<{
  open: boolean;
  regionType: RegionType;
  regionId: string;
  regionLabel: string;
  datasets: DatasetCatalogEntry[];
  // Preselected in the dialog: what the region currently displays.
  preselectedSlugs: string[];
  extraOptions?: ExtraDownloadOption[];
  // Which value kinds exist for this region at all (NUTS 0-2 have no raw
  // values, LAU no aggregated ones).
  availableScopes: Exclude<DownloadScope, "both">[];
}>();

const emit = defineEmits<{ close: [] }>();

const dialogRef = ref<HTMLDialogElement | null>(null);
const selectedSlugs = ref<string[]>([]);
const scope = ref<DownloadScope>("both");
const format = ref<DownloadFormat>("csv");
const fromYear = ref<string>("");
const toYear = ref<string>("");
const includeMetadata = ref(true);

// All selectable entries: catalog datasets plus the region-specific special
// series (each with the value kind under which they are delivered).
const options = computed(() => [
  ...(props.extraOptions ?? []).map((o) => ({ slug: o.slug, label: o.label, scope: o.scope })),
  ...props.datasets.map((d) => ({
    slug: d.route_slug,
    label: formatDatasetLabel(d.route_slug),
    scope: (props.availableScopes.includes("raw") && props.regionType !== "agentur"
      ? "raw"
      : "aggregated") as Exclude<DownloadScope, "both">,
  })),
]);

const scopeOptions = computed(() =>
  [
    { value: "raw" as const, label: "Nicht aggregiert (Ursprungswerte)" },
    { value: "aggregated" as const, label: "Aggregiert (berechnet)" },
    { value: "both" as const, label: "Beides in einer Datei" },
  ].filter((o) => (o.value === "both" ? props.availableScopes.length > 1 : props.availableScopes.includes(o.value))),
);

// On open: adopt the region's selection, set scope to the most sensible value.
watch(
  () => props.open,
  (open) => {
    if (open) {
      selectedSlugs.value = props.preselectedSlugs.filter((s) => options.value.some((o) => o.slug === s));
      scope.value = props.availableScopes.length > 1 ? "both" : props.availableScopes[0];
      dialogRef.value?.showModal();
    } else {
      dialogRef.value?.close();
    }
  },
);

function toggleSlug(slug: string) {
  selectedSlugs.value = selectedSlugs.value.includes(slug)
    ? selectedSlugs.value.filter((s) => s !== slug)
    : [...selectedSlugs.value, slug];
}

const allSelected = computed(() => selectedSlugs.value.length === 0);

// An empty selection means "all available" server-side - which is exactly what
// "Alle Datensätze" means here.
function selectAll() {
  selectedSlugs.value = [];
}

function parseYear(value: string): number | null {
  const year = Number.parseInt(value, 10);
  return Number.isFinite(year) ? year : null;
}

const query = computed(() => ({
  regionType: props.regionType,
  regionId: props.regionId,
  datasets: selectedSlugs.value,
  scope: scope.value,
  fromYear: parseYear(fromYear.value),
  toYear: parseYear(toYear.value),
}));

const filename = computed(() => regionFilename(query.value));
const downloadUrl = computed(() => regionDownloadUrl(query.value, format.value));
const apiUrl = computed(() => regionApiUrl(query.value));

// Datasets that contribute nothing to the chosen value kind - without a hint
// the user would silently get an empty file.
const ignoredSelection = computed(() => {
  if (scope.value === "both" || !selectedSlugs.value.length) return [];
  return options.value.filter((o) => selectedSlugs.value.includes(o.slug) && o.scope !== scope.value);
});

// "All available datasets" means "empty selection" server-side for /regions/.../data,
// but GET /datasets doesn't know that meaning - so resolve it to the concrete
// slug list here before building the metadata URL.
const metadataSlugs = computed(() => (allSelected.value ? options.value.map((o) => o.slug) : selectedSlugs.value));
const metadataApiUrl = computed(() => datasetMetadataApiUrl(metadataSlugs.value));
const metadataDownloadUrl = computed(() => datasetMetadataUrl(metadataSlugs.value, format.value));
const metadataFilename = computed(() => datasetMetadataFilename(filename.value));

// Catalog cards with the unchanged source data for the current selection
// (/katalog/:slug scrolls to the card and highlights it). For "all" that would be
// too many links - jumping to the catalog as a whole is enough.
const sourceLinkOptions = computed(() =>
  options.value.filter((o) => selectedSlugs.value.includes(o.slug) && hasSourceData(o.slug)),
);

// One click triggers the data and (if selected) metadata download at the same time.
function startDownload() {
  const urls = [downloadUrl.value];
  if (includeMetadata.value && metadataSlugs.value.length) urls.push(metadataDownloadUrl.value);
  triggerDownloads(urls);
}
</script>

<template>
  <dialog ref="dialogRef" class="download-dialog" @close="emit('close')">
    <form method="dialog" class="download-dialog-header">
      <h3>Daten herunterladen · {{ props.regionLabel }}</h3>
      <button type="submit" class="link-button" aria-label="Schließen">✕</button>
    </form>

    <fieldset>
      <legend>Datensätze</legend>
      <label class="download-option">
        <input type="radio" :checked="allSelected" @change="selectAll" />
        Alle verfügbaren Datensätze
      </label>
      <label v-for="o in options" :key="o.slug" class="download-option">
        <input type="checkbox" :checked="selectedSlugs.includes(o.slug)" @change="toggleSlug(o.slug)" />
        {{ o.label }}
      </label>
    </fieldset>

    <fieldset v-if="scopeOptions.length > 1">
      <legend>Wertearten</legend>
      <label v-for="o in scopeOptions" :key="o.value" class="download-option">
        <input v-model="scope" type="radio" :value="o.value" />
        {{ o.label }}
      </label>
    </fieldset>

    <fieldset>
      <legend>Format</legend>
      <label class="download-option"><input v-model="format" type="radio" value="csv" /> CSV</label>
      <label class="download-option"><input v-model="format" type="radio" value="json" /> JSON</label>
    </fieldset>

    <fieldset class="download-years">
      <legend>Zeitraum (optional)</legend>
      <label>von <input v-model="fromYear" type="number" min="1990" max="2100" placeholder="z.B. 2010" /></label>
      <label>bis <input v-model="toYear" type="number" min="1990" max="2100" placeholder="z.B. 2024" /></label>
    </fieldset>

    <label class="download-option">
      <input v-model="includeMetadata" type="checkbox" />
      Metadaten (Beschreibung, Herkunft, Lizenz) separat mit exportieren
    </label>

    <p v-if="ignoredSelection.length" class="hint">
      Für die gewählte Werteart liefern folgende Einträge nichts:
      {{ ignoredSelection.map((o) => o.label).join(", ") }}.
    </p>

    <div class="download-dialog-actions">
      <button type="button" class="link-button" @click="startDownload">
        {{ format.toUpperCase() }} herunterladen ↓
      </button>
    </div>

    <p class="hint">
      Die Werte hier sind auf aktuelle Gebietsstände harmonisiert und interpoliert. Die unveränderten Quelldaten
      je Datensatz gibt es im Datenkatalog:
      <template v-if="sourceLinkOptions.length">
        <template v-for="(o, i) in sourceLinkOptions" :key="o.slug">
          <RouterLink :to="{ name: 'katalog', params: { slug: o.slug } }" @click="emit('close')">{{ o.label }}</RouterLink
          ><template v-if="i < sourceLinkOptions.length - 1">, </template>
        </template>
      </template>
      <RouterLink v-else :to="{ name: 'katalog' }" @click="emit('close')">zum Datenkatalog</RouterLink>.
    </p>

    <details class="catalog-details">
      <summary>Dieselben Daten per API (curl/Link)</summary>
      <p class="hint">
        Bei dem curl-Befehl wird das Format über den Accept-Header aus während der Link stattdessen
        <code>format=</code> enthält, weil in dem Link für den Browser kein Header gesetzt werden kann.
      </p>
      <ApiSnippet :api-url="apiUrl" :download-url="downloadUrl" :format="format" :filename="filename" />
      <p v-if="metadataSlugs.length" class="hint">Metadaten zu den ausgewählten Datensätzen:</p>
      <ApiSnippet
        v-if="metadataSlugs.length"
        :api-url="metadataApiUrl"
        :download-url="metadataDownloadUrl"
        :format="format"
        :filename="metadataFilename"
      />
      <p class="hint">
        Feldbeschreibung und alle Parameter:
        <a href="/api/docs/index.html" target="_blank" rel="noopener noreferrer">API-Dokumentation ↗</a>
      </p>
    </details>
  </dialog>
</template>
