<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { apiGet, API_BASE } from "../api/client";
import type { DatasetMetadataEntry } from "../types/api";
import { formatDateDE, formatNumber } from "../format";

// Der Rohdaten-Endpunkt für Agenturbezirke heißt anders als der Katalog-Slug
// (kein generischer /{slug}-Route wie bei den übrigen, meta.yaml-basierten
// Kreis-/Gemeinden-Datensätzen, siehe load_arbeitslosenzahlen_afa.py /
// RegisterSoziodataRoutes) - deshalb hier einmalig hart verdrahtet statt
// generisch aus dem Slug abgeleitet.
const AGENTUR_SLUG = "arbeitslosenzahlen-afa-agenturbezirke";
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
  const path = e.dataset_slug === AGENTUR_SLUG ? AGENTUR_DATA_PATH : `/${e.dataset_slug}?limit=${DATA_LINK_LIMIT}`;
  return API_BASE + path;
}

const entries = ref<DatasetMetadataEntry[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);

const search = ref("");
const geodataLevelFilter = ref("");
const licenseFilter = ref("");

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
      <li v-for="e in filtered" :key="e.dataset_slug" class="catalog-card">
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

        <div class="catalog-card-footer">
          <a :href="dataHref(e)" target="_blank" rel="noopener noreferrer">Rohdaten (JSON) ↗</a>
          <code>{{ e.dataset_uri }}</code>
        </div>
      </li>
    </ul>
  </div>
</template>
