<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { apiGet, unwrapNull } from "../api/client";
import type {
  AgenturbezirkArbeitslosigkeitPoint,
  AgenturbezirkSummary,
  AgenturNutsMembership,
  AgenturNuts3Share,
  DatasetCatalogEntry,
  DatasetCatalogResponse,
  DatasetMetadataEntry,
  LauRegion,
  NutsLauAggregatePoint,
  NutsAgenturbezirk,
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
import DownloadDialog, { type ExtraDownloadOption } from "./DownloadDialog.vue";

// Slug of the supplementary Kreis-level dataset "Arbeitslose" (INKAR/BBSR) that the
// "Siehe auch" link in the Agenturbezirk panel preselects in the NUTS-3 dataset
// picker.
const INKAR_ARBEITSLOSE_SLUG = "arbeitslose";

type RegionType = "nuts" | "lau" | "agentur";

// NUTS hierarchy: 0=Land, 1=Bundesland, 2=Regierungsbezirk, 3=Kreis.
// Selectable within the "nuts" tab in addition to regionType (see nutsBrowseLevel
// below) - the code itself is prefix-hierarchical (DE -> DE3 -> DE30 -> DE300), so
// breadcrumb/child lists need no backend endpoint of their own, see
// nutsBreadcrumb()/nutsChildren().
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

// "dataset=slug1,slug2" in the route <-> string[] in component state.
function parseSlugsParam(value: unknown): string[] {
  if (typeof value !== "string" || !value) return [];
  return value.split(",").map((s) => s.trim()).filter(Boolean);
}

// Unlike NUTS/LAU, Agenturbezirke have no dataset catalog but always exactly two
// fixed metrics - with only two known options, three tabs (Arbeitslose /
// Unterbeschäftigung / both) are clearer than checkboxes (no "empty selection"
// special cases, every combination is a single click). The selection still lives
// as string[] in state/in the route (?metrics=) so it works with the same pattern
// as nutsSelectedSlugs/lauSelectedSlugs (see currentQuery).
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

// All three area types live in one combined list (see combinedListItems below),
// distinguishable via the activeTypes filter instead of tabs. regionType is still
// needed as its own ref: it controls which of the three detail blocks below is
// shown - derived from the type of the last selected region (set by selectNuts/
// selectLau/selectAgentur), no longer from a route path segment.
const regionType = ref<RegionType>("nuts");

const REGION_TYPE_LABELS: Record<RegionType, string> = {
  nuts: "NUTS (Land–Kreis)",
  lau: "Gemeinden (LAU)",
  agentur: "Agenturbezirke",
};

// Filter for the combined list (see combinedListItems) - independent of regionType
// (the detail view): several types can be active at once, at least one must
// remain.
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

// Rough, purely client-side type detection for deep links without ?type=
// (e.g. hand-typed/old URLs) - ba_schl is purely numeric, NUTS codes are 2-5
// characters with a 2-letter country prefix, LAU gisco_ids always contain an
// underscore ("DE_12345678") and thus fall through.
// Internal navigation (select*()) always sets ?type= explicitly - this is only a
// fallback for the first load.
function inferRegionType(id: string): RegionType {
  if (/^\d+$/.test(id)) return "agentur";
  if (/^[A-Za-z]{2}[0-9A-Za-z]{0,3}$/.test(id)) return "nuts";
  return "lau";
}

// Shared search across all active area types (see combinedListItems) - replaces
// the formerly per-type nutsSearch/lauSearch/agenturSearch.
const combinedSearch = ref("");

// --- NUTS (Land/Bundesland/Regierungsbezirk/Kreis) ---
const nutsRegions = ref<NutsRegionSummary[]>([]);
const nutsLoadError = ref<string | null>(null);
// Level within the NUTS hits of the combined list (see NutsLevel above); default 3
// (Kreise) matches the previous, sole behavior of this view. Only relevant/visible
// while "nuts" is in activeTypes.
const nutsBrowseLevel = ref<NutsLevel>(3);
const selectedNuts = ref<NutsRegionSummary | null>(null);
const nutsDatasets = ref<DatasetCatalogEntry[] | null>(null);
const nutsDatasetsError = ref<string | null>(null);
// Gemeinden of a Kreis (level 3 only) + LAU values aggregated up to Kreis level
// (population sum etc., see mart_nuts3_lau_aggregate) - both independent of the
// dataset selection below.
const nutsChildLau = ref<LauRegion[] | null>(null);
const nutsChildLauError = ref<string | null>(null);
const nutsLauAggregate = ref<NutsLauAggregatePoint[] | null>(null);
// Multi-select of datasets for the current region + display mode (separate/
// combined) - persists across region switches within NUTS-3 (reconcileNutsSelection()
// trims it to what is actually available in the new region) and is always mirrored
// into the route via the nutsQuery watcher (see below) (F12.1: a link must reflect
// the state actually displayed).
const nutsSelectedSlugs = ref<string[]>([]);

onMounted(async () => {
  try {
    // Without a level filter /nuts returns all four levels in one response (see
    // GetAllNutsRegions) - the full parent/child relationship can be derived purely
    // client-side from the NUTS code prefix (see nutsById/nutsBreadcrumb/
    // nutsChildren below), without backend endpoints of their own for
    // breadcrumb/child lists.
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

// NUTS codes are prefix-hierarchical (DE -> DE3 -> DE30 -> DE300): the parent code
// is the same code shortened by one character. NUTS-0 (2 characters) has no
// parent.
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

// formatPercent() from format.ts expects a share (0..1) and multiplies by 100 -
// but schuldnerquote_prozent/beschaeftigungsquote are already in percent (e.g.
// 9.8 = 9.8%), hence a simple formatter of its own here.
function formatPercentValue(value: number | null): string {
  return value !== null ? `${formatNumber(value)} %` : "–";
}

const nutsFiltered = computed(() => {
  const needle = combinedSearch.value.trim().toLowerCase();
  const inLevel = nutsRegions.value.filter((r) => r.nuts_level === nutsBrowseLevel.value);
  if (!needle) return inLevel;
  return inLevel.filter((r) => {
    if (r.region_name.toLowerCase().includes(needle) || r.nuts_id.toLowerCase().includes(needle)) return true;
    // Bundesland abbreviations (e.g. "NRW") also match against the alias list,
    // since the official region_name itself is not an abbreviation.
    return (BUNDESLAND_ALIASES[r.region_name] ?? []).some((alias) => alias.toLowerCase().includes(needle));
  });
});
const nutsListItems = computed(() =>
  nutsFiltered.value.map((r) => ({ id: r.nuts_id, name: r.region_name, code: r.nuts_id })),
);

// Trims the wanted selection to what is actually available for the current region
// and on the first call (no preselection/nothing left) picks the first available
// dataset - so at least one dataset is always selected after each region selection
// and thus (via the nutsQuery watcher) ?dataset= is always set in the route.
function reconcileNutsSelection(wanted: string[]) {
  const available = new Set((nutsDatasets.value ?? []).map((d) => d.route_slug));
  let next = wanted.filter((s) => available.has(s));
  if (!next.length) {
    const first = nutsDatasets.value?.[0]?.route_slug;
    next = first ? [first] : [];
  }
  nutsSelectedSlugs.value = next;
}

// navigate=false is used by applyRouteSelection() when the selection already comes
// from the route (deep link/back navigation) - otherwise another router.push() here
// would push the route onto itself.
// wantedSlugs: explicit preselection (e.g. cross-link from the dataset catalog or
// deep link); without it the previous selection stays (a region switch keeps the
// previously chosen datasets if available in the new region - see
// reconcileNutsSelection).
// Region push whose completion the currentQuery watcher waits for: select*() changes
// component state synchronously and the watcher would otherwise issue its
// router.replace() while the navigation is still pending - that aborts the push and
// replaces the *previous* history entry instead (browser back then lands one view
// too far back).
let pendingRegionPush: Promise<unknown> = Promise.resolve();
function pushRegionRoute(to: Parameters<typeof router.push>[0]) {
  pendingRegionPush = router.push(to);
}

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
  nutsAgenturbezirke.value = null;
  if (navigate) {
    pushRegionRoute({ name: "regionen", params: { regionId: nutsId }, query: { type: "nuts" } });
  }
  // Don't await: independent of the dataset catalog/Gemeinden below.
  if (region.nuts_level >= 1) loadNutsAgenturbezirke(region.nuts_id);

  // GET /nuts/{id}/datasets now covers all levels: for Kreise (level 3) from
  // mart_region_datasets (Kreis-grain), for Land/Bundesland/Regierungsbezirk (levels
  // 0-2) from mart_nuts012_dataset_aggregate, which already aggregates the same
  // Kreis-grain datasets up to these levels (see DatasetPicker.vue nutsLevel prop for
  // the matching time-series query).
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/nuts/${region.nuts_id}/datasets`);
    if (selectedNuts.value !== region) return;
    nutsDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    if (selectedNuts.value === region) nutsDatasetsError.value = err instanceof Error ? err.message : String(err);
    return;
  }
  reconcileNutsSelection(wanted);

  if (region.nuts_level !== 3) return;

  // Gemeinden of this Kreis + LAU values aggregated at Kreis level - both
  // independent of the dataset selection above (level 3 only, see the earlier
  // return above).
  try {
    // /lau?nuts_parent_id= returns one row per (gisco_id, year) - dedupe to the
    // latest vintage per Gemeinde as in runLauSearch() below (see
    // dedupeByGiscoId).
    const body = await apiGet<LauRegion[]>("/lau", { nuts_parent_id: region.nuts_id, limit: "null" });
    if (selectedNuts.value !== region) return;
    nutsChildLau.value = dedupeByGiscoId(body.data ?? []);
  } catch (err) {
    if (selectedNuts.value === region) nutsChildLauError.value = err instanceof Error ? err.message : String(err);
  }
  try {
    const body = await apiGet<NutsLauAggregateResponse>(`/nuts/${region.nuts_id}/lau-aggregate`);
    if (selectedNuts.value !== region) return;
    nutsLauAggregate.value = body.data?.series ?? [];
  } catch {
    if (selectedNuts.value !== region) return;
    // 404 is the normal case as long as no LAU population data with gisco_id is
    // loaded locally (see mart_nuts3_lau_aggregate comment) - no error text, the
    // section then simply stays empty.
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
// Analogous to nutsSelectedSlugs.
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
// The search over the combined list can enable LAU via the filter afterwards while a
// search term is already set (e.g. first NUTS/Agentur, then activating LAU) -
// without this second watcher lauResults would stay empty until the search term
// changes again.
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
    pushRegionRoute({ name: "regionen", params: { regionId: giscoId }, query: { type: "lau" } });
  }
  try {
    const body = await apiGet<LauRegion>(`/lau/${giscoId}`);
    if (selectedGiscoId.value !== giscoId) return;
    selectedLau.value = body.data;
  } catch (err) {
    if (selectedGiscoId.value === giscoId) lauDetailError.value = err instanceof Error ? err.message : String(err);
    return;
  }

  const nuts3 = unwrapNull(selectedLau.value.nuts3_id);
  if (!nuts3) return;
  try {
    const body = await apiGet<DatasetCatalogResponse>(`/lau/${giscoId}/datasets`);
    if (selectedGiscoId.value !== giscoId) return;
    lauDatasets.value = body.data?.datasets ?? [];
  } catch (err) {
    if (selectedGiscoId.value === giscoId) lauDatasetsError.value = err instanceof Error ? err.message : String(err);
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
// n:m mapping to overlapping NUTS-3 regions (area/population shares), see
// mart_agentur_nuts3_crosswalk in the dbt project - a spatial estimate, not an
// official assignment.
const agenturNuts3Shares = ref<AgenturNuts3Share[] | null>(null);
const agenturNuts3SharesError = ref<string | null>(null);
let agenturLoadPromise: Promise<void> | null = null;
// Placement in the NUTS hierarchy (GET /agenturbezirke/{ba_schl}/nuts-hierarchy)
// - parent (smallest fully containing level) incl. breadcrumb.
const agenturHierarchy = ref<AgenturNutsMembership[] | null>(null);
// Agenturbezirke of the chosen NUTS region (GET /nuts/{nuts_id}/agenturbezirke).
const nutsAgenturbezirke = ref<NutsAgenturbezirk[] | null>(null);

// Origin/license/description of the AfA dataset - as with NUTS/LAU via
// DatasetPicker/DatasetInfo, loaded once here (one dataset for all Agenturbezirke,
// no switching as with DatasetPicker).
const agenturMetadata = ref<DatasetMetadataEntry | null>(null);
let agenturMetadataLoaded = false;

async function loadAgenturMetadata() {
  if (agenturMetadataLoaded) return;
  agenturMetadataLoaded = true;
  try {
    const body = await apiGet<DatasetMetadataEntry>(`/datasets/${AGENTUR_DATASET_SLUG}`);
    agenturMetadata.value = body.data ?? null;
  } catch {
    // No catalog entry - display stays with the static hint text.
  }
}

// Tabs above the chart: both metrics visible by default, the table shows only the
// columns of the chosen metric(s) (see agenturTableColumns). As with
// nutsSelectedSlugs/lauSelectedSlugs the selection persists across region switches
// and is always mirrored into the route via the currentQuery watcher (below).
const agenturSelectedMetrics = ref<AgenturMetric[]>(["arbeitslose", "unterbeschaeftigung"]);

// Derives the active tab from the current selection (for the is-active marker in
// the template) - the normal case (two metrics) is "both", anything else is
// clearly one of the two single selections.
const agenturMetricsTab = computed<AgenturMetricsTab>(() => {
  if (agenturSelectedMetrics.value.length === 2) return "both";
  return agenturSelectedMetrics.value[0] === "unterbeschaeftigung" ? "unterbeschaeftigung" : "arbeitslose";
});

function selectAgenturMetricsTab(tab: AgenturMetricsTab) {
  agenturSelectedMetrics.value = AGENTUR_METRICS_TABS.find((t) => t.value === tab)!.metrics;
}

// The promise is remembered so that a click from the NUTS panel (openAgentur) can
// also wait for the end of an already running load.
function loadAgenturbezirke(): Promise<void> {
  agenturLoadPromise ??= (async () => {
    try {
      const body = await apiGet<AgenturbezirkSummary[]>("/agenturbezirke");
      agenturbezirke.value = body.data ?? [];
    } catch (err) {
      agenturLoadError.value = err instanceof Error ? err.message : String(err);
    }
    loadAgenturMetadata();
  })();
  return agenturLoadPromise;
}

// Jump from the NUTS panel to an Agenturbezirk: the list must be loaded for this,
// even if the "Agenturbezirke" list filter is currently off.
async function openAgentur(baSchl: string) {
  await loadAgenturbezirke();
  selectAgentur(baSchl);
}

async function loadNutsAgenturbezirke(nutsId: string) {
  try {
    const body = await apiGet<NutsAgenturbezirk[]>(`/nuts/${nutsId}/agenturbezirke`);
    // The user may have selected another region in the meantime.
    if (selectedNuts.value?.nuts_id === nutsId) nutsAgenturbezirke.value = body.data ?? [];
  } catch {
    if (selectedNuts.value?.nuts_id === nutsId) nutsAgenturbezirke.value = [];
  }
}

const nutsAgenturContained = computed(() => (nutsAgenturbezirke.value ?? []).filter((a) => a.is_fully_contained));
const nutsAgenturPartial = computed(() => (nutsAgenturbezirke.value ?? []).filter((a) => !a.is_fully_contained));

// Parent of the chosen Agenturbezirk (smallest fully containing NUTS level) and -
// if no level contains it fully - the overlapping Bundesländer.
const agenturParent = computed(() => agenturHierarchy.value?.find((m) => m.is_parent) ?? null);
const agenturBreadcrumb = computed(() => {
  const parent = agenturParent.value;
  if (!parent) return [];
  const chain = nutsBreadcrumb(parent.nuts_id);
  const parentRegion = nutsById.value.get(parent.nuts_id);
  return parentRegion ? [...chain, parentRegion] : chain;
});
const agenturSpanningBundeslaender = computed(() =>
  agenturParent.value ? [] : (agenturHierarchy.value ?? []).filter((m) => m.nuts_level === 1),
);

// Agentur is loaded eagerly (like NUTS) as soon as "agentur" is among the active
// list filters - all three types are active by default, so the combined list
// usually needs the data right at mount.
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
  agenturHierarchy.value = null;
  // Unlike NUTS/LAU (see reconcileNutsSelection) there is no region-dependent
  // availability to check here - both metrics exist equally for every
  // Agenturbezirk, so an explicit selection can be adopted directly.
  if (wantedMetrics?.length) agenturSelectedMetrics.value = wantedMetrics;
  if (navigate) {
    pushRegionRoute({ name: "regionen", params: { regionId: baSchl }, query: { type: "agentur" } });
  }
  loadAgenturHierarchy(baSchl);
  try {
    const body = await apiGet<AgenturbezirkArbeitslosigkeitPoint[]>(`/agenturbezirke/${baSchl}/arbeitslosigkeit`);
    if (selectedAgentur.value === agentur) agenturPoints.value = body.data ?? [];
  } catch (err) {
    if (selectedAgentur.value === agentur) agenturPointsError.value = err instanceof Error ? err.message : String(err);
  }
  try {
    const body = await apiGet<AgenturNuts3Share[]>(`/agenturbezirke/${baSchl}/nuts3`);
    if (selectedAgentur.value === agentur) agenturNuts3Shares.value = body.data ?? [];
  } catch (err) {
    if (selectedAgentur.value === agentur) agenturNuts3SharesError.value = err instanceof Error ? err.message : String(err);
  }
}

async function loadAgenturHierarchy(baSchl: string) {
  try {
    const body = await apiGet<AgenturNutsMembership[]>(`/agenturbezirke/${baSchl}/nuts-hierarchy`);
    if (selectedAgentur.value?.ba_schl === baSchl) agenturHierarchy.value = body.data ?? [];
  } catch {
    if (selectedAgentur.value?.ba_schl === baSchl) agenturHierarchy.value = [];
  }
}

// Aggregated Kreis data for this Agenturbezirk (see
// GET /agenturbezirke/{ba_schl}/dataset-aggregate/{slug}, area-share-weighted via
// mart_agentur_nuts3_crosswalk) - "offer aggregated data in each case" for Agentur,
// analogous to the NUTS-0/1/2 dataset-aggregate and the LAU Kreis dataset catalog.
// The candidate slugs come from the largest overlapping Kreis (agenturNuts3Shares is
// already sorted by area_share_of_agentur descending, see backend) - there is no
// catalog endpoint of its own for Agentur, but Kreis-grain datasets practically
// match what is listed per Kreis under /nuts/{id}/datasets.
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
    // /nuts/{id}/datasets also lists datasets that can't be aggregated to
    // Agenturbezirke (e.g. beschaeftigungsquote-gemeinden, Gemeinde-grain) - only
    // what appears in mart_agentur_dataset_aggregate is selectable.
    const aggregatable = await loadAgenturAggregatableSlugs();
    agenturAggregateDatasets.value = (body.data?.datasets ?? []).filter((d) => aggregatable.has(d.route_slug));
    reconcileAgenturAggregateSelection(agenturAggregateSelectedSlugs.value);
  } catch (err) {
    agenturAggregateDatasetsError.value = err instanceof Error ? err.message : String(err);
  }
});

function reconcileAgenturAggregateSelection(wanted: string[]) {
  const available = new Set((agenturAggregateDatasets.value ?? []).map((d) => d.route_slug));
  // No fallback to a first dataset: the default is "no selection" = plain BA view
  // with actual values (see DatasetPicker, kind="agentur").
  agenturAggregateSelectedSlugs.value = wanted.filter((s) => available.has(s));
}

// "Unterbeschäftigung" has only been collected by the BA since 2009, "Arbeitslose"
// for longer - periods in which NONE of the currently chosen metrics has a value
// would otherwise show up as empty rows/an empty chart stretch at the start.
// Filtering is based on the current selection (not blindly on "any value present"),
// so that e.g. the pure "Unterbeschäftigung" tab starts right at 2009 instead of
// beginning with years of empty "–" rows.
const agenturVisiblePoints = computed(() => {
  if (!agenturPoints.value) return [];
  return agenturPoints.value.filter(
    (p) =>
      (agenturSelectedMetrics.value.includes("arbeitslose") && unwrapNull(p.arbeitslose) !== null) ||
      (agenturSelectedMetrics.value.includes("unterbeschaeftigung") && unwrapNull(p.unterbeschaeftigung) !== null),
  );
});

// The chart shows both metrics as separate series, toggleable per tab; the table
// shows only the columns of the chosen metric(s) (see agenturTableColumns).
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
// Monthly series for the combined chart in the DatasetPicker (next to the yearly
// Kreis aggregates) - same selection as in the chart above.
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
// The table now shows (like the chart) only the selected metrics as columns instead
// of always both - consistent with the NUTS/LAU dataset blocks, where deselected
// datasets no longer appear anywhere either.
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

// Jumps from the map or other views directly to a region (the cross-link
// "Regionen -> Karte" is the reverse, see MapView.vue). nutsLevel is only evaluated
// for geoType="nuts" (since the level switcher, MapView.vue has its own default of
// 3/Kreise there - without this parameter a jump to e.g. a Bundesland would come to
// nothing there, because the interactive map layer would still show the wrong NUTS
// level).
function viewOnMap(geoType: RegionType, geoId: string, nutsLevel?: number) {
  const query: Record<string, string> = { geo: geoType, region: geoId };
  if (geoType === "nuts" && nutsLevel !== undefined) query.nutsLevel = String(nutsLevel);
  router.push({ name: "karte", query });
}

// Combined list across all active area types (see activeTypes) - replaces the
// formerly per-type list panels. NUTS hits follow the current nutsBrowseLevel,
// Agentur is always fully loaded, LAU only from a 2-character search term (see
// combinedSearch watcher).
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

// Mirrors type filter + the currently chosen region incl. its dataset/view/metric
// selection back into the route (router.replace, so pure selection changes create
// no history entries) - replaces the formerly per-type nutsDatasetQuery-/
// lauDatasetQuery-/agenturMetricsQuery watchers with a single one, analogous to
// MapView.vue's currentQuery pattern.
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
watch([currentQuery, currentRegionId], async () => {
  await pendingRegionPush;
  // For the route.name check see the comment in applyRouteSelection() - because of
  // <KeepAlive> this watcher keeps running outside the "regionen" route too, but
  // must not navigate back there then.
  if (route.name !== "regionen") return;
  router.replace({ name: "regionen", params: { regionId: currentRegionId.value }, query: currentQuery.value });
});

// Copies type filter/regionId/type/dataset from the current route into component
// state - for deep links, browser back/forward and external cross-links (e.g. from
// the dataset catalog). navigate=false prevents the select*() functions from
// pushing the route (onto itself) again.
function applyRouteSelection() {
  // RegionBrowser stays active via <KeepAlive> in App.vue even outside the
  // "regionen" route, so its watchers keep running e.g. while navigating to the
  // map. route is (via useRoute()) the globally current route object, not "the
  // route this component was last active with" - without this check the map's own
  // ?dataset= (same query key!) would be misread here as NUTS-3 dataset selection
  // and even navigated back to /regionen via the currentQuery watcher above.
  if (route.name !== "regionen") return;
  activeTypes.value = parseTypesParam(route.query.types);

  const id = route.params.regionId as string | undefined;
  const slugsFromRoute = parseSlugsParam(route.query.dataset);
  const type = (route.query.type as RegionType | undefined) ?? (id ? inferRegionType(id) : regionType.value);

  if (type === "nuts" && route.query.level !== undefined) {
    nutsBrowseLevel.value = parseNutsLevelParam(route.query.level);
  }

  if (!id) {
    // No region switch, but dataset slugs without a region: "preselected" from the
    // dataset catalog until a region is chosen (see hint text/DatasetPicker below
    // in the template).
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
      // Region is already active (e.g. a link that only changes the dataset
      // selection for the same region) - no new fetch needed, but still adopt the
      // explicit selection from the route. Without this branch selectNuts() below
      // would not be called at all (early return), and a link with a different
      // ?dataset= to the same region would have no effect.
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
      // Analogous to above - e.g. the map sidebar link to the same Agenturbezirk,
      // but with a different metric.
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

// --- Download dialog -------------------------------------------------------
// One dialog for all three region types; which region/datasets it offers is derived
// from the currently open detail panel. The region's selection is preselected
// (see preselected-slugs in the template).
const downloadOpen = ref(false);

// BA monthly values (Agentur) and Gemeinde rollups (Kreis) are not catalog
// datasets, but carry slugs of their own in the export (see region_data.go).
const AGENTUR_RAW_OPTIONS: ExtraDownloadOption[] = [
  { slug: "ba_arbeitslose", label: "Arbeitslose (BA, monatlich)", scope: "raw" },
  { slug: "ba_unterbeschaeftigung", label: "Unterbeschäftigung (BA, monatlich)", scope: "raw" },
];
const NUTS3_LAU_AGGREGATE_OPTIONS: ExtraDownloadOption[] = [
  { slug: "lau_population_sum", label: "Bevölkerung der Gemeinden (Summe)", scope: "aggregated" },
  {
    slug: "lau_beschaeftigungsquote_weighted_avg",
    label: "Beschäftigungsquote der Gemeinden (gewichtet)",
    scope: "aggregated",
  },
];

// NUTS-0/1/2 have no Kreis rows of their own (only aggregates), LAU conversely no
// aggregated values - see GET /regions/{type}/{id}/data.
const downloadTarget = computed(() => {
  if (regionType.value === "nuts" && selectedNuts.value) {
    const isKreis = selectedNuts.value.nuts_level === 3;
    return {
      regionType: "nuts" as const,
      regionId: selectedNuts.value.nuts_id,
      regionLabel: selectedNuts.value.region_name,
      datasets: nutsDatasets.value ?? [],
      preselectedSlugs: nutsSelectedSlugs.value,
      extraOptions: isKreis ? NUTS3_LAU_AGGREGATE_OPTIONS : [],
      availableScopes: isKreis ? (["raw", "aggregated"] as const) : (["aggregated"] as const),
    };
  }
  if (regionType.value === "lau" && selectedLau.value && selectedGiscoId.value) {
    return {
      regionType: "lau" as const,
      regionId: selectedGiscoId.value,
      regionLabel: selectedLau.value.lau_name ?? selectedGiscoId.value,
      datasets: lauDatasets.value ?? [],
      preselectedSlugs: lauSelectedSlugs.value,
      extraOptions: [],
      availableScopes: ["raw"] as const,
    };
  }
  if (regionType.value === "agentur" && selectedAgentur.value) {
    return {
      regionType: "agentur" as const,
      regionId: selectedAgentur.value.ba_schl,
      regionLabel: selectedAgentur.value.name ?? selectedAgentur.value.ba_schl,
      datasets: agenturAggregateDatasets.value ?? [],
      // The BA metrics visible in the panel plus the chosen Kreis aggregates.
      preselectedSlugs: [
        ...agenturSelectedMetrics.value.map((m) => (m === "arbeitslose" ? "ba_arbeitslose" : "ba_unterbeschaeftigung")),
        ...agenturAggregateSelectedSlugs.value,
      ],
      extraOptions: AGENTUR_RAW_OPTIONS,
      availableScopes: ["raw", "aggregated"] as const,
    };
  }
  return null;
});
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
              type="button"
              class="link-button"
              @click="viewOnMap('nuts', selectedNuts.nuts_id, selectedNuts.nuts_level)"
            >
              Auf der Karte anzeigen ↗
            </button>
            <button type="button" class="link-button" @click="downloadOpen = true">
              Daten herunterladen ↓
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

          <template v-if="selectedNuts.nuts_level >= 1 && nutsAgenturbezirke && nutsAgenturbezirke.length">
            <template v-if="nutsAgenturContained.length">
              <h3>Agenturbezirke in dieser Region</h3>
              <p class="hint">
                Agenturbezirke der Bundesagentur für Arbeit, die vollständig in dieser Region liegen (räumlich
                ermittelt, keine amtliche Zuordnung).
              </p>
              <ul class="child-region-list">
                <li v-for="a in nutsAgenturContained" :key="a.ba_schl">
                  <button type="button" class="link-button" @click="openAgentur(a.ba_schl)">
                    {{ a.name ?? a.ba_schl }} <span class="region-code">BA {{ a.ba_schl }}</span>
                  </button>
                </li>
              </ul>
            </template>
            <template v-if="nutsAgenturPartial.length">
              <h3>Teilweise überlappende Agenturbezirke</h3>
              <p class="hint">
                Diese Agenturbezirke reichen über die Region hinaus; angegeben ist, welcher Anteil ihrer Fläche hier
                liegt.
              </p>
              <ul class="child-region-list">
                <li v-for="a in nutsAgenturPartial" :key="a.ba_schl">
                  <button type="button" class="link-button" @click="openAgentur(a.ba_schl)">
                    {{ a.name ?? a.ba_schl }} <span class="region-code">BA {{ a.ba_schl }}</span>
                  </button>
                  ({{ formatPercent(a.area_share_of_agentur) }})
                </li>
              </ul>
            </template>
          </template>

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
            <button type="button" class="link-button" @click="downloadOpen = true">
              Daten herunterladen ↓
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
          <nav v-if="agenturBreadcrumb.length" class="breadcrumb" aria-label="Übergeordnete Regionen">
            <template v-for="ancestor in agenturBreadcrumb" :key="ancestor.nuts_id">
              <button type="button" class="link-button" @click="selectNuts(ancestor.nuts_id)">
                {{ ancestor.region_name }}
              </button>
              <span class="breadcrumb-sep">›</span>
            </template>
          </nav>
          <p v-else-if="agenturSpanningBundeslaender.length" class="hint">
            Dieser Agenturbezirk liegt nicht vollständig in einem Bundesland, sondern verteilt sich auf:
            <template v-for="(m, i) in agenturSpanningBundeslaender" :key="m.nuts_id">
              <template v-if="i > 0">, </template>
              <button type="button" class="link-button" @click="selectNuts(m.nuts_id)">
                {{ m.region_name ?? m.nuts_id }}
              </button>
              ({{ formatPercent(m.area_share_of_agentur) }})
            </template>
          </p>
          <div class="detail-header">
            <h2>{{ selectedAgentur.name }}</h2>
            <span class="region-code">BA {{ selectedAgentur.ba_schl }}</span>
            <button type="button" class="link-button" @click="viewOnMap('agentur', selectedAgentur.ba_schl)">
              Auf der Karte anzeigen ↗
            </button>
            <button type="button" class="link-button" @click="downloadOpen = true">
              Daten herunterladen ↓
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
            <!-- No Kreis dataset selected: plain BA view (actual values) -->
            <DatasetInfo v-if="agenturMetadata" :metadata="agenturMetadata" />
            <TimeseriesChart v-if="agenturChartReady" :period-series="agenturChartSeries" />
            <DataTable v-if="agenturChartReady" :period-rows="agenturTableRows" :period-columns="agenturTableColumns" />
            <!-- Combined view: BA monthly values as first tab next to the Kreis datasets -->
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

    <DownloadDialog
      v-if="downloadTarget"
      :open="downloadOpen"
      :region-type="downloadTarget.regionType"
      :region-id="downloadTarget.regionId"
      :region-label="downloadTarget.regionLabel"
      :datasets="downloadTarget.datasets"
      :preselected-slugs="downloadTarget.preselectedSlugs"
      :extra-options="downloadTarget.extraOptions"
      :available-scopes="[...downloadTarget.availableScopes]"
      @close="downloadOpen = false"
    />
  </div>
</template>
