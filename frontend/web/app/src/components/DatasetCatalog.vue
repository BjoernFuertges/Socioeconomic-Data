<script setup lang="ts">
import { computed, nextTick, onActivated, onMounted, ref, watch } from "vue";
import type { RouteLocationRaw } from "vue-router";
import { useRoute, useRouter } from "vue-router";
import { apiGet, API_BASE } from "../api/client";
import { absoluteApiUrl, type DownloadFormat } from "../api/download";
import type { DatasetMetadataEntry, GenericDatasetRow } from "../types/api";
import { formatDateDE, formatNumber } from "../format";
import { AGENTUR_DATASET_SLUG } from "../constants";
import DataTable from "./DataTable.vue";
import ApiSnippet from "./ApiSnippet.vue";

const AGENTUR_DATA_PATH = "/agenturbezirke";

const GEODATA_LEVEL_LABELS: Record<string, string> = {
  NUTS_3: "NUTS-3 (Kreise)",
  LAU: "Gemeinden (LAU)",
  AGENTURBEZIRK: "Agenturbezirke (BA)",
};

function geodataLevelLabel(level: string | null | undefined): string {
  if (!level) return "–";
  return GEODATA_LEVEL_LABELS[level] ?? level;
}

// Nur für den Rohdaten-Link: begrenzt die Anfrage, weil manche Datensätze
// (z.B. beschaeftigungsquote-gemeinden) weit über 100.000 Zeilen haben.
const DATA_LINK_LIMIT = 100;

function dataHref(e: DatasetMetadataEntry): string {
  const path = e.dataset_slug === AGENTUR_DATASET_SLUG ? AGENTUR_DATA_PATH : `/${e.dataset_slug}?limit=${DATA_LINK_LIMIT}`;
  return API_BASE + path;
}

// Pfad des vollständigen Datensatzes - dieselbe Route wie die Vorschau,
// aber ohne limit: limit=null hebt die Obergrenze im Backend auf (siehe
// normalizeNullableLimitQuery in common.go). Der Agenturbezirk-Datensatz hat
// keine generische /{slug}-Route (siehe constants.ts).
function dataPath(e: DatasetMetadataEntry): string {
  return e.dataset_slug === AGENTUR_DATASET_SLUG ? AGENTUR_DATA_PATH : `/${e.dataset_slug}`;
}

// Format je Karte; CSV ist für einen Download der plausiblere Default.
const downloadFormats = ref<Map<string, DownloadFormat>>(new Map());
function downloadFormat(slug: string): DownloadFormat {
  return downloadFormats.value.get(slug) ?? "csv";
}
function setDownloadFormat(slug: string, format: DownloadFormat) {
  downloadFormats.value = new Map(downloadFormats.value).set(slug, format);
}

// Für curl/Skripte: ohne format=, das Format kommt dort über den Accept-Header.
function fullDataApiUrl(e: DatasetMetadataEntry): string {
  return absoluteApiUrl(dataPath(e), { limit: "null" });
}

// Für den Browser-Download: <a href> kann keinen Accept-Header setzen.
function fullDataDownloadUrl(e: DatasetMetadataEntry): string {
  return absoluteApiUrl(dataPath(e), { limit: "null", format: downloadFormat(e.dataset_slug), download: 1 });
}

// Statt den Link direkt auf die rohe JSON-API zeigen zu lassen (unformatiert,
// neuer Tab), zeigt eine aufklappbare Vorschau dieselben Zeilen als Tabelle
// im Katalog selbst - der reine JSON-Link bleibt als kleiner, sekundärer
// "API-Antwort"-Link erhalten (siehe Footer im Template) für alle, die
// tatsächlich die Rohantwort brauchen.
type PreviewState =
  | { kind: "loading" }
  | { kind: "loaded"; rows: GenericDatasetRow[] }
  | { kind: "error"; message: string };
const previews = ref<Map<string, PreviewState>>(new Map());

async function loadPreview(e: DatasetMetadataEntry) {
  if (previews.value.has(e.dataset_slug)) return;
  previews.value.set(e.dataset_slug, { kind: "loading" });
  try {
    const isAgentur = e.dataset_slug === AGENTUR_DATASET_SLUG;
    const body = await apiGet<GenericDatasetRow[]>(
      isAgentur ? AGENTUR_DATA_PATH : `/${e.dataset_slug}`,
      isAgentur ? undefined : { limit: DATA_LINK_LIMIT },
    );
    previews.value.set(e.dataset_slug, { kind: "loaded", rows: body.data ?? [] });
  } catch (err) {
    previews.value.set(e.dataset_slug, {
      kind: "error",
      message: err instanceof Error ? err.message : String(err),
    });
  }
}

function onPreviewToggle(e: DatasetMetadataEntry, evt: Event) {
  if ((evt.target as HTMLDetailsElement).open) loadPreview(e);
}

// Cross-Link "Datenkatalog -> Regionen": springt in die passende Gebietsebene
// und (für die slug-basierten Kreis-/Gemeinden-Datensätze) wählt den
// Datensatz vor, sobald dort eine Region gewählt wird (siehe
// RegionBrowser.vue: nutsDatasetInitialSlug/lauDatasetInitialSlug via
// ?dataset=). Agenturbezirke haben keinen Datensatz-Picker - dort reicht der
// Sprung in den Gebietstyp.
function regionsLinkFor(e: DatasetMetadataEntry): RouteLocationRaw | null {
  if (e.geodata_level === "NUTS_3") return { name: "regionen", params: { regionType: "nuts" }, query: { dataset: e.dataset_slug } };
  if (e.geodata_level === "LAU") return { name: "regionen", params: { regionType: "lau" }, query: { dataset: e.dataset_slug } };
  if (e.geodata_level === "AGENTURBEZIRK") return { name: "regionen", params: { regionType: "agentur" } };
  return null;
}

const route = useRoute();
const router = useRouter();

const entries = ref<DatasetMetadataEntry[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);

// Aus der Route vorbefüllt (Deep-Link/geteilter Link), danach per Debounce
// synchron zur Route gehalten (router.replace statt push, damit Tippen keine
// Chronik-Einträge erzeugt) - macht Suche/Filter teilbar/bookmarkbar.
const search = ref((route.query.q as string) ?? "");
const geodataLevelFilter = ref((route.query.level as string) ?? "");
const licenseFilter = ref((route.query.license as string) ?? "");

let queryTimer: ReturnType<typeof setTimeout> | null = null;
watch([search, geodataLevelFilter, licenseFilter], () => {
  if (queryTimer) clearTimeout(queryTimer);
  queryTimer = setTimeout(() => {
    router.replace({
      name: "katalog",
      params: route.params,
      query: {
        q: search.value || undefined,
        level: geodataLevelFilter.value || undefined,
        license: licenseFilter.value || undefined,
      },
    });
  }, 300);
});

// Slug aus /katalog/:slug (Deep-Link, z.B. von der Karte oder von außen) -
// scrollt zum passenden Katalogeintrag und hebt ihn hervor.
const highlightedSlug = computed(() => (route.params.slug as string | undefined) || null);

async function scrollToHighlighted() {
  if (!highlightedSlug.value) return;
  await nextTick();
  document.getElementById(`catalog-${highlightedSlug.value}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

watch(highlightedSlug, scrollToHighlighted);
onActivated(scrollToHighlighted);

onMounted(async () => {
  loading.value = true;
  error.value = null;
  try {
    const body = await apiGet<DatasetMetadataEntry[]>("/datasets");
    entries.value = body.data ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
  await scrollToHighlighted();
});

const geodataLevels = computed(() => {
  const seen = new Set(entries.value.map((e) => e.geodata_level).filter((v): v is string => !!v));
  return Array.from(seen).sort();
});

const licenses = computed(() => {
  const seen = new Set(entries.value.map((e) => e.license));
  return Array.from(seen).sort();
});

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase();
  return entries.value.filter((e) => {
    if (geodataLevelFilter.value && e.geodata_level !== geodataLevelFilter.value) return false;
    if (licenseFilter.value && e.license !== licenseFilter.value) return false;
    if (!needle) return true;
    return (
      e.dataset_slug.toLowerCase().includes(needle) ||
      (e.name ?? "").toLowerCase().includes(needle) ||
      (e.description ?? "").toLowerCase().includes(needle) ||
      (e.provenance ?? "").toLowerCase().includes(needle)
    );
  });
});

function formatCoverage(e: DatasetMetadataEntry): string {
  if (!e.min_year || !e.max_year) return "–";
  return e.min_year === e.max_year ? `${e.min_year}` : `${e.min_year}–${e.max_year}`;
}

// Für Excel unter deutscher Locale: Semikolon statt Komma als Trenner, sonst
// wird die Datei beim Doppelklick-Öffnen als eine einzige Spalte interpretiert.
const CSV_DELIMITER = ";";

function toCsvField(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return "";
  const str = String(value);
  if (str.includes('"') || str.includes(CSV_DELIMITER) || str.includes("\n") || str.includes("\r")) {
    return `"${str.replace(/"/g, '""')}"`;
  }
  return str;
}

function exportCsv() {
  const header = [
    "Slug",
    "Name",
    "Beschreibung",
    "Herkunft",
    "Geodatenebene",
    "Von",
    "Bis",
    "Datensätze",
    "Lizenz",
    "Lizenz-URL",
    "Aktualisiert",
    "Persistente ID",
  ];
  const rows = filtered.value.map((e) => [
    e.dataset_slug,
    e.name ?? "",
    e.description ?? "",
    e.provenance ?? "",
    e.geodata_level ?? "",
    e.min_year ?? "",
    e.max_year ?? "",
    e.total_records ?? "",
    e.license,
    e.license_url ?? "",
    e.updated_at ?? "",
    e.dataset_uri,
  ]);
  const csv = [header, ...rows].map((row) => row.map(toCsvField).join(CSV_DELIMITER)).join("\r\n");
  // BOM voranstellen, damit Excel die Datei als UTF-8 statt Systemcodepage liest (wichtig für Umlaute).
  const blob = new Blob(["﻿" + csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = "datenkatalog.csv";
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
</script>

<template>
  <div class="dataset-catalog">
    <div class="catalog-filters">
      <label class="visually-hidden" for="catalog-search">Datensatz suchen</label>
      <input
        id="catalog-search"
        v-model="search"
        type="search"
        placeholder="Datensatz suchen (Name, Beschreibung, Herkunft)…"
        autocomplete="off"
      />

      <label class="visually-hidden" for="catalog-level-filter">Nach Geodatenebene filtern</label>
      <select id="catalog-level-filter" v-model="geodataLevelFilter">
        <option value="">Alle Ebenen</option>
        <option v-for="lvl in geodataLevels" :key="lvl" :value="lvl">{{ geodataLevelLabel(lvl) }}</option>
      </select>

      <label class="visually-hidden" for="catalog-license-filter">Nach Lizenz filtern</label>
      <select id="catalog-license-filter" v-model="licenseFilter">
        <option value="">Alle Lizenzen</option>
        <option v-for="lic in licenses" :key="lic" :value="lic">{{ lic }}</option>
      </select>

      <button type="button" class="link-button" :disabled="!filtered.length" @click="exportCsv">
        Als CSV exportieren ({{ filtered.length }})
      </button>
    </div>

    <p v-if="loading" class="hint">Lade Datenkatalog…</p>
    <p v-else-if="error" class="error-text">Fehler beim Laden: {{ error }}</p>
    <p v-else-if="!filtered.length" class="hint">Keine Treffer.</p>

    <ul v-else class="catalog-list">
      <li
        v-for="e in filtered"
        :id="`catalog-${e.dataset_slug}`"
        :key="e.dataset_slug"
        class="catalog-card"
        :class="{ 'catalog-card--highlighted': e.dataset_slug === highlightedSlug }"
      >
        <div class="catalog-card-header">
          <h3>{{ e.name ?? e.dataset_slug }}</h3>
          <span class="badge">{{ geodataLevelLabel(e.geodata_level) }}</span>
        </div>

        <p v-if="e.description" class="catalog-description">{{ e.description }}</p>

        <dl class="catalog-meta">
          <template v-if="e.provenance">
            <dt>Herkunft</dt>
            <dd>{{ e.provenance }}</dd>
          </template>

          <dt>Abdeckung</dt>
          <dd>
            {{ formatCoverage(e) }}<span v-if="e.total_records"> · {{ formatNumber(e.total_records) }} Datensätze</span>
          </dd>

          <dt>Lizenz</dt>
          <dd>
            <a v-if="e.license_url" :href="e.license_url" target="_blank" rel="noopener noreferrer">{{
              e.license
            }}</a>
            <span v-else>{{ e.license }}</span>
            <span v-if="e.license_backfilled" class="hint">
              (für ältere Jahre rückwirkend angenommen, nicht einzeln verifiziert)
            </span>
          </dd>

          <dt>Aktualisiert</dt>
          <dd>{{ e.updated_at ? formatDateDE(e.updated_at) : "–" }}</dd>
        </dl>

        <details v-if="e.description_long" class="catalog-details">
          <summary>Ausführliche Beschreibung</summary>
          <p>{{ e.description_long }}</p>
        </details>

        <details class="catalog-details" @toggle="onPreviewToggle(e, $event)">
          <summary>Datenvorschau</summary>
          <p v-if="previews.get(e.dataset_slug)?.kind === 'loading'" class="hint">Lade…</p>
          <p v-else-if="previews.get(e.dataset_slug)?.kind === 'error'" class="error-text">
            Vorschau konnte nicht geladen werden: {{ (previews.get(e.dataset_slug) as { message: string }).message }}
          </p>
          <template v-else-if="previews.get(e.dataset_slug)?.kind === 'loaded'">
            <p v-if="!(previews.get(e.dataset_slug) as { rows: GenericDatasetRow[] }).rows.length" class="hint">
              Keine Zeilen vorhanden.
            </p>
            <DataTable v-else :rows="(previews.get(e.dataset_slug) as { rows: GenericDatasetRow[] }).rows" />
          </template>
        </details>

        <details class="catalog-details">
          <summary>Datensatz herunterladen (CSV/JSON)</summary>
          <p class="hint">
            Vollständiger Datensatz ({{ e.total_records ? formatNumber(e.total_records) + " Zeilen" : "alle Zeilen" }}),
            nicht nur die Vorschau oben. Für einzelne Regionen mit aggregierten Werten: „In Regionen anzeigen“ unten.
          </p>
          <div class="download-format-choice">
            <label>
              <input
                type="radio"
                :name="`fmt-${e.dataset_slug}`"
                :checked="downloadFormat(e.dataset_slug) === 'csv'"
                @change="setDownloadFormat(e.dataset_slug, 'csv')"
              />
              CSV
            </label>
            <label>
              <input
                type="radio"
                :name="`fmt-${e.dataset_slug}`"
                :checked="downloadFormat(e.dataset_slug) === 'json'"
                @change="setDownloadFormat(e.dataset_slug, 'json')"
              />
              JSON
            </label>
            <a :href="fullDataDownloadUrl(e)" class="link-button" download>
              {{ downloadFormat(e.dataset_slug).toUpperCase() }} herunterladen ↓
            </a>
          </div>
          <ApiSnippet
            :api-url="fullDataApiUrl(e)"
            :download-url="fullDataDownloadUrl(e)"
            :format="downloadFormat(e.dataset_slug)"
            :filename="e.dataset_slug"
          />
        </details>

        <div class="catalog-card-footer">
          <RouterLink v-if="regionsLinkFor(e)" :to="regionsLinkFor(e)!" class="link-button">In Regionen anzeigen →</RouterLink>
          <a :href="dataHref(e)" target="_blank" rel="noopener noreferrer" class="catalog-api-link">API-Antwort (JSON) ↗</a>
          <code>{{ e.dataset_uri }}</code>
        </div>
      </li>
    </ul>
  </div>
</template>
