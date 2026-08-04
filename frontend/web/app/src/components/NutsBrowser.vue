<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { apiGet } from "../api/client";
import type { DatasetCatalogEntry, DatasetCatalogResponse, NutsRegionSummary } from "../types/api";
import RegionList from "./RegionList.vue";
import DatasetPicker from "./DatasetPicker.vue";

const allRegions = ref<NutsRegionSummary[]>([]);
const loadError = ref<string | null>(null);
const search = ref("");
const selectedRegion = ref<NutsRegionSummary | null>(null);
const datasets = ref<DatasetCatalogEntry[] | null>(null);
const datasetsError = ref<string | null>(null);

onMounted(async () => {
  try {
    const body = await apiGet<NutsRegionSummary[]>("/nuts", { level: 3, limit: "null" });
    allRegions.value = body.data ?? [];
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err);
  }
});

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase();
  if (!needle) return allRegions.value;
  return allRegions.value.filter(
    (r) => r.region_name.toLowerCase().includes(needle) || r.nuts_id.toLowerCase().includes(needle),
  );
});

const listItems = computed(() =>
  filtered.value.map((r) => ({ id: r.nuts_id, name: r.region_name, code: r.nuts_id })),
);

async function selectRegion(nutsId: string) {
  const region = allRegions.value.find((r) => r.nuts_id === nutsId);
  if (!region) return;
  selectedRegion.value = region;
  datasets.value = null;
  datasetsError.value = null;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${region.nuts_id}/datasets`);
    datasets.value = body.data?.datasets ?? [];
  } catch (err) {
    datasetsError.value = err instanceof Error ? err.message : String(err);
  }
}
</script>

<template>
  <div class="browser-layout">
    <div class="panel list-panel">
      <label class="visually-hidden" for="nuts-search">NUTS-3-Region suchen</label>
      <input
        id="nuts-search"
        v-model="search"
        type="search"
        placeholder="Region suchen (Name oder NUTS-Code)…"
        autocomplete="off"
      />
      <p v-if="loadError" class="error-text">Regionen konnten nicht geladen werden: {{ loadError }}</p>
      <RegionList
        v-else
        :items="listItems"
        :selected-id="selectedRegion?.nuts_id ?? null"
        empty-hint="Keine Treffer."
        @select="selectRegion"
      />
    </div>
    <div class="panel detail-panel">
      <p v-if="!selectedRegion" class="hint">Links eine Region auswählen, um Details zu sehen.</p>
      <template v-else>
        <div class="detail-header">
          <h2>{{ selectedRegion.region_name }}</h2>
          <span class="region-code">
            {{ selectedRegion.nuts_id }}<template v-if="selectedRegion.ars"> · ARS {{ selectedRegion.ars }}</template>
          </span>
        </div>
        <p v-if="datasetsError" class="error-text">Fehler: {{ datasetsError }}</p>
        <p v-else-if="datasets === null" class="hint">Lade verfügbare Datensätze…</p>
        <p v-else-if="!datasets.length" class="hint">
          Für diese Region sind keine sozioökonomischen Datensätze verfügbar.
        </p>
        <DatasetPicker v-else :key="selectedRegion.nuts_id" :datasets="datasets" :nuts-code="selectedRegion.nuts_id" />
      </template>
    </div>
  </div>
</template>
