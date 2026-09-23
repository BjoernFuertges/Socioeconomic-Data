<script setup lang="ts">
// Download-Dialog für eine Region: stellt Datensätze, Wertearten (roh/
// aggregiert), Format und Jahresbereich zusammen und lädt sie über den
// kombinierten Endpoint GET /regions/{type}/{id}/data. Derselbe Request wird
// darunter als curl-Befehl/Link angezeigt (ApiSnippet), damit der Download
// auch außerhalb der UI reproduzierbar ist.
import { computed, ref, watch } from "vue";
import type { DatasetCatalogEntry } from "../types/api";
import {
  regionApiUrl,
  regionDownloadUrl,
  regionFilename,
  type DownloadFormat,
  type DownloadScope,
  type RegionType,
} from "../api/download";
import { formatDatasetLabel } from "../format";
import ApiSnippet from "./ApiSnippet.vue";

// Zusätzliche, nicht katalogbasierte Wertereihen einer Region (BA-Monatswerte
// eines Agenturbezirks, Gemeinde-Rollups eines Kreises) - im Long-Format des
// Exports tragen sie eigene dataset_slugs, siehe region_data.go.
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
  // Im Dialog vorausgewählt: was die Region gerade anzeigt.
  preselectedSlugs: string[];
  extraOptions?: ExtraDownloadOption[];
  // Welche Wertearten es für diese Region überhaupt gibt (NUTS 0-2 haben
  // keine Rohwerte, LAU keine aggregierten).
  availableScopes: Exclude<DownloadScope, "both">[];
}>();

const emit = defineEmits<{ close: [] }>();

const dialogRef = ref<HTMLDialogElement | null>(null);
const selectedSlugs = ref<string[]>([]);
const scope = ref<DownloadScope>("both");
const format = ref<DownloadFormat>("csv");
const fromYear = ref<string>("");
const toYear = ref<string>("");

// Alle wählbaren Einträge: Katalog-Datensätze plus die region-spezifischen
// Sonderreihen (jeweils mit der Werteart, unter der sie geliefert werden).
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

// Beim Öffnen: Auswahl der Region übernehmen, Scope auf das Sinnvollste setzen.
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

// Leere Auswahl heißt serverseitig "alle verfügbaren" - genau das ist hier
// die Bedeutung von "Alle Datensätze".
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

// Datensätze, die zur gewählten Werteart nichts beitragen - ohne Hinweis
// bekäme man sonst kommentarlos eine leere Datei.
const ignoredSelection = computed(() => {
  if (scope.value === "both" || !selectedSlugs.value.length) return [];
  return options.value.filter((o) => selectedSlugs.value.includes(o.slug) && o.scope !== scope.value);
});
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

    <p v-if="ignoredSelection.length" class="hint">
      Für die gewählte Werteart liefern folgende Einträge nichts:
      {{ ignoredSelection.map((o) => o.label).join(", ") }}.
    </p>

    <div class="download-dialog-actions">
      <a :href="downloadUrl" class="link-button" download>{{ format.toUpperCase() }} herunterladen ↓</a>
    </div>

    <details class="catalog-details">
      <summary>Dieselben Daten per API (curl/Link)</summary>
      <p class="hint">
        Bei dem curl-Befehl wird das Format über den Accept-Header aus während der Link stattdessen
        <code>format=</code> enthält, weil in dem Link für den Browser kein Header gesetzt werden kann.
      </p>
      <ApiSnippet :api-url="apiUrl" :download-url="downloadUrl" :format="format" :filename="filename" />
      <p class="hint">
        Feldbeschreibung und alle Parameter:
        <a href="/api/docs/index.html" target="_blank" rel="noopener noreferrer">API-Dokumentation ↗</a>
      </p>
    </details>
  </dialog>
</template>
