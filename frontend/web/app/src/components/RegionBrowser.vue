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
  NutsLauAggregatePoint,
  NutsLauAggregateResponse,
  NutsRegionSummary,
} from "../types/api";
import { formatDatasetLabel, formatMonthLabel, formatNumber, formatPercent } from "../format";
import { AGENTUR_DATASET_SLUG, BUNDESLAND_ALIASES } from "../constants";
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

// NUTS-Hierarchie: 0=Land, 1=Bundesland, 2=Regierungsbezirk, 3=Kreis.
// Innerhalb des "nuts"-Tabs zusätzlich zum regionType wählbar (siehe
// nutsBrowseLevel unten) - der Code selbst ist präfixhierarchisch
// (DE -> DE3 -> DE30 -> DE300), Breadcrumb/Kinderlisten kommen daher ohne
// eigenen Backend-Endpunkt aus, siehe nutsBreadcrumb()/nutsChildren().
type NutsLevel = 0 | 1 | 2 | 3;
const NUTS_LEVEL_LABELS: Record<NutsLevel, string> = {
  0: "Land",
  1: "Bundesland",
  2: "Regierungsbezirk",
  3: "Kreis",
};
function parseNutsLevelParam(value: unknown): NutsLevel {
  const n = typeof value === "string" ? Number(value) : NaN;
  return n === 0 || n === 1 || n === 2 || n === 3 ? (n as NutsLevel) : 3;
}

// "dataset=slug1,slug2" in der Route <-> string[] im Komponentenzustand.
function parseSlugsParam(value: unknown): string[] {
  if (typeof value !== "string" || !value) return [];
  return value.split(",").map((s) => s.trim()).filter(Boolean);
}

// Agenturbezirke haben (anders als NUTS/LAU) keinen Datensatz-Katalog,
// sondern immer genau zwei feste Metriken - bei nur zwei fest bekannten
// Optionen sind drei Tabs (Arbeitslose / Unterbeschäftigung / beide) klarer
// als Checkboxen (keine "leere Auswahl"-Sonderfälle, jede Kombination ist
// ein einzelner Klick). Die Auswahl bleibt trotzdem als string[] im Zustand/
// in der Route (?metrics=), damit sie mit demselben Muster wie
// nutsSelectedSlugs/lauSelectedSlugs funktioniert (siehe currentQuery).
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

// Alle drei Gebietstypen leben jetzt in einer kombinierten Liste (siehe
// combinedListItems unten), per activeTypes-Filter unterscheidbar statt
// über Tabs. regionType ist trotzdem als eigener Ref nötig: er steuert,
// welcher der drei Detail-Blöcke unten angezeigt wird - abgeleitet aus dem
// Typ der zuletzt gewählten Region (gesetzt von selectNuts/selectLau/
// selectAgentur), nicht mehr aus einem Routen-Pfadsegment.
const regionType = ref<RegionType>("nuts");

const REGION_TYPE_LABELS: Record<RegionType, string> = {
  nuts: "NUTS (Land–Kreis)",
  lau: "Gemeinden (LAU)",
  agentur: "Agenturbezirke",
};

// Filter für die kombinierte Liste (siehe combinedListItems) - unabhängig
// von regionType (der Detail-Ansicht): mehrere Typen können gleichzeitig
// aktiv sein, mindestens einer muss es bleiben.
const ALL_REGION_TYPES: RegionType[] = ["nuts", "lau", "agentur"];
const activeTypes = ref<RegionType[]>([...ALL_REGION_TYPES]);
function toggleTypeFilter(type: RegionType) {
  if (activeTypes.value.includes(type)) {
    if (activeTypes.value.length === 1) return;
    activeTypes.value = activeTypes.value.filter((t) => t !== type);
  } else {
    activeTypes.value = [...activeTypes.value, type];
  }
}
function parseTypesParam(value: unknown): RegionType[] {
  if (typeof value !== "string" || !value) return [...ALL_REGION_TYPES];
  const parsed = value
    .split(",")
    .map((s) => s.trim())
    .filter((s): s is RegionType => s === "nuts" || s === "lau" || s === "agentur");
  return parsed.length ? parsed : [...ALL_REGION_TYPES];
}

// Grobe, rein clientseitige Typ-Erkennung für Deep-Links ohne ?type=
// (z.B. von Hand eingetippte/alte URLs) - ba_schl ist rein numerisch,
// NUTS-Codes sind 2-5 Zeichen mit 2-Buchstaben-Länderpräfix, LAU gisco_ids
// enthalten immer einen Unterstrich ("DE_12345678") und fallen damit durch.
// Interne Navigation (select*()) setzt ?type= immer explizit - das hier ist
// nur ein Fallback für den Erstaufruf.
function inferRegionType(id: string): RegionType {
  if (/^\d+$/.test(id)) return "agentur";
  if (/^[A-Za-z]{2}[0-9A-Za-z]{0,3}$/.test(id)) return "nuts";
  return "lau";
}

// Gemeinsame Suche über alle aktiven Gebietstypen (siehe combinedListItems) -
// ersetzt die früher pro Typ getrennten nutsSearch/lauSearch/agenturSearch.
const combinedSearch = ref("");

// --- NUTS (Land/Bundesland/Regierungsbezirk/Kreis) ---
const nutsRegions = ref<NutsRegionSummary[]>([]);
const nutsLoadError = ref<string | null>(null);
// Ebene innerhalb der NUTS-Treffer der kombinierten Liste (siehe NutsLevel
// oben); Default 3 (Kreise) entspricht dem bisherigen, alleinigen Verhalten
// dieser Ansicht. Nur relevant/sichtbar, solange "nuts" in activeTypes ist.
const nutsBrowseLevel = ref<NutsLevel>(3);
const selectedNuts = ref<NutsRegionSummary | null>(null);
const nutsDatasets = ref<DatasetCatalogEntry[] | null>(null);
const nutsDatasetsError = ref<string | null>(null);
// Gemeinden eines Kreises (nur Ebene 3) + auf Kreisebene hochaggregierte
// LAU-Werte (Bevölkerungssumme etc., siehe mart_nuts3_lau_aggregate) - beide
// unabhängig von der Datensatz-Auswahl unten.
const nutsChildLau = ref<LauRegion[] | null>(null);
const nutsChildLauError = ref<string | null>(null);
const nutsLauAggregate = ref<NutsLauAggregatePoint[] | null>(null);
// Mehrfachauswahl an Datensätzen für die aktuelle Region + Anzeigemodus
// (getrennt/kombiniert) - bleibt beim Regionswechsel innerhalb NUTS-3
// bestehen (reconcileNutsSelection() kürzt sie ggf. auf das, was in der
// neuen Region tatsächlich verfügbar ist) und wird per nutsQuery-Watcher
// (siehe unten) immer in die Route gespiegelt (F12.1: Verlinkung muss den
// tatsächlich angezeigten Zustand abbilden).
const nutsSelectedSlugs = ref<string[]>([]);

onMounted(async () => {
  try {
    // Ohne level-Filter liefert /nuts alle vier Ebenen in einer Antwort
    // (siehe GetAllNutsRegions) - daraus lässt sich die komplette
    // Eltern-/Kind-Beziehung rein clientseitig über den NUTS-Code-Präfix
    // ableiten (siehe nutsById/nutsBreadcrumb/nutsChildren unten), ohne
    // eigene Backend-Endpunkte für Breadcrumb/Kinderlisten.
    const body = await apiGet<NutsRegionSummary[]>("/nuts", { limit: "null" });
    nutsRegions.value = body.data ?? [];
  } catch (err) {
    nutsLoadError.value = err instanceof Error ? err.message : String(err);
  }
});

const nutsById = computed(() => {
  const map = new Map<string, NutsRegionSummary>();
  for (const r of nutsRegions.value) map.set(r.nuts_id, r);
  return map;
});

// NUTS-Codes sind präfixhierarchisch (DE -> DE3 -> DE30 -> DE300): der
// Parent-Code ist derselbe Code, um ein Zeichen gekürzt. NUTS-0 (2 Zeichen)
// hat keinen Parent mehr.
function nutsParentId(id: string): string | null {
  return id.length > 2 ? id.slice(0, id.length - 1) : null;
}

function nutsBreadcrumb(id: string | null): NutsRegionSummary[] {
  const chain: NutsRegionSummary[] = [];
  let current = id ? nutsParentId(id) : null;
  while (current) {
    const region = nutsById.value.get(current);
    if (region) chain.unshift(region);
    current = nutsParentId(current);
  }
  return chain;
}

function nutsChildren(id: string, level: number): NutsRegionSummary[] {
  return nutsRegions.value
    .filter((r) => r.nuts_level === level + 1 && r.nuts_id.startsWith(id))
    .sort((a, b) => a.region_name.localeCompare(b.region_name));
}

function switchNutsLevel(level: NutsLevel) {
  nutsBrowseLevel.value = level;
}

// formatPercent() aus format.ts erwartet einen Anteil (0..1) und multipliziert
// mit 100 - schuldnerquote_prozent/beschaeftigungsquote sind aber bereits in
// Prozent (z.B. 9.8 = 9.8%), deshalb hier ein eigener, einfacher Formatter.
function formatPercentValue(value: number | null): string {
  return value !== null ? `${formatNumber(value)} %` : "–";
}

const nutsFiltered = computed(() => {
  const needle = combinedSearch.value.trim().toLowerCase();
  const inLevel = nutsRegions.value.filter((r) => r.nuts_level === nutsBrowseLevel.value);
  if (!needle) return inLevel;
  return inLevel.filter((r) => {
    if (r.region_name.toLowerCase().includes(needle) || r.nuts_id.toLowerCase().includes(needle)) return true;
    // Bundesland-Abkürzungen (z.B. "NRW") matchen zusätzlich gegen die
    // Alias-Liste, da der amtliche region_name selbst keine Abkürzung ist.
    return (BUNDESLAND_ALIASES[r.region_name] ?? []).some((alias) => alias.toLowerCase().includes(needle));
  });
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
  regionType.value = "nuts";
  selectedNuts.value = region;
  nutsBrowseLevel.value = region.nuts_level as NutsLevel;
  const wanted = wantedSlugs ?? nutsSelectedSlugs.value;
  nutsDatasets.value = null;
  nutsDatasetsError.value = null;
  nutsChildLau.value = null;
  nutsChildLauError.value = null;
  nutsLauAggregate.value = null;
  if (navigate) {
    router.push({ name: "regionen", params: { regionId: nutsId }, query: { type: "nuts" } });
  }

  // GET /nuts/{id}/datasets deckt inzwischen alle Ebenen ab: für Kreise
  // (Ebene 3) aus mart_region_datasets (Kreis-grain), für Land/Bundesland/
  // Regierungsbezirk (Ebene 0-2) aus mart_nuts012_dataset_aggregate, das
  // dieselben Kreis-grain-Datensätze bereits auf diese Ebenen hochaggregiert
  // (siehe DatasetPicker.vue nutsLevel-Prop für die dazu passende
  // Zeitreihen-Abfrage).
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${region.nuts_id}/datasets`);
    nutsDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    nutsDatasetsError.value = err instanceof Error ? err.message : String(err);
    return;
  }
  reconcileNutsSelection(wanted);

  if (region.nuts_level !== 3) return;

  // Gemeinden dieses Kreises + auf Kreisebene aggregierte LAU-Werte - beide
  // unabhängig von der Datensatz-Auswahl oben (nur Ebene 3, siehe früher
  // Return oben).
  try {
    // /lau?nuts_parent_id= liefert eine Zeile je (gisco_id, Jahr) - wie bei
    // runLauSearch() unten auf den jeweils neuesten Jahrgang je Gemeinde
    // dedupen (siehe dedupeByGiscoId).
    const body = await apiGet<LauRegion[]>("/lau", { nuts_parent_id: region.nuts_id, limit: "null" });
    nutsChildLau.value = dedupeByGiscoId(body.data ?? []);
  } catch (err) {
    nutsChildLauError.value = err instanceof Error ? err.message : String(err);
  }
  try {
    const body = await apiGet<NutsLauAggregateResponse>(`/nuts/${region.nuts_id}/lau-aggregate`);
    nutsLauAggregate.value = body.data?.series ?? [];
  } catch {
    // 404 ist der Normalfall, solange lokal keine LAU-Populationsdaten mit
    // gisco_id geladen sind (siehe mart_nuts3_lau_aggregate-Kommentar) -
    // kein Fehlertext, der Abschnitt bleibt dann einfach leer.
    nutsLauAggregate.value = [];
  }
}

// --- LAU (Gemeinden) ---
const lauSearching = ref(false);
const lauSearchError = ref<string | null>(null);
const lauResults = ref<LauRegion[]>([]);
const selectedGiscoId = ref<string | null>(null);
const selectedLau = ref<LauRegion | null>(null);
const lauDetailError = ref<string | null>(null);
const lauDatasets = ref<DatasetCatalogEntry[] | null>(null);
const lauDatasetsError = ref<string | null>(null);
// Analog zu nutsSelectedSlugs.
const lauSelectedSlugs = ref<string[]>([]);

function dedupeByGiscoId(rows: LauRegion[]): LauRegion[] {
  const byId = new Map<string, LauRegion>();
  for (const row of rows) {
    const existing = byId.get(row.gisco_id);
    if (!existing || row.year > existing.year) byId.set(row.gisco_id, row);
  }
  return Array.from(byId.values());
}

let lauSearchTimer: ReturnType<typeof setTimeout> | null = null;
watch(combinedSearch, (query) => {
  if (lauSearchTimer) clearTimeout(lauSearchTimer);
  const trimmed = query.trim();
  if (trimmed.length < 2) {
    lauResults.value = [];
    lauSearchError.value = null;
    return;
  }
  if (!activeTypes.value.includes("lau")) return;
  lauSearchTimer = setTimeout(() => runLauSearch(trimmed), 350);
});
// Suche über die kombinierte Liste kann LAU nachträglich per Filter dazuschalten,
// während schon ein Suchbegriff steht (z.B. erst NUTS/Agentur, dann LAU
// aktivieren) - ohne diesen zweiten Watcher bliebe lauResults dann leer, bis
// der Suchbegriff erneut geändert wird.
watch(activeTypes, (types) => {
  const trimmed = combinedSearch.value.trim();
  if (types.includes("lau") && trimmed.length >= 2 && !lauResults.value.length && !lauSearching.value) {
    runLauSearch(trimmed);
  }
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
  regionType.value = "lau";
  selectedGiscoId.value = giscoId;
  selectedLau.value = null;
  lauDetailError.value = null;
  lauDatasets.value = null;
  lauDatasetsError.value = null;
  const wanted = wantedSlugs ?? lauSelectedSlugs.value;
  if (navigate) {
    router.push({ name: "regionen", params: { regionId: giscoId }, query: { type: "lau" } });
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
// Regionswechsel bestehen und wird per currentQuery-Watcher (unten)
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

// Agentur wird (wie NUTS) eager geladen, sobald "agentur" zu den aktiven
// Listen-Filtern gehört - standardmäßig sind alle drei Typen aktiv, die
// kombinierte Liste braucht die Daten also i.d.R. sofort beim Mount.
watch(
  activeTypes,
  (types) => {
    if (types.includes("agentur")) loadAgenturbezirke();
  },
  { immediate: true },
);

const agenturFiltered = computed(() => {
  const needle = combinedSearch.value.trim().toLowerCase();
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
  regionType.value = "agentur";
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
    router.push({ name: "regionen", params: { regionId: baSchl }, query: { type: "agentur" } });
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

// Aggregierte Kreis-Daten für diesen Agenturbezirk (siehe
// GET /agenturbezirke/{ba_schl}/dataset-aggregate/{slug}, flächenanteils-
// gewichtet über mart_agentur_nuts3_crosswalk) - "Jeweils aggregierte
// Daten anbieten" für Agentur, analog zum NUTS-0/1/2-dataset-aggregate und
// zum LAU-Kreis-Datensatzkatalog. Die Kandidaten-Slugs kommen vom größten
// überlappenden Kreis (agenturNuts3Shares ist bereits nach area_share_of_
// agentur absteigend sortiert, siehe Backend) - eigener Katalog-Endpunkt
// für Agentur existiert nicht, Kreis-grain-Datensätze sind aber praktisch
// deckungsgleich mit dem, was pro Kreis unter /nuts/{id}/datasets steht.
const agenturAggregateDatasets = ref<DatasetCatalogEntry[] | null>(null);
const agenturAggregateDatasetsError = ref<string | null>(null);
const agenturAggregateSelectedSlugs = ref<string[]>([]);

let agenturAggregatableSlugs: Set<string> | null = null;
async function loadAgenturAggregatableSlugs(): Promise<Set<string>> {
  if (agenturAggregatableSlugs) return agenturAggregatableSlugs;
  const body = await apiGet<{ dataset_slug: string }[]>("/agenturbezirke/dataset-aggregate");
  agenturAggregatableSlugs = new Set((body.data ?? []).map((r) => r.dataset_slug));
  return agenturAggregatableSlugs;
}

watch(agenturNuts3Shares, async (shares) => {
  agenturAggregateDatasets.value = null;
  agenturAggregateDatasetsError.value = null;
  const primaryNuts3 = shares?.[0]?.nuts3_id;
  if (!primaryNuts3) return;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${primaryNuts3}/datasets`);
    // /nuts/{id}/datasets listet auch Datensätze, die nicht auf Agenturbezirke
    // aggregierbar sind (z.B. beschaeftigungsquote-gemeinden, Gemeinde-grain) -
    // nur was in mart_agentur_dataset_aggregate vorkommt, ist wählbar.
    const aggregatable = await loadAgenturAggregatableSlugs();
    agenturAggregateDatasets.value = (body.data?.datasets ?? []).filter((d) => aggregatable.has(d.route_slug));
    reconcileAgenturAggregateSelection(agenturAggregateSelectedSlugs.value);
  } catch (err) {
    agenturAggregateDatasetsError.value = err instanceof Error ? err.message : String(err);
  }
});

function reconcileAgenturAggregateSelection(wanted: string[]) {
  const available = new Set((agenturAggregateDatasets.value ?? []).map((d) => d.route_slug));
  // Kein Fallback auf einen ersten Datensatz: Standard ist "keine Auswahl" =
  // reine BA-Ansicht mit echten Werten (siehe DatasetPicker, kind="agentur").
  agenturAggregateSelectedSlugs.value = wanted.filter((s) => available.has(s));
}

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
// Monatsreihen für das kombinierte Diagramm im DatasetPicker (neben den
// jährlichen Kreis-Aggregaten) - dieselbe Auswahl wie im Chart oben.
const agenturMonthlySeries = computed(() =>
  (["arbeitslose", "unterbeschaeftigung"] as AgenturMetric[])
    .filter((m) => agenturSelectedMetrics.value.includes(m))
    .map((m) => ({
      key: `ba-${m}`,
      label: `${AGENTUR_METRIC_LABELS[m]} (BA, monatlich)`,
      points: (agenturPoints.value ?? []).flatMap((p) => {
        const value = unwrapNull(m === "arbeitslose" ? p.arbeitslose : p.unterbeschaeftigung);
        return value === null ? [] : [{ year: p.year, month: p.month, value, is_interpolated: false }];
      }),
    }))
    .filter((s) => s.points.length),
);
const agenturChartReady = computed(() => !!agenturPoints.value?.length && agenturVisiblePoints.value.length > 0);
const agenturPickerReady = computed(
  () =>
    !agenturAggregateDatasetsError.value &&
    !agenturNuts3SharesError.value &&
    !!agenturNuts3Shares.value &&
    !!agenturAggregateDatasets.value?.length,
);
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
  selectNuts(nuts3Id, [INKAR_ARBEITSLOSE_SLUG]);
}

// Springt von der Karte oder anderen Ansichten aus direkt zu einer Region
// (Cross-Link "Regionen -> Karte" ist die Umkehrung, siehe MapView.vue).
function viewOnMap(geoType: RegionType, geoId: string) {
  router.push({ name: "karte", query: { geo: geoType, region: geoId } });
}

// Kombinierte Liste über alle aktiven Gebietstypen (siehe activeTypes) -
// ersetzt die früher pro Typ getrennten Listen-Panels. NUTS-Treffer folgen
// der aktuellen nutsBrowseLevel-Ebene, Agentur ist immer vollständig
// geladen, LAU nur ab 2 Zeichen Suchbegriff (siehe combinedSearch-Watcher).
type CombinedRegionItem = { id: string; name: string; code: string; type: RegionType; badge: string };
const combinedListItems = computed<CombinedRegionItem[]>(() => {
  const items: CombinedRegionItem[] = [];
  if (activeTypes.value.includes("nuts")) {
    for (const it of nutsListItems.value) items.push({ ...it, type: "nuts", badge: NUTS_LEVEL_LABELS[nutsBrowseLevel.value] });
  }
  if (activeTypes.value.includes("agentur")) {
    for (const it of agenturListItems.value) items.push({ ...it, type: "agentur", badge: "Agentur" });
  }
  if (activeTypes.value.includes("lau")) {
    for (const it of lauListItems.value) items.push({ ...it, type: "lau", badge: "LAU" });
  }
  return items;
});
const combinedSelectedId = computed<string | null>(() => {
  if (regionType.value === "nuts") return selectedNuts.value?.nuts_id ?? null;
  if (regionType.value === "lau") return selectedGiscoId.value;
  return selectedAgentur.value?.ba_schl ?? null;
});
const combinedEmptyHint = computed(() => {
  const onlyLau = activeTypes.value.length === 1 && activeTypes.value[0] === "lau";
  if (onlyLau && combinedSearch.value.trim().length < 2) return "Gemeindenamen eingeben, um zu suchen.";
  if (lauSearching.value && activeTypes.value.includes("lau")) return "Suche…";
  return "Keine Treffer.";
});
function selectCombinedItem(item: CombinedRegionItem) {
  if (item.type === "nuts") selectNuts(item.id);
  else if (item.type === "lau") selectLau(item.id);
  else selectAgentur(item.id);
}

// Spiegelt Typ-Filter + die aktuell gewählte Region samt ihrer
// Datensatz-/Ansichts-/Metrik-Auswahl in die Route zurück (router.replace,
// damit reine Auswahländerungen keine Chronik-Einträge erzeugen) - ersetzt
// die früher pro Typ getrennten nutsDatasetQuery-/lauDatasetQuery-/
// agenturMetricsQuery-Watcher durch einen einzigen, analog zu MapView.vue's
// currentQuery-Muster.
const currentQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = {};
  if (activeTypes.value.length !== ALL_REGION_TYPES.length) q.types = activeTypes.value.join(",");
  if (regionType.value === "nuts" && selectedNuts.value) {
    q.type = "nuts";
    if (nutsBrowseLevel.value !== 3) q.level = String(nutsBrowseLevel.value);
    if (nutsSelectedSlugs.value.length) q.dataset = nutsSelectedSlugs.value.join(",");
  } else if (regionType.value === "lau" && selectedGiscoId.value) {
    q.type = "lau";
    if (lauSelectedSlugs.value.length) q.dataset = lauSelectedSlugs.value.join(",");
  } else if (regionType.value === "agentur" && selectedAgentur.value) {
    q.type = "agentur";
    if (agenturSelectedMetrics.value.length) q.metrics = agenturSelectedMetrics.value.join(",");
  }
  return q;
});
const currentRegionId = computed<string | undefined>(() => {
  if (regionType.value === "nuts") return selectedNuts.value?.nuts_id;
  if (regionType.value === "lau") return selectedGiscoId.value ?? undefined;
  if (regionType.value === "agentur") return selectedAgentur.value?.ba_schl;
  return undefined;
});
watch([currentQuery, currentRegionId], ([query, id]) => {
  // route.name-Check siehe Kommentar in applyRouteSelection() - wegen
  // <KeepAlive> läuft dieser Watcher auch außerhalb der "regionen"-Route
  // weiter, darf dann aber nicht dorthin zurücknavigieren.
  if (route.name !== "regionen") return;
  router.replace({ name: "regionen", params: { regionId: id }, query });
});

// Übernimmt Typ-Filter/regionId/Typ/Datensatz aus der aktuellen Route in den
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
  // fehlinterpretiert und über den currentQuery-Watcher oben sogar zurück
  // auf /regionen navigiert werden.
  if (route.name !== "regionen") return;
  activeTypes.value = parseTypesParam(route.query.types);

  const id = route.params.regionId as string | undefined;
  const slugsFromRoute = parseSlugsParam(route.query.dataset);
  const type = (route.query.type as RegionType | undefined) ?? (id ? inferRegionType(id) : regionType.value);

  if (type === "nuts" && route.query.level !== undefined) {
    nutsBrowseLevel.value = parseNutsLevelParam(route.query.level);
  }

  if (!id) {
    // Kein Regionswechsel, aber Datensatz-Slugs ohne Region: aus dem
    // Datenkatalog "vorausgewählt", bis eine Region gewählt wird (siehe
    // Hinweistext/DatasetPicker unten im Template).
    if (slugsFromRoute.length && type === "nuts") {
      nutsSelectedSlugs.value = slugsFromRoute;
    }
    if (slugsFromRoute.length && type === "lau") {
      lauSelectedSlugs.value = slugsFromRoute;
    }
    return;
  }

  if (type === "nuts") {
    regionType.value = "nuts";
    if (!nutsRegions.value.length) return;
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
    regionType.value = "lau";
    if (selectedGiscoId.value === id) {
      if (slugsFromRoute.length) reconcileLauSelection(slugsFromRoute);
      return;
    }
    selectLau(id, slugsFromRoute.length ? slugsFromRoute : undefined, false);
  } else if (type === "agentur") {
    regionType.value = "agentur";
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
    () => route.params.regionId,
    () => route.query.type,
    () => route.query.types,
    () => route.query.dataset,
    () => route.query.metrics,
    () => route.query.level,
    nutsRegions,
    agenturbezirke,
  ],
  applyRouteSelection,
  { immediate: true },
);
</script>

<template>
  <div class="region-browser">
    <div class="region-type-filter" role="group" aria-label="Gebietstyp filtern">
      <button
        v-for="type in ALL_REGION_TYPES"
        :key="type"
        type="button"
        class="nav-tab"
        :class="{ 'is-active': activeTypes.includes(type) }"
        :aria-pressed="activeTypes.includes(type)"
        @click="toggleTypeFilter(type)"
      >
        {{ REGION_TYPE_LABELS[type] }}
      </button>
    </div>

    <div class="browser-layout">
      <div class="panel list-panel">
        <div v-if="activeTypes.includes('nuts')" class="nuts-level-switch" role="tablist" aria-label="NUTS-Ebene wählen">
          <button
            v-for="level in ([0, 1, 2, 3] as NutsLevel[])"
            :key="level"
            type="button"
            class="nav-tab"
            :class="{ 'is-active': nutsBrowseLevel === level }"
            role="tab"
            :aria-selected="nutsBrowseLevel === level"
            @click="switchNutsLevel(level)"
          >
            {{ NUTS_LEVEL_LABELS[level] }}
          </button>
        </div>
        <label class="visually-hidden" for="combined-search">Region suchen</label>
        <input
          id="combined-search"
          v-model="combinedSearch"
          type="search"
          placeholder="Region suchen (Name, NUTS-Code, Gemeinde oder BA-Schlüssel)…"
          autocomplete="off"
        />
        <p v-if="nutsLoadError" class="error-text">NUTS-Regionen konnten nicht geladen werden: {{ nutsLoadError }}</p>
        <p v-if="agenturLoadError" class="error-text">Agenturbezirke konnten nicht geladen werden: {{ agenturLoadError }}</p>
        <p v-if="lauSearchError" class="error-text">Gemeinde-Suche fehlgeschlagen: {{ lauSearchError }}</p>
        <RegionList
          :items="combinedListItems"
          :selected-id="combinedSelectedId"
          :empty-hint="combinedEmptyHint"
          :max-shown="600"
          @select="(item) => selectCombinedItem(item as CombinedRegionItem)"
        />
      </div>
      <div class="panel detail-panel">
        <template v-if="regionType === 'nuts'">
          <p v-if="!selectedNuts && nutsSelectedSlugs.length" class="hint">
            Datensatz „{{ nutsSelectedSlugs.map(formatDatasetLabel).join(", ") }}“ vorausgewählt – links eine Region
            wählen.
          </p>
          <p v-else-if="!selectedNuts" class="hint">Links eine Region auswählen, um Details zu sehen.</p>
          <template v-else>
          <nav v-if="nutsBreadcrumb(selectedNuts.nuts_id).length" class="breadcrumb" aria-label="Übergeordnete Regionen">
            <template v-for="ancestor in nutsBreadcrumb(selectedNuts.nuts_id)" :key="ancestor.nuts_id">
              <button type="button" class="link-button" @click="selectNuts(ancestor.nuts_id)">
                {{ ancestor.region_name }}
              </button>
              <span class="breadcrumb-sep">›</span>
            </template>
          </nav>
          <div class="detail-header">
            <h2>{{ selectedNuts.region_name }}</h2>
            <span class="region-code">
              {{ selectedNuts.nuts_id }} · {{ selectedNuts.nuts_level_code }}
              <template v-if="selectedNuts.ars"> · ARS {{ selectedNuts.ars }}</template>
            </span>
            <button
              v-if="selectedNuts.nuts_level === 3"
              type="button"
              class="link-button"
              @click="viewOnMap('nuts', selectedNuts.nuts_id)"
            >
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
            :nuts-level="selectedNuts.nuts_level"
            :selected-slugs="nutsSelectedSlugs"
            @update:selected-slugs="(s) => (nutsSelectedSlugs = s)"
          />

          <template v-if="selectedNuts.nuts_level !== 3">
            <template v-if="nutsChildren(selectedNuts.nuts_id, selectedNuts.nuts_level).length">
              <h3>Untergeordnete Regionen</h3>
              <ul class="child-region-list">
                <li v-for="child in nutsChildren(selectedNuts.nuts_id, selectedNuts.nuts_level)" :key="child.nuts_id">
                  <button type="button" class="link-button" @click="selectNuts(child.nuts_id)">
                    {{ child.region_name }} <span class="region-code">{{ child.nuts_id }}</span>
                  </button>
                </li>
              </ul>
            </template>
          </template>

          <template v-else>
            <h3>Auf Kreisebene aggregierte LAU-Werte</h3>
            <p v-if="nutsLauAggregate === null" class="hint">Lade Gemeinde-Aggregate…</p>
            <p v-else-if="!nutsLauAggregate.length" class="hint">
              Keine aggregierten Gemeinde-Werte für diesen Kreis verfügbar.
            </p>
            <dl v-else class="meta-grid">
              <dt>Jahr</dt>
              <dd>{{ nutsLauAggregate[0].year }}</dd>
              <dt>Bevölkerung (Summe der Gemeinden)</dt>
              <dd>
                {{
                  unwrapNull(nutsLauAggregate[0].lau_population_sum) !== null
                    ? formatNumber(unwrapNull(nutsLauAggregate[0].lau_population_sum)!)
                    : "–"
                }}
                <span
                  class="hint"
                  title="Cross-Check gegen die native Kreis-Bevölkerung oben - kann durch fehlende Gemeinde-Werte abweichen."
                  >(Cross-Check)</span
                >
              </dd>
              <dt>Beschäftigungsquote (bevölkerungsgewichtet)</dt>
              <dd>{{ formatPercentValue(unwrapNull(nutsLauAggregate[0].lau_beschaeftigungsquote_weighted_avg)) }}</dd>
            </dl>

            <h3>Gemeinden in diesem Kreis</h3>
            <p v-if="nutsChildLauError" class="error-text">Fehler: {{ nutsChildLauError }}</p>
            <p v-else-if="nutsChildLau === null" class="hint">Lade Gemeinden…</p>
            <p v-else-if="!nutsChildLau.length" class="hint">Keine Gemeinden für diesen Kreis gefunden.</p>
            <ul v-else class="child-region-list">
              <li v-for="lau in nutsChildLau" :key="lau.gisco_id">
                <button type="button" class="link-button" @click="selectLau(lau.gisco_id)">
                  {{ lau.lau_name }} <span class="region-code">{{ lau.gisco_id }}</span>
                </button>
              </li>
            </ul>
          </template>
        </template>
        </template>

        <template v-else-if="regionType === 'lau'">
        <p v-if="!selectedGiscoId && lauSelectedSlugs.length" class="hint">
          Datensatz „{{ lauSelectedSlugs.map(formatDatasetLabel).join(", ") }}“ vorausgewählt – links eine Gemeinde
          suchen.
        </p>
        <p v-else-if="!selectedGiscoId" class="hint">Links eine Gemeinde auswählen, um Details zu sehen.</p>
        <p v-else-if="lauDetailError" class="error-text">Fehler: {{ lauDetailError }}</p>
        <p v-else-if="!selectedLau" class="hint">Lade Details…</p>
        <template v-else>
          <nav v-if="lauNuts3Code && nutsById.get(lauNuts3Code)" class="breadcrumb" aria-label="Übergeordnete Regionen">
            <template v-for="ancestor in nutsBreadcrumb(lauNuts3Code)" :key="ancestor.nuts_id">
              <button type="button" class="link-button" @click="selectNuts(ancestor.nuts_id)">
                {{ ancestor.region_name }}
              </button>
              <span class="breadcrumb-sep">›</span>
            </template>
            <button type="button" class="link-button" @click="selectNuts(lauNuts3Code)">
              {{ nutsById.get(lauNuts3Code)?.region_name }}
            </button>
          </nav>
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
              @update:selected-slugs="(s) => (lauSelectedSlugs = s)"
            />
          </template>
        </template>
        </template>

        <template v-else>
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
            <p v-else-if="agenturLatest" class="hint">
              Neuester Berichtsmonat: {{ formatMonthLabel(agenturLatest.year, agenturLatest.month) }}
            </p>
          </template>

          <p class="hint">
            Optional Kreis-Datensätze (INKAR/BBSR) zum Vergleich hinzufügen: aus den Kreisen oben auf diesen
            Agenturbezirk aggregiert (Summen bzw. bevölkerungsgewichtete Durchschnitte; räumliche Schätzung, keine
            amtliche Zuordnung). Sobald ein Datensatz gewählt ist, zeigt das Diagramm indexierte Werte (Monats- und
            Jahreswerte kombiniert).
          </p>
          <p v-if="agenturAggregateDatasetsError" class="error-text">Fehler: {{ agenturAggregateDatasetsError }}</p>
          <p v-else-if="agenturNuts3SharesError" class="hint">
            Keine Kreis-Zuordnung verfügbar – keine aggregierten Daten möglich.
          </p>
          <p v-else-if="!agenturNuts3Shares || agenturAggregateDatasets === null" class="hint">
            Lade verfügbare Datensätze…
          </p>
          <p v-else-if="!agenturAggregateDatasets.length" class="hint">
            Für die Kreise dieses Agenturbezirks sind keine sozioökonomischen Datensätze verfügbar.
          </p>
          <DatasetPicker
            v-if="agenturPickerReady"
            :key="selectedAgentur.ba_schl"
            kind="agentur"
            :datasets="agenturAggregateDatasets!"
            :nuts-code="selectedAgentur.ba_schl"
            :selected-slugs="agenturAggregateSelectedSlugs"
            :monthly-series="agenturMonthlySeries"
            @update:selected-slugs="(s) => (agenturAggregateSelectedSlugs = s)"
          >
            <!-- Kein Kreis-Datensatz gewählt: reine BA-Ansicht (echte Werte) -->
            <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />
            <TimeseriesChart v-if="agenturChartReady" :period-series="agenturChartSeries" />
            <DataTable v-if="agenturChartReady" :period-rows="agenturTableRows" :period-columns="agenturTableColumns" />
            <!-- Kombinierte Ansicht: BA-Monatswerte als erster Tab neben den Kreis-Datensätzen -->
            <template #ba-tab>
              <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />
              <DataTable v-if="agenturChartReady" :period-rows="agenturTableRows" :period-columns="agenturTableColumns" />
            </template>
          </DatasetPicker>
          <template v-else>
            <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />
            <TimeseriesChart v-if="agenturChartReady" :period-series="agenturChartSeries" />
            <DataTable v-if="agenturChartReady" :period-rows="agenturTableRows" :period-columns="agenturTableColumns" />
          </template>
        </template>
        </template>
      </div>
    </div>
  </div>
</template>
