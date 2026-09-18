<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { apiGet, unwrapNull } from "../api/client";
import type {
  AgenturbezirkArbeitslosigkeitPoint,
  AgenturbezirkSummary,
  AgenturNuts3Share,
  DatasetCatalogEntry,
  DatasetCatalogResponse,
  DatasetMetadataEntry,
  LauRegion,
  NutsRegionSummary,
} from "../types/api";
import { formatDatasetLabel, formatMonthLabel, formatNumber, formatPercent } from "../format";
import { AGENTUR_DATASET_SLUG } from "../constants";
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
type DatasetViewMode = "separate" | "combined";

// "dataset=slug1,slug2" in der Route <-> string[] im Komponentenzustand.
function parseSlugsParam(value: unknown): string[] {
  if (typeof value !== "string" || !value) return [];
  return value.split(",").map((s) => s.trim()).filter(Boolean);
}
function parseViewModeParam(value: unknown): DatasetViewMode {
  return value === "combined" ? "combined" : "separate";
}

// Agenturbezirke haben (anders als NUTS/LAU) keinen Datensatz-Katalog,
// sondern immer genau zwei feste Metriken - bei nur zwei fest bekannten
// Optionen sind drei Tabs (Arbeitslose / Unterbeschäftigung / beide) klarer
// als Checkboxen (keine "leere Auswahl"-Sonderfälle, jede Kombination ist
// ein einzelner Klick). Die Auswahl bleibt trotzdem als string[] im Zustand/
// in der Route (?metrics=), damit sie mit demselben Muster wie
// nutsSelectedSlugs/lauSelectedSlugs funktioniert (siehe agenturMetricsQuery).
type AgenturMetric = "arbeitslose" | "unterbeschaeftigung";
const AGENTUR_METRIC_LABELS: Record<AgenturMetric, string> = {
  arbeitslose: "Arbeitslose",
  unterbeschaeftigung: "Unterbeschäftigung",
};
type AgenturMetricsTab = "arbeitslose" | "unterbeschaeftigung" | "both";
const AGENTUR_METRICS_TABS: { value: AgenturMetricsTab; label: string; metrics: AgenturMetric[] }[] = [
  { value: "arbeitslose", label: "Arbeitslose", metrics: ["arbeitslose"] },
  { value: "unterbeschaeftigung", label: "Unterbeschäftigung", metrics: ["unterbeschaeftigung"] },
  { value: "both", label: "Arbeitslose & Unterbeschäftigung", metrics: ["arbeitslose", "unterbeschaeftigung"] },
];
function parseMetricsParam(value: unknown): AgenturMetric[] {
  if (typeof value !== "string" || !value) return [];
  return value
    .split(",")
    .map((s) => s.trim())
    .filter((s): s is AgenturMetric => s === "arbeitslose" || s === "unterbeschaeftigung");
}

const route = useRoute();
const router = useRouter();

// Gebietstyp + ausgewählte Region leben in der Route (/regionen/:regionType/:regionId),
// nicht mehr in lokalen Refs - macht die Auswahl deep-linkbar/teilbar (siehe applyRouteSelection).
const regionType = computed<RegionType>(() => (route.params.regionType as RegionType) || "nuts");

const REGION_TYPE_LABELS: Record<RegionType, string> = {
  nuts: "NUTS-3",
  lau: "Gemeinden (LAU)",
  agentur: "Agenturbezirke",
};

function switchRegionType(type: RegionType) {
  router.push({ name: "regionen", params: { regionType: type } });
}

// --- NUTS-3 ---
const nutsRegions = ref<NutsRegionSummary[]>([]);
const nutsLoadError = ref<string | null>(null);
const nutsSearch = ref("");
const selectedNuts = ref<NutsRegionSummary | null>(null);
const nutsDatasets = ref<DatasetCatalogEntry[] | null>(null);
const nutsDatasetsError = ref<string | null>(null);
// Mehrfachauswahl an Datensätzen für die aktuelle Region + Anzeigemodus
// (getrennt/kombiniert) - bleibt beim Regionswechsel innerhalb NUTS-3
// bestehen (reconcileNutsSelection() kürzt sie ggf. auf das, was in der
// neuen Region tatsächlich verfügbar ist) und wird per nutsQuery-Watcher
// (siehe unten) immer in die Route gespiegelt (F12.1: Verlinkung muss den
// tatsächlich angezeigten Zustand abbilden).
const nutsSelectedSlugs = ref<string[]>([]);
const nutsViewMode = ref<DatasetViewMode>("separate");

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

// Kürzt die gewünschte Auswahl auf das, was für die aktuelle Region
// tatsächlich verfügbar ist, und wählt beim ersten Aufruf (keine
// Vorauswahl/nichts mehr übrig) den ersten verfügbaren Datensatz - so ist
// nach jeder Regionsauswahl immer mindestens ein Datensatz gewählt und damit
// (über den nutsQuery-Watcher) auch immer ?dataset= in der Route gesetzt.
function reconcileNutsSelection(wanted: string[]) {
  const available = new Set((nutsDatasets.value ?? []).map((d) => d.route_slug));
  let next = wanted.filter((s) => available.has(s));
  if (!next.length) {
    const first = nutsDatasets.value?.[0]?.route_slug;
    next = first ? [first] : [];
  }
  nutsSelectedSlugs.value = next;
}

// navigate=false wird von applyRouteSelection() genutzt, wenn die Auswahl
// bereits aus der Route kommt (Deep-Link/Zurück-Navigation) - sonst würde ein
// erneuter router.push() hier die Route auf sich selbst schieben.
// wantedSlugs: explizite Vorauswahl (z.B. Cross-Link aus dem Datenkatalog
// oder Deep-Link); ohne Angabe bleibt die bisherige Auswahl bestehen
// (Regionswechsel behält die zuvor gewählten Datensätze bei, sofern in der
// neuen Region verfügbar - siehe reconcileNutsSelection).
async function selectNuts(nutsId: string, wantedSlugs?: string[], navigate = true) {
  const region = nutsRegions.value.find((r) => r.nuts_id === nutsId);
  if (!region) return;
  selectedNuts.value = region;
  const wanted = wantedSlugs ?? nutsSelectedSlugs.value;
  nutsDatasets.value = null;
  nutsDatasetsError.value = null;
  if (navigate) {
    router.push({ name: "regionen", params: { regionType: "nuts", regionId: nutsId } });
  }
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${region.nuts_id}/datasets`);
    nutsDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    nutsDatasetsError.value = err instanceof Error ? err.message : String(err);
    return;
  }
  reconcileNutsSelection(wanted);
}

// Spiegelt die aktuelle Datensatz-Auswahl/-Ansicht in die Route zurück
// (router.replace, damit reine Auswahländerungen keine Chronik-Einträge
// erzeugen), sobald eine NUTS-3-Region gewählt ist - analog zu MapView.vue's
// currentQuery/watch-Muster. Macht Requirement 1 robust: ?dataset= ist ab dem
// Moment gesetzt, in dem eine Region gewählt ist, und bleibt synchron mit
// jeder späteren Auswahl-/Ansichtsänderung.
const nutsDatasetQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = {};
  if (nutsSelectedSlugs.value.length) q.dataset = nutsSelectedSlugs.value.join(",");
  if (nutsSelectedSlugs.value.length > 1) q.view = nutsViewMode.value;
  return q;
});
watch(nutsDatasetQuery, (query) => {
  // route.name-Check siehe Kommentar in applyRouteSelection() - wegen
  // <KeepAlive> läuft dieser Watcher auch außerhalb der "regionen"-Route
  // weiter, darf dann aber nicht dorthin zurücknavigieren.
  if (route.name !== "regionen" || regionType.value !== "nuts" || !selectedNuts.value) return;
  router.replace({ name: "regionen", params: { regionType: "nuts", regionId: selectedNuts.value.nuts_id }, query });
});

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
// Analog zu nutsSelectedSlugs/nutsViewMode.
const lauSelectedSlugs = ref<string[]>([]);
const lauViewMode = ref<DatasetViewMode>("separate");

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

function reconcileLauSelection(wanted: string[]) {
  const available = new Set((lauDatasets.value ?? []).map((d) => d.route_slug));
  let next = wanted.filter((s) => available.has(s));
  if (!next.length) {
    const first = lauDatasets.value?.[0]?.route_slug;
    next = first ? [first] : [];
  }
  lauSelectedSlugs.value = next;
}

async function selectLau(giscoId: string, wantedSlugs?: string[], navigate = true) {
  selectedGiscoId.value = giscoId;
  selectedLau.value = null;
  lauDetailError.value = null;
  lauDatasets.value = null;
  lauDatasetsError.value = null;
  const wanted = wantedSlugs ?? lauSelectedSlugs.value;
  if (navigate) {
    router.push({ name: "regionen", params: { regionType: "lau", regionId: giscoId } });
  }
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
    return;
  }
  reconcileLauSelection(wanted);
}

// Analog zu nutsDatasetQuery.
const lauDatasetQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = {};
  if (lauSelectedSlugs.value.length) q.dataset = lauSelectedSlugs.value.join(",");
  if (lauSelectedSlugs.value.length > 1) q.view = lauViewMode.value;
  return q;
});
watch(lauDatasetQuery, (query) => {
  if (route.name !== "regionen" || regionType.value !== "lau" || !selectedGiscoId.value) return;
  router.replace({ name: "regionen", params: { regionType: "lau", regionId: selectedGiscoId.value }, query });
});

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
// n:m-Zuordnung zu überlappenden NUTS-3-Regionen (Flächen-/Bevölkerungsanteile),
// siehe mart_agentur_nuts3_crosswalk im dbt-Projekt - eine räumliche
// Schätzung, keine amtliche Zuordnung.
const agenturNuts3Shares = ref<AgenturNuts3Share[] | null>(null);
const agenturNuts3SharesError = ref<string | null>(null);
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
    const body = await apiGet<DatasetMetadataEntry>(`/datasets/${AGENTUR_DATASET_SLUG}`);
    agenturMetadata.value = body.data ?? null;
  } catch {
    // Kein Katalogeintrag - Anzeige bleibt beim statischen Hinweistext.
  }
}

// Tabs über dem Chart: beide Metriken standardmäßig sichtbar, die Tabelle
// zeigt nur die Spalten der gewählten Metrik(en) (siehe agenturTableColumns).
// Wie nutsSelectedSlugs/lauSelectedSlugs bleibt die Auswahl beim
// Regionswechsel bestehen und wird per agenturMetricsQuery-Watcher (unten)
// immer in die Route gespiegelt.
const agenturSelectedMetrics = ref<AgenturMetric[]>(["arbeitslose", "unterbeschaeftigung"]);

// Leitet aus der aktuellen Auswahl den aktiven Tab ab (für die
// is-active-Markierung im Template) - der Regelfall (zwei Metriken) ist
// "both", alles andere ist eindeutig eine der beiden Einzelauswahlen.
const agenturMetricsTab = computed<AgenturMetricsTab>(() => {
  if (agenturSelectedMetrics.value.length === 2) return "both";
  return agenturSelectedMetrics.value[0] === "unterbeschaeftigung" ? "unterbeschaeftigung" : "arbeitslose";
});

function selectAgenturMetricsTab(tab: AgenturMetricsTab) {
  agenturSelectedMetrics.value = AGENTUR_METRICS_TABS.find((t) => t.value === tab)!.metrics;
}

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

async function selectAgentur(baSchl: string, navigate = true, wantedMetrics?: AgenturMetric[]) {
  const agentur = agenturbezirke.value.find((a) => a.ba_schl === baSchl);
  if (!agentur) return;
  selectedAgentur.value = agentur;
  agenturPoints.value = null;
  agenturPointsError.value = null;
  agenturNuts3Shares.value = null;
  agenturNuts3SharesError.value = null;
  // Anders als bei NUTS/LAU (siehe reconcileNutsSelection) gibt es hier keine
  // von der Region abhängige Verfügbarkeit zu prüfen - beide Metriken
  // existieren für jeden Agenturbezirk gleich, eine explizite Auswahl kann
  // also direkt übernommen werden.
  if (wantedMetrics?.length) agenturSelectedMetrics.value = wantedMetrics;
  if (navigate) {
    router.push({ name: "regionen", params: { regionType: "agentur", regionId: baSchl } });
  }
  try {
    const body = await apiGet<AgenturbezirkArbeitslosigkeitPoint[]>(`/agenturbezirke/${baSchl}/arbeitslosigkeit`);
    agenturPoints.value = body.data ?? [];
  } catch (err) {
    agenturPointsError.value = err instanceof Error ? err.message : String(err);
  }
  try {
    const body = await apiGet<AgenturNuts3Share[]>(`/agenturbezirke/${baSchl}/nuts3`);
    agenturNuts3Shares.value = body.data ?? [];
  } catch (err) {
    agenturNuts3SharesError.value = err instanceof Error ? err.message : String(err);
  }
}

// Analog zu nutsDatasetQuery/lauDatasetQuery - spiegelt die Metrik-Auswahl
// immer in die Route, sobald ein Agenturbezirk gewählt ist.
const agenturMetricsQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = {};
  if (agenturSelectedMetrics.value.length) q.metrics = agenturSelectedMetrics.value.join(",");
  return q;
});
watch(agenturMetricsQuery, (query) => {
  if (route.name !== "regionen" || regionType.value !== "agentur" || !selectedAgentur.value) return;
  router.replace({ name: "regionen", params: { regionType: "agentur", regionId: selectedAgentur.value.ba_schl }, query });
});

// "Unterbeschäftigung" wird von der BA erst seit 2009 erhoben, "Arbeitslose"
// schon länger - Perioden, in denen KEINE der gerade gewählten Metriken
// einen Wert hat, würden sonst als leere Zeilen/eine leere Chart-Strecke am
// Anfang auftauchen. Gefiltert wird auf Basis der aktuellen Auswahl (nicht
// stur auf "irgendein Wert vorhanden"), damit z.B. der reine
// "Unterbeschäftigung"-Tab bei 2007-2009 direkt einsetzt, statt mit Jahren
// leerer "–"-Zeilen zu beginnen.
const agenturVisiblePoints = computed(() => {
  if (!agenturPoints.value) return [];
  return agenturPoints.value.filter(
    (p) =>
      (agenturSelectedMetrics.value.includes("arbeitslose") && unwrapNull(p.arbeitslose) !== null) ||
      (agenturSelectedMetrics.value.includes("unterbeschaeftigung") && unwrapNull(p.unterbeschaeftigung) !== null),
  );
});

// Chart zeigt beide Metriken als eigene Serie, je nach Tab ein-/ausblendbar;
// die Tabelle zeigt nur die Spalten der gewählten Metrik(en) (siehe
// agenturTableColumns).
const agenturChartSeries = computed(() => {
  const series: { key: string; label: string; points: { label: string; value: number | null }[] }[] = [];
  if (agenturSelectedMetrics.value.includes("arbeitslose")) {
    series.push({
      key: "arbeitslose",
      label: AGENTUR_METRIC_LABELS.arbeitslose,
      points: agenturVisiblePoints.value.map((p) => ({
        label: formatMonthLabel(p.year, p.month),
        value: unwrapNull(p.arbeitslose),
      })),
    });
  }
  if (agenturSelectedMetrics.value.includes("unterbeschaeftigung")) {
    series.push({
      key: "unterbeschaeftigung",
      label: AGENTUR_METRIC_LABELS.unterbeschaeftigung,
      points: agenturVisiblePoints.value.map((p) => ({
        label: formatMonthLabel(p.year, p.month),
        value: unwrapNull(p.unterbeschaeftigung),
      })),
    });
  }
  return series;
});
const agenturTableRows = computed(() =>
  agenturVisiblePoints.value.map((p) => ({
    label: formatMonthLabel(p.year, p.month),
    arbeitslose: unwrapNull(p.arbeitslose),
    unterbeschaeftigung: unwrapNull(p.unterbeschaeftigung),
  })),
);
// Tabelle zeigt jetzt (wie der Chart) nur die ausgewählten Metriken als
// Spalten, statt immer beide - konsistent mit den NUTS/LAU-Datensatzblöcken,
// wo abgewählte Datensätze ebenfalls nirgends mehr auftauchen.
const agenturTableColumns = computed(() =>
  (["arbeitslose", "unterbeschaeftigung"] as AgenturMetric[])
    .filter((m) => agenturSelectedMetrics.value.includes(m))
    .map((key) => ({ key, label: AGENTUR_METRIC_LABELS[key] })),
);
const agenturLatest = computed(() => {
  if (!agenturVisiblePoints.value.length) return null;
  return agenturVisiblePoints.value[agenturVisiblePoints.value.length - 1];
});

function jumpToInkarArbeitslose(nuts3Id: string) {
  // selectNuts() navigiert bereits nach regionType "nuts" (siehe dort) - ein
  // zusätzlicher switchRegionType()-Aufruf würde nur eine zweite,
  // überflüssige Route-Navigation davor schieben.
  selectNuts(nuts3Id, [INKAR_ARBEITSLOSE_SLUG]);
}

// Springt von der Karte oder anderen Ansichten aus direkt zu einer Region
// (Cross-Link "Regionen -> Karte" ist die Umkehrung, siehe MapView.vue).
function viewOnMap(geoType: RegionType, geoId: string) {
  router.push({ name: "karte", query: { geo: geoType, region: geoId } });
}

// Übernimmt regionType/regionId/dataset aus der aktuellen Route in den
// Komponentenzustand - für Deep-Links, Browser-Zurück/Vor und externe
// Cross-Links (z.B. aus dem Datenkatalog). navigate=false verhindert, dass
// die select*()-Funktionen die Route (auf sich selbst) erneut pushen.
function applyRouteSelection() {
  // RegionBrowser bleibt über <KeepAlive> in App.vue auch außerhalb der
  // "regionen"-Route aktiv, seine Watcher laufen also z.B. auch beim
  // Navigieren zur Karte weiter. route ist (via useRoute()) das global
  // aktuelle Route-Objekt, nicht "die Route, mit der diese Komponente
  // zuletzt aktiv war" - ohne diese Prüfung würde z.B. Kartes eigenes
  // ?dataset= (gleicher Query-Key!) hier als NUTS-3-Datensatzauswahl
  // fehlinterpretiert (regionType fällt ohne :regionType-Param auf "nuts"
  // zurück) und über den nutsDatasetQuery-Watcher unten sogar zurück auf
  // /regionen navigiert werden.
  if (route.name !== "regionen") return;
  const type = regionType.value;
  const id = route.params.regionId as string | undefined;
  const slugsFromRoute = parseSlugsParam(route.query.dataset);
  const viewFromRoute = parseViewModeParam(route.query.view);

  if (!id) {
    // Kein Regionswechsel, aber Datensatz-Slugs ohne Region: aus dem
    // Datenkatalog "vorausgewählt", bis eine Region gewählt wird (siehe
    // Hinweistext/DatasetPicker unten im Template).
    if (slugsFromRoute.length && type === "nuts") {
      nutsSelectedSlugs.value = slugsFromRoute;
      nutsViewMode.value = viewFromRoute;
    }
    if (slugsFromRoute.length && type === "lau") {
      lauSelectedSlugs.value = slugsFromRoute;
      lauViewMode.value = viewFromRoute;
    }
    return;
  }

  if (type === "nuts") {
    if (!nutsRegions.value.length) return;
    if (viewFromRoute) nutsViewMode.value = viewFromRoute;
    if (selectedNuts.value?.nuts_id === id) {
      // Region ist schon aktiv (z.B. ein Link, der nur die Datensatz-Auswahl
      // für dieselbe Region ändert) - kein erneuter Fetch nötig, aber die
      // explizite Auswahl aus der Route trotzdem übernehmen. Ohne diesen
      // Zweig würde selectNuts() unten gar nicht aufgerufen (early return),
      // und ein Link mit anderem ?dataset= auf dieselbe Region bliebe wirkungslos.
      if (slugsFromRoute.length) reconcileNutsSelection(slugsFromRoute);
      return;
    }
    selectNuts(id, slugsFromRoute.length ? slugsFromRoute : undefined, false);
  } else if (type === "lau") {
    if (viewFromRoute) lauViewMode.value = viewFromRoute;
    if (selectedGiscoId.value === id) {
      if (slugsFromRoute.length) reconcileLauSelection(slugsFromRoute);
      return;
    }
    selectLau(id, slugsFromRoute.length ? slugsFromRoute : undefined, false);
  } else if (type === "agentur") {
    if (!agenturbezirke.value.length) return;
    const metricsFromRoute = parseMetricsParam(route.query.metrics);
    if (selectedAgentur.value?.ba_schl === id) {
      // Analog zu oben - z.B. der Karten-Sidebar-Link auf denselben
      // Agenturbezirk, aber mit einer anderen Metrik.
      if (metricsFromRoute.length) agenturSelectedMetrics.value = metricsFromRoute;
      return;
    }
    selectAgentur(id, false, metricsFromRoute.length ? metricsFromRoute : undefined);
  }
}

watch(
  [
    () => route.params.regionType,
    () => route.params.regionId,
    () => route.query.dataset,
    () => route.query.metrics,
    () => route.query.view,
    nutsRegions,
    agenturbezirke,
  ],
  applyRouteSelection,
  { immediate: true },
);
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
          @select="(id: string) => selectNuts(id)"
        />
      </div>
      <div class="panel detail-panel">
        <p v-if="!selectedNuts && nutsSelectedSlugs.length" class="hint">
          Datensatz „{{ nutsSelectedSlugs.map(formatDatasetLabel).join(", ") }}“ vorausgewählt – links eine Region
          wählen.
        </p>
        <p v-else-if="!selectedNuts" class="hint">Links eine Region auswählen, um Details zu sehen.</p>
        <template v-else>
          <div class="detail-header">
            <h2>{{ selectedNuts.region_name }}</h2>
            <span class="region-code">
              {{ selectedNuts.nuts_id }}<template v-if="selectedNuts.ars"> · ARS {{ selectedNuts.ars }}</template>
            </span>
            <button type="button" class="link-button" @click="viewOnMap('nuts', selectedNuts.nuts_id)">
              Auf der Karte anzeigen ↗
            </button>
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
            :selected-slugs="nutsSelectedSlugs"
            :view-mode="nutsViewMode"
            @update:selected-slugs="(s) => (nutsSelectedSlugs = s)"
            @update:view-mode="(m) => (nutsViewMode = m)"
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
        <RegionList
          v-else
          :items="lauListItems"
          :selected-id="selectedGiscoId"
          :empty-hint="lauEmptyHint"
          @select="(id: string) => selectLau(id)"
        />
      </div>
      <div class="panel detail-panel">
        <p v-if="!selectedGiscoId && lauSelectedSlugs.length" class="hint">
          Datensatz „{{ lauSelectedSlugs.map(formatDatasetLabel).join(", ") }}“ vorausgewählt – links eine Gemeinde
          suchen.
        </p>
        <p v-else-if="!selectedGiscoId" class="hint">Links eine Gemeinde auswählen, um Details zu sehen.</p>
        <p v-else-if="lauDetailError" class="error-text">Fehler: {{ lauDetailError }}</p>
        <p v-else-if="!selectedLau" class="hint">Lade Details…</p>
        <template v-else>
          <div class="detail-header">
            <h2>{{ selectedLau.lau_name }}</h2>
            <span class="region-code">{{ selectedLau.gisco_id }}</span>
            <button type="button" class="link-button" @click="viewOnMap('lau', selectedLau.gisco_id)">
              Auf der Karte anzeigen ↗
            </button>
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
            <DatasetPicker
              v-else
              :key="selectedGiscoId ?? ''"
              :datasets="lauDatasets"
              :nuts-code="lauNuts3Code"
              :selected-slugs="lauSelectedSlugs"
              :view-mode="lauViewMode"
              @update:selected-slugs="(s) => (lauSelectedSlugs = s)"
              @update:view-mode="(m) => (lauViewMode = m)"
            />
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
            <button type="button" class="link-button" @click="viewOnMap('agentur', selectedAgentur.ba_schl)">
              Auf der Karte anzeigen ↗
            </button>
          </div>
          <p class="hint">
            Monatliche Arbeitslosen-/Unterbeschäftigungszahlen der Bundesagentur für Arbeit (Agenturbezirke sind
            eine eigenständige Gebietsgliederung, nicht deckungsgleich mit NUTS-3/LAU).
          </p>
          <div v-if="agenturNuts3SharesError" class="error-text">
            NUTS-3-Zuordnung konnte nicht geladen werden: {{ agenturNuts3SharesError }}
          </div>
          <div v-else-if="agenturNuts3Shares && agenturNuts3Shares.length" class="nuts3-composition">
            <p class="hint">
              Dieser Agenturbezirk setzt sich aus folgenden Kreisen zusammen (räumlich ermittelt, keine amtliche
              Zuordnung). Auf Kreisebene gibt es dort zusätzlich die INKAR-Arbeitslosenzahlen:
            </p>
            <table class="nuts3-share-table">
              <thead>
                <tr>
                  <th scope="col">Kreis</th>
                  <th scope="col">
                    Anteil des Kreises an diesem Agenturbezirk (Fläche)
                    <span
                      class="info-icon"
                      tabindex="0"
                      title="Beispiel: Wie viel % der Fläche dieses Agenturbezirks macht dieser Kreis aus?"
                      >ⓘ</span
                    >
                  </th>
                  <th scope="col">
                    Anteil des Agenturbezirks an diesem Kreis (Fläche)
                    <span
                      class="info-icon"
                      tabindex="0"
                      title="Beispiel: Wie viel % der Fläche dieses Kreises liegt in diesem Agenturbezirk?"
                      >ⓘ</span
                    >
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="share in agenturNuts3Shares" :key="share.nuts3_id">
                  <td>
                    <button type="button" class="link-button" @click="jumpToInkarArbeitslose(share.nuts3_id)">
                      {{ share.region_name ?? share.nuts3_id }} ({{ share.nuts3_id }})
                    </button>
                  </td>
                  <td>{{ formatPercent(share.area_share_of_agentur) }}</td>
                  <td>{{ formatPercent(share.area_share_of_nuts3) }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />

          <p v-if="agenturPointsError" class="error-text">Fehler: {{ agenturPointsError }}</p>
          <p v-else-if="agenturPoints === null" class="hint">Lade Zeitreihe…</p>
          <p v-else-if="!agenturPoints.length" class="hint">Keine Werte für diesen Agenturbezirk vorhanden.</p>
          <template v-else>
            <div class="metric-toggle" role="tablist" aria-label="Anzuzeigende Metriken">
              <button
                v-for="tab in AGENTUR_METRICS_TABS"
                :key="tab.value"
                type="button"
                class="nav-tab"
                :class="{ 'is-active': agenturMetricsTab === tab.value }"
                role="tab"
                :aria-selected="agenturMetricsTab === tab.value"
                @click="selectAgenturMetricsTab(tab.value)"
              >
                {{ tab.label }}
              </button>
            </div>
            <p v-if="!agenturVisiblePoints.length" class="hint">
              Für „{{
                AGENTUR_METRICS_TABS.find((t) => t.value === agenturMetricsTab)?.label
              }}“ liegen für diesen Agenturbezirk keine Werte vor.
            </p>
            <template v-else>
              <p v-if="agenturLatest" class="hint">
                Neuester Berichtsmonat: {{ formatMonthLabel(agenturLatest.year, agenturLatest.month) }}
              </p>
              <TimeseriesChart :period-series="agenturChartSeries" />
              <DataTable :period-rows="agenturTableRows" :period-columns="agenturTableColumns" />
            </template>
          </template>
        </template>
      </div>
    </div>
  </div>
</template>
