<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { apiGet, unwrapNull } from "../api/client";
import type { DatasetCatalogEntry, DatasetCatalogResponse, LauRegion } from "../types/api";
import { formatNumber } from "../format";
import RegionList from "./RegionList.vue";
import DatasetPicker from "./DatasetPicker.vue";

const search = ref("");
const searching = ref(false);
const searchError = ref<string | null>(null);
const results = ref<LauRegion[]>([]);

const selectedGiscoId = ref<string | null>(null);
const selectedRegion = ref<LauRegion | null>(null);
const detailError = ref<string | null>(null);
const datasets = ref<DatasetCatalogEntry[] | null>(null);
const datasetsError = ref<string | null>(null);

// /lau liefert eine Zeile pro (gisco_id, year); für die Such-UI reicht ein
// Eintrag pro Region – das jeweils jüngste Jahr behalten.
function dedupeByGiscoId(rows: LauRegion[]): LauRegion[] {
  const byId = new Map<string, LauRegion>();
  for (const row of rows) {
    const existing = byId.get(row.gisco_id);
    if (!existing || row.year > existing.year) byId.set(row.gisco_id, row);
  }
  return Array.from(byId.values());
}

let searchTimer: ReturnType<typeof setTimeout> | null = null;
watch(search, (query) => {
  if (searchTimer) clearTimeout(searchTimer);
  const trimmed = query.trim();
  if (trimmed.length < 2) {
    results.value = [];
    searchError.value = null;
    return;
  }
  searchTimer = setTimeout(() => runSearch(trimmed), 350);
});

async function runSearch(query: string) {
  searching.value = true;
  searchError.value = null;
  try {
    const body = await apiGet<LauRegion[]>("/lau", { geodata_name_contains: query, limit: 20 });
    results.value = dedupeByGiscoId(body.data ?? []);
  } catch (err) {
    searchError.value = err instanceof Error ? err.message : String(err);
  } finally {
    searching.value = false;
  }
}

const listItems = computed(() =>
  results.value.map((r) => ({ id: r.gisco_id, name: r.lau_name, code: r.gisco_id })),
);

const emptyHint = computed(() => {
  if (search.value.trim().length < 2) return "Gemeindenamen eingeben, um zu suchen.";
  if (searching.value) return "Suche…";
  return "Keine Treffer.";
});

async function selectRegion(giscoId: string) {
  selectedGiscoId.value = giscoId;
  selectedRegion.value = null;
  detailError.value = null;
  datasets.value = null;
  datasetsError.value = null;
  try {
    const body = await apiGet<LauRegion>(`/lau/${giscoId}`);
    selectedRegion.value = body.data;
  } catch (err) {
    detailError.value = err instanceof Error ? err.message : String(err);
    return;
  }

  const nuts3 = unwrapNull(selectedRegion.value.nuts_3_code);
  if (!nuts3) return;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/lau/${giscoId}/datasets`);
    datasets.value = body.data?.datasets ?? [];
  } catch (err) {
    datasetsError.value = err instanceof Error ? err.message : String(err);
  }
}

const population = computed(() => (selectedRegion.value ? unwrapNull(selectedRegion.value.population_count) : null));
const country = computed(() =>
  selectedRegion.value ? unwrapNull(selectedRegion.value.country) || selectedRegion.value.country_code : null,
);
const nuts3Code = computed(() => (selectedRegion.value ? unwrapNull(selectedRegion.value.nuts_3_code) : null));
</script>

<template>
  <div class="browser-layout">
    <div class="panel list-panel">
      <label class="visually-hidden" for="lau-search">Gemeinde suchen</label>
      <input
        id="lau-search"
        v-model="search"
        type="search"
        placeholder="Gemeinde suchen (mind. 2 Zeichen)…"
        autocomplete="off"
      />
      <p v-if="searchError" class="error-text">Suche fehlgeschlagen: {{ searchError }}</p>
      <RegionList v-else :items="listItems" :selected-id="selectedGiscoId" :empty-hint="emptyHint" @select="selectRegion" />
    </div>
    <div class="panel detail-panel">
      <p v-if="!selectedGiscoId" class="hint">Links eine Gemeinde auswählen, um Details zu sehen.</p>
      <p v-else-if="detailError" class="error-text">Fehler: {{ detailError }}</p>
      <p v-else-if="!selectedRegion" class="hint">Lade Details…</p>
      <template v-else>
        <div class="detail-header">
          <h2>{{ selectedRegion.lau_name }}</h2>
          <span class="region-code">{{ selectedRegion.gisco_id }}</span>
        </div>
        <dl class="meta-grid">
          <dt>Land</dt>
          <dd>{{ country ?? "–" }}</dd>
          <dt>Jahr</dt>
          <dd>{{ selectedRegion.year }}</dd>
          <dt>Bevölkerung</dt>
          <dd>{{ population !== null ? formatNumber(population) : "unbekannt" }}</dd>
          <dt>NUTS-3</dt>
          <dd>{{ nuts3Code || "keine Zuordnung" }}</dd>
        </dl>

        <p v-if="!nuts3Code" class="hint">
          Keine NUTS-3-Zuordnung vorhanden – keine sozioökonomischen Datensätze verfügbar.
        </p>
        <template v-else>
          <p v-if="datasetsError" class="error-text">Fehler: {{ datasetsError }}</p>
          <p v-else-if="datasets === null" class="hint">Lade verfügbare Datensätze…</p>
          <p v-else-if="!datasets.length" class="hint">
            Für diese Region sind keine sozioökonomischen Datensätze verfügbar.
          </p>
          <DatasetPicker v-else :key="selectedGiscoId ?? ''" :datasets="datasets" :nuts-code="nuts3Code" />
        </template>
      </template>
    </div>
  </div>
</template>
