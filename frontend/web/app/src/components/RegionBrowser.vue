<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { apiGet, unwrapNull } from "../api/client";
import type {
  AgenturbezirkArbeitslosigkeitPoint,
  AgenturbezirkSummary,
  DatasetCatalogEntry,
  DatasetCatalogResponse,
  DatasetMetadataEntry,
  LauRegion,
  NutsRegionSummary,
} from "../types/api";
import { formatMonthLabel, formatNumber } from "../format";
import RegionList from "./RegionList.vue";
import DatasetPicker from "./DatasetPicker.vue";
import TimeseriesChart from "./TimeseriesChart.vue";
import DataTable from "./DataTable.vue";
import DatasetInfo from "./DatasetInfo.vue";

// Slug des ergänzenden Kreis-Level-Datensatzes "Arbeitslose" (INKAR/BBSR),
// den der "Siehe auch"-Link im Agenturbezirk-Panel im NUTS-3-Datensatz-
// Picker vorauswählt.
const INKAR_ARBEITSLOSE_SLUG = "arbeitslose";

type RegionType = "nuts" | "lau" | "agentur";
const regionType = ref<RegionType>("nuts");

const REGION_TYPE_LABELS: Record<RegionType, string> = {
  nuts: "NUTS-3",
  lau: "Gemeinden (LAU)",
  agentur: "Agenturbezirke",
};

function switchRegionType(type: RegionType) {
  regionType.value = type;
}

// --- NUTS-3 ---
const nutsRegions = ref<NutsRegionSummary[]>([]);
const nutsLoadError = ref<string | null>(null);
const nutsSearch = ref("");
const selectedNuts = ref<NutsRegionSummary | null>(null);
const nutsDatasets = ref<DatasetCatalogEntry[] | null>(null);
const nutsDatasetsError = ref<string | null>(null);
// Beim Sprung aus dem Agenturbezirk-Panel zum INKAR-Datensatz vorausgewählter
// Slug für den DatasetPicker; bei normaler Regionsauswahl aus der Liste
// bleibt er undefined (siehe selectNuts).
const nutsDatasetInitialSlug = ref<string | undefined>(undefined);

onMounted(async () => {
  try {
    const body = await apiGet<NutsRegionSummary[]>("/nuts", { level: 3, limit: "null" });
    nutsRegions.value = body.data ?? [];
  } catch (err) {
    nutsLoadError.value = err instanceof Error ? err.message : String(err);
  }
});

const nutsFiltered = computed(() => {
  const needle = nutsSearch.value.trim().toLowerCase();
  if (!needle) return nutsRegions.value;
  return nutsRegions.value.filter(
    (r) => r.region_name.toLowerCase().includes(needle) || r.nuts_id.toLowerCase().includes(needle),
  );
});
const nutsListItems = computed(() =>
  nutsFiltered.value.map((r) => ({ id: r.nuts_id, name: r.region_name, code: r.nuts_id })),
);

async function selectNuts(nutsId: string, initialSlug?: string) {
  const region = nutsRegions.value.find((r) => r.nuts_id === nutsId);
  if (!region) return;
  selectedNuts.value = region;
  nutsDatasetInitialSlug.value = initialSlug;
  nutsDatasets.value = null;
  nutsDatasetsError.value = null;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${region.nuts_id}/datasets`);
    nutsDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    nutsDatasetsError.value = err instanceof Error ? err.message : String(err);
  }
}

// --- LAU (Gemeinden) ---
const lauSearch = ref("");
const lauSearching = ref(false);
const lauSearchError = ref<string | null>(null);
const lauResults = ref<LauRegion[]>([]);
const selectedGiscoId = ref<string | null>(null);
const selectedLau = ref<LauRegion | null>(null);
const lauDetailError = ref<string | null>(null);
const lauDatasets = ref<DatasetCatalogEntry[] | null>(null);
const lauDatasetsError = ref<string | null>(null);

function dedupeByGiscoId(rows: LauRegion[]): LauRegion[] {
  const byId = new Map<string, LauRegion>();
  for (const row of rows) {
    const existing = byId.get(row.gisco_id);
    if (!existing || row.year > existing.year) byId.set(row.gisco_id, row);
  }
  return Array.from(byId.values());
}

let lauSearchTimer: ReturnType<typeof setTimeout> | null = null;
watch(lauSearch, (query) => {
  if (lauSearchTimer) clearTimeout(lauSearchTimer);
  const trimmed = query.trim();
  if (trimmed.length < 2) {
    lauResults.value = [];
    lauSearchError.value = null;
    return;
  }
  lauSearchTimer = setTimeout(() => runLauSearch(trimmed), 350);
});

async function runLauSearch(query: string) {
  lauSearching.value = true;
  lauSearchError.value = null;
  try {
    const body = await apiGet<LauRegion[]>("/lau", { geodata_name_contains: query, limit: 20 });
    lauResults.value = dedupeByGiscoId(body.data ?? []);
  } catch (err) {
    lauSearchError.value = err instanceof Error ? err.message : String(err);
  } finally {
    lauSearching.value = false;
  }
}

const lauListItems = computed(() =>
  lauResults.value.map((r) => ({ id: r.gisco_id, name: r.lau_name, code: r.gisco_id })),
);
const lauEmptyHint = computed(() => {
  if (lauSearch.value.trim().length < 2) return "Gemeindenamen eingeben, um zu suchen.";
  if (lauSearching.value) return "Suche…";
  return "Keine Treffer.";
});

async function selectLau(giscoId: string) {
  selectedGiscoId.value = giscoId;
  selectedLau.value = null;
  lauDetailError.value = null;
  lauDatasets.value = null;
  lauDatasetsError.value = null;
  try {
    const body = await apiGet<LauRegion>(`/lau/${giscoId}`);
    selectedLau.value = body.data;
  } catch (err) {
    lauDetailError.value = err instanceof Error ? err.message : String(err);
    return;
  }

  const nuts3 = unwrapNull(selectedLau.value.nuts3_id);
  if (!nuts3) return;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/lau/${giscoId}/datasets`);
    lauDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    lauDatasetsError.value = err instanceof Error ? err.message : String(err);
  }
}

const lauPopulation = computed(() => (selectedLau.value ? unwrapNull(selectedLau.value.population_count) : null));
const lauCountry = computed(() =>
  selectedLau.value ? unwrapNull(selectedLau.value.country) || selectedLau.value.country_code : null,
);
const lauNuts3Code = computed(() => (selectedLau.value ? unwrapNull(selectedLau.value.nuts3_id) : null));
const lauNuts3IsEstimate = computed(
  () => selectedLau.value != null && unwrapNull(selectedLau.value.nuts3_link_source) === "spatial_estimate",
);

// --- Agenturbezirke ---
const agenturbezirke = ref<AgenturbezirkSummary[]>([]);
const agenturLoadError = ref<string | null>(null);
const agenturSearch = ref("");
const selectedAgentur = ref<AgenturbezirkSummary | null>(null);
const agenturPoints = ref<AgenturbezirkArbeitslosigkeitPoint[] | null>(null);
const agenturPointsError = ref<string | null>(null);
let agenturLoaded = false;

// Herkunft/Lizenz/Beschreibung des AfA-Datensatzes - wie bei NUTS/LAU über
// DatasetPicker/DatasetInfo, hier einmalig geladen (ein Datensatz für alle
// Agenturbezirke, kein Wechsel wie bei DatasetPicker).
const agenturMetadata = ref<DatasetMetadataEntry | null>(null);
let agenturMetadataLoaded = false;

async function loadAgenturMetadata() {
  if (agenturMetadataLoaded) return;
  agenturMetadataLoaded = true;
  try {
    const body = await apiGet<DatasetMetadataEntry>("/datasets/arbeitslosenzahlen-afa-agenturbezirke");
    agenturMetadata.value = body.data ?? null;
  } catch {
    // Kein Katalogeintrag - Anzeige bleibt beim statischen Hinweistext.
  }
}

// Checkboxen über dem Chart: beide Metriken standardmäßig sichtbar, die
// Tabelle zeigt unabhängig davon immer beide Spalten.
const showArbeitslose = ref(true);
const showUnterbeschaeftigung = ref(true);

async function loadAgenturbezirke() {
  if (agenturLoaded) return;
  agenturLoaded = true;
  try {
    const body = await apiGet<AgenturbezirkSummary[]>("/agenturbezirke");
    agenturbezirke.value = body.data ?? [];
  } catch (err) {
    agenturLoadError.value = err instanceof Error ? err.message : String(err);
  }
  loadAgenturMetadata();
}

watch(
  regionType,
  (type) => {
    if (type === "agentur") loadAgenturbezirke();
  },
  { immediate: true },
);

const agenturFiltered = computed(() => {
  const needle = agenturSearch.value.trim().toLowerCase();
  if (!needle) return agenturbezirke.value;
  return agenturbezirke.value.filter(
    (a) => a.name.toLowerCase().includes(needle) || a.ba_schl.includes(needle),
  );
});
const agenturListItems = computed(() =>
  agenturFiltered.value.map((a) => ({ id: a.ba_schl, name: a.name, code: a.ba_schl })),
);

async function selectAgentur(baSchl: string) {
  const agentur = agenturbezirke.value.find((a) => a.ba_schl === baSchl);
  if (!agentur) return;
  selectedAgentur.value = agentur;
  agenturPoints.value = null;
  agenturPointsError.value = null;
  try {
    const body = await apiGet<AgenturbezirkArbeitslosigkeitPoint[]>(`/agenturbezirke/${baSchl}/arbeitslosigkeit`);
    agenturPoints.value = body.data ?? [];
  } catch (err) {
    agenturPointsError.value = err instanceof Error ? err.message : String(err);
  }
}

// Chart zeigt beide Metriken als eigene Serie, je nach Checkbox-Status
// ein-/ausblendbar; die Tabelle zeigt unabhängig davon immer beide Spalten.
const agenturChartSeries = computed(() => {
  if (!agenturPoints.value) return [];
  const series: { key: string; label: string; points: { label: string; value: number | null }[] }[] = [];
  if (showArbeitslose.value) {
    series.push({
      key: "arbeitslose",
      label: "Arbeitslose",
      points: agenturPoints.value.map((p) => ({
        label: formatMonthLabel(p.year, p.month),
        value: unwrapNull(p.arbeitslose),
      })),
    });
  }
  if (showUnterbeschaeftigung.value) {
    series.push({
      key: "unterbeschaeftigung",
      label: "Unterbeschäftigung",
      points: agenturPoints.value.map((p) => ({
        label: formatMonthLabel(p.year, p.month),
        value: unwrapNull(p.unterbeschaeftigung),
      })),
    });
  }
  return series;
});
const agenturTableRows = computed(() => {
  if (!agenturPoints.value) return [];
  return agenturPoints.value.map((p) => ({
    label: formatMonthLabel(p.year, p.month),
    arbeitslose: unwrapNull(p.arbeitslose),
    unterbeschaeftigung: unwrapNull(p.unterbeschaeftigung),
  }));
});
const agenturLatest = computed(() => {
  if (!agenturPoints.value || !agenturPoints.value.length) return null;
  return agenturPoints.value[agenturPoints.value.length - 1];
});

// nuts3_id ist eine räumliche Schätzung (siehe int_agentur_nuts3_spatial_link
// im dbt-Projekt), keine amtliche Zuordnung - Regionsname kommt aus der
// beim Mount bereits geladenen NUTS-3-Liste (kein Zusatz-Request nötig, da
// deren onMounted-Fetch unabhängig vom aktiven Gebietstyp läuft).
const agenturNuts3Region = computed(() => {
  const nuts3Id = selectedAgentur.value ? unwrapNull(selectedAgentur.value.nuts3_id) : null;
  if (!nuts3Id) return null;
  return nutsRegions.value.find((r) => r.nuts_id === nuts3Id) ?? null;
});

function jumpToInkarArbeitslose() {
  const nuts3Id = agenturNuts3Region.value?.nuts_id;
  if (!nuts3Id) return;
  switchRegionType("nuts");
  selectNuts(nuts3Id, INKAR_ARBEITSLOSE_SLUG);
}
</script>

<template>
  <div class="region-browser">
    <div class="region-type-switch" role="tablist" aria-label="Gebietstyp wählen">
      <button
        v-for="type in (['nuts', 'lau', 'agentur'] as RegionType[])"
        :key="type"
        type="button"
        class="nav-tab"
        :class="{ 'is-active': regionType === type }"
        role="tab"
        :aria-selected="regionType === type"
        @click="switchRegionType(type)"
      >
        {{ REGION_TYPE_LABELS[type] }}
      </button>
    </div>

    <div v-if="regionType === 'nuts'" class="browser-layout">
      <div class="panel list-panel">
        <label class="visually-hidden" for="nuts-search">NUTS-3-Region suchen</label>
        <input
          id="nuts-search"
          v-model="nutsSearch"
          type="search"
          placeholder="Region suchen (Name oder NUTS-Code)…"
          autocomplete="off"
        />
        <p v-if="nutsLoadError" class="error-text">Regionen konnten nicht geladen werden: {{ nutsLoadError }}</p>
        <RegionList
          v-else
          :items="nutsListItems"
          :selected-id="selectedNuts?.nuts_id ?? null"
          empty-hint="Keine Treffer."
          @select="selectNuts"
        />
      </div>
      <div class="panel detail-panel">
        <p v-if="!selectedNuts" class="hint">Links eine Region auswählen, um Details zu sehen.</p>
        <template v-else>
          <div class="detail-header">
            <h2>{{ selectedNuts.region_name }}</h2>
            <span class="region-code">
              {{ selectedNuts.nuts_id }}<template v-if="selectedNuts.ars"> · ARS {{ selectedNuts.ars }}</template>
            </span>
          </div>
          <p v-if="nutsDatasetsError" class="error-text">Fehler: {{ nutsDatasetsError }}</p>
          <p v-else-if="nutsDatasets === null" class="hint">Lade verfügbare Datensätze…</p>
          <p v-else-if="!nutsDatasets.length" class="hint">
            Für diese Region sind keine sozioökonomischen Datensätze verfügbar.
          </p>
          <DatasetPicker
            v-else
            :key="selectedNuts.nuts_id"
            :datasets="nutsDatasets"
            :nuts-code="selectedNuts.nuts_id"
            :initial-slug="nutsDatasetInitialSlug"
          />
        </template>
      </div>
    </div>

    <div v-else-if="regionType === 'lau'" class="browser-layout">
      <div class="panel list-panel">
        <label class="visually-hidden" for="lau-search">Gemeinde suchen</label>
        <input
          id="lau-search"
          v-model="lauSearch"
          type="search"
          placeholder="Gemeinde suchen (mind. 2 Zeichen)…"
          autocomplete="off"
        />
        <p v-if="lauSearchError" class="error-text">Suche fehlgeschlagen: {{ lauSearchError }}</p>
        <RegionList v-else :items="lauListItems" :selected-id="selectedGiscoId" :empty-hint="lauEmptyHint" @select="selectLau" />
      </div>
      <div class="panel detail-panel">
        <p v-if="!selectedGiscoId" class="hint">Links eine Gemeinde auswählen, um Details zu sehen.</p>
        <p v-else-if="lauDetailError" class="error-text">Fehler: {{ lauDetailError }}</p>
        <p v-else-if="!selectedLau" class="hint">Lade Details…</p>
        <template v-else>
          <div class="detail-header">
            <h2>{{ selectedLau.lau_name }}</h2>
            <span class="region-code">{{ selectedLau.gisco_id }}</span>
          </div>
          <dl class="meta-grid">
            <dt>Land</dt>
            <dd>{{ lauCountry ?? "–" }}</dd>
            <dt>Jahr</dt>
            <dd>{{ selectedLau.year }}</dd>
            <dt>Bevölkerung</dt>
            <dd>{{ lauPopulation !== null ? formatNumber(lauPopulation) : "unbekannt" }}</dd>
            <dt>NUTS-3</dt>
            <dd>
              {{ lauNuts3Code || "keine Zuordnung" }}
              <span
                v-if="lauNuts3Code && lauNuts3IsEstimate"
                class="hint"
                title="Räumlich ermittelt (größte Flächenüberlappung), keine amtliche Zuordnung aus den Quelldaten."
              >
                (räumlich ermittelt)
              </span>
            </dd>
          </dl>

          <p v-if="!lauNuts3Code" class="hint">
            Keine NUTS-3-Zuordnung vorhanden – keine sozioökonomischen Datensätze verfügbar.
          </p>
          <template v-else>
            <p v-if="lauDatasetsError" class="error-text">Fehler: {{ lauDatasetsError }}</p>
            <p v-else-if="lauDatasets === null" class="hint">Lade verfügbare Datensätze…</p>
            <p v-else-if="!lauDatasets.length" class="hint">
              Für diese Region sind keine sozioökonomischen Datensätze verfügbar.
            </p>
            <DatasetPicker v-else :key="selectedGiscoId ?? ''" :datasets="lauDatasets" :nuts-code="lauNuts3Code" />
          </template>
        </template>
      </div>
    </div>

    <div v-else class="browser-layout">
      <div class="panel list-panel">
        <label class="visually-hidden" for="agentur-search">Agenturbezirk suchen</label>
        <input
          id="agentur-search"
          v-model="agenturSearch"
          type="search"
          placeholder="Agenturbezirk suchen (Name oder BA-Schlüssel)…"
          autocomplete="off"
        />
        <p v-if="agenturLoadError" class="error-text">Agenturbezirke konnten nicht geladen werden: {{ agenturLoadError }}</p>
        <RegionList
          v-else
          :items="agenturListItems"
          :selected-id="selectedAgentur?.ba_schl ?? null"
          empty-hint="Keine Treffer."
          @select="selectAgentur"
        />
      </div>
      <div class="panel detail-panel">
        <p v-if="!selectedAgentur" class="hint">Links einen Agenturbezirk auswählen, um Details zu sehen.</p>
        <template v-else>
          <div class="detail-header">
            <h2>{{ selectedAgentur.name }}</h2>
            <span class="region-code">BA {{ selectedAgentur.ba_schl }}</span>
          </div>
          <p class="hint">
            Monatliche Arbeitslosen-/Unterbeschäftigungszahlen der Bundesagentur für Arbeit (Agenturbezirke sind
            eine eigenständige Gebietsgliederung, nicht deckungsgleich mit NUTS-3/LAU).
          </p>
          <p v-if="agenturNuts3Region" class="hint">
            Auf Kreisebene gibt es zusätzlich die INKAR-Arbeitslosenzahlen für
            <button type="button" class="link-button" @click="jumpToInkarArbeitslose">
              {{ agenturNuts3Region.region_name }} ({{ agenturNuts3Region.nuts_id }})
            </button>
            – räumlich am nächsten liegende NUTS-3-Region, nicht deckungsgleich mit diesem Agenturbezirk.
          </p>

          <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />

          <p v-if="agenturPointsError" class="error-text">Fehler: {{ agenturPointsError }}</p>
          <p v-else-if="agenturPoints === null" class="hint">Lade Zeitreihe…</p>
          <p v-else-if="!agenturPoints.length" class="hint">Keine Werte für diesen Agenturbezirk vorhanden.</p>
          <template v-else>
            <p v-if="agenturLatest" class="hint">
              Neuester Berichtsmonat: {{ formatMonthLabel(agenturLatest.year, agenturLatest.month) }}
            </p>
            <fieldset class="metric-toggle">
              <legend class="visually-hidden">Anzuzeigende Metriken</legend>
              <label><input v-model="showArbeitslose" type="checkbox" /> Arbeitslose</label>
              <label><input v-model="showUnterbeschaeftigung" type="checkbox" /> Unterbeschäftigung</label>
            </fieldset>
            <TimeseriesChart :period-series="agenturChartSeries" />
            <DataTable :period-rows="agenturTableRows" />
          </template>
        </template>
      </div>
    </div>
  </div>
</template>
