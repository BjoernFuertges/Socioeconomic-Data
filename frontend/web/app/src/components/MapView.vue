<script setup lang="ts">
import { computed, onActivated, onMounted, onUnmounted, ref, watch } from "vue";
import type { RouteLocationRaw } from "vue-router";
import { useRoute, useRouter } from "vue-router";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { apiGet, unwrapNull } from "../api/client";
import AgenturAggregationNote from "./AgenturAggregationNote.vue";
import type {
  AgenturbezirkLatestValue,
  AgenturDatasetAggregatePoint,
  AgenturDatasetAggregateYear,
  AgenturbezirkPeriod,
  GeoRegionSummary,
  InventoryEntry,
  LauTimeseriesPoint,
  NutsTimeseriesPoint,
  TimeseriesPoint,
} from "../types/api";
import { formatDatasetLabel, formatMonthLabel, formatNumber } from "../format";
import { AGENTUR_DATASET_SLUG } from "../constants";
import {
  MAP_ONLY_FILL_DARK,
  MAP_ONLY_FILL_LIGHT,
  NO_DATA_FILL_DARK,
  NO_DATA_FILL_LIGHT,
  sequentialColor,
  sequentialSteps,
} from "../mapColor";

const route = useRoute();
const router = useRouter();

// Kein externer Tile-Anbieter: statt Kacheln aus API, Bundeslandgrenzen aus der eigenen, historisierten NUTS-Geometrie gezeichnet
// (NUTS-0 = Staatsgrenze, NUTS-1 = Bundesländer in Deutschland), keine Ortsnamen oder andere Kartenbeschriftungen
const BOUNDARY_COLOR_LIGHT = "#1a1a1a";
const BOUNDARY_COLOR_DARK = "#f5f5f5";

// Unterstützte NUTS-3-Zielversionen - muss mit supportedNutsVersions in
// backend/internal/handlers/kreis_generic.go übereinstimmen. Entspricht den
// in stg_nuts_geometry vorhandenen Vintage-Jahren; mart_nuts_version_crosswalk
// berechnet für jedes Paar davon eine direkte (1-Hop) Abbildung.
const NUTS_VERSIONS = [2003, 2006, 2010, 2013, 2016, 2021, 2024] as const;

// Unterstützte LAU-Zielversionen - muss mit supportedLauVersions in
// backend/internal/handlers/lau_generic.go übereinstimmen. Entspricht den
// in mart_lau_geometry_historized vorhandenen Vintage-Jahren;
// mart_lau_version_crosswalk berechnet für jedes Paar davon eine Abbildung.
const LAU_VERSIONS = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024] as const;

type GeoType = "nuts" | "lau" | "agentur";

// Agenturbezirke haben (anders als NUTS/LAU) keine Zeitreihen-/Interpolations-
// Mart, keinen Datensatz-Slug und keine Geometrie-Versionen (nur eine
// aktuelle Grenzen-Vintage) - statt Datensatz+Version wählt man hier direkt
// zwischen den zwei verfügbaren Metriken sowie Jahr+Monat (monatliche
// Berichtsdaten, siehe mart_agenturbezirk_sociodata.sql).
type AgenturMetric = "arbeitslose" | "unterbeschaeftigung";
const AGENTUR_METRIC_LABELS: Record<AgenturMetric, string> = {
  arbeitslose: "Arbeitslose",
  unterbeschaeftigung: "Unterbeschäftigung",
};
const selectedAgenturMetric = ref<AgenturMetric>("arbeitslose");
// Rohdaten von /agenturbezirke/period (Geometrie + beide Metriken für den
// gewählten Berichtsmonat) - beim Metrik-Wechsel wird daraus nur
// valuesByRegion neu abgeleitet, kein erneuter Request nötig.
const agenturLatestRaw = ref<AgenturbezirkLatestValue[]>([]);

// Alternativ zu den BA-Monatswerten (arbeitslose/unterbeschaeftigung) können
// auch die auf Agenturbezirke aggregierten Kreis-Datensätze eingefärbt werden
// (GET /agenturbezirke/dataset-aggregate[/{slug}], flächenanteilsgewichtet über
// mart_agentur_dataset_aggregate). "" = BA-Monatswerte (Metrik + Jahr/Monat),
// sonst der Slug des aggregierten Datensatzes (dann nur Jahr, kein Monat).
// Die Geometrie stammt in beiden Fällen aus /agenturbezirke/period.
const selectedAgenturDataset = ref<string>("");
const selectedAgenturAggYear = ref<number | null>(null);
// dataset-slug -> verfügbare Jahre (absteigend)
const agenturAggYears = ref<Map<string, number[]>>(new Map());
const agenturAggDatasetOptions = computed(() => Array.from(agenturAggYears.value.keys()).sort());
const agenturAggError = ref<string | null>(null);
const agenturAggYearOptions = computed(() => agenturAggYears.value.get(selectedAgenturDataset.value) ?? []);

// Verfügbare Berichtsmonate (von /agenturbezirke/periods, neueste zuerst) -
// füllt die Jahr/Monat-Auswahl für Agenturbezirke, analog zur Jahres-
// /Versionsauswahl bei NUTS/LAU. Agenturbezirke führen nur eine aktuelle
// Grenzen-Vintage (siehe mart_agenturbezirk_sociodata.sql), deshalb gibt es
// hier - anders als NUTS-/LAU-Version - keine Versionsauswahl.
const agenturPeriodsError = ref<string | null>(null);
const agenturPeriods = ref<AgenturbezirkPeriod[]>([]);
const selectedAgenturYear = ref<number | null>(null);
const selectedAgenturMonth = ref<number | null>(null);
let agenturPeriodsLoaded = false;
// Verhindert, dass die Erstbefüllung von selectedAgenturYear/-Month (unten)
// den watch([selectedAgenturYear, selectedAgenturMonth])-Handler zusätzlich
// zum ohnehin laufenden loadGeometries()-Aufruf auslöst (der diese Werte
// gerade erst über loadAgenturPeriods() gesetzt hat) - sonst liefe der
// Period-Request beim ersten Wechsel auf "agentur" doppelt.
let suppressAgenturPeriodWatch = false;

const agenturYearOptions = computed(() =>
  Array.from(new Set(agenturPeriods.value.map((p) => p.year))).sort((a, b) => b - a),
);
const agenturMonthOptions = computed(() =>
  agenturPeriods.value
    .filter((p) => p.year === selectedAgenturYear.value)
    .map((p) => p.month)
    .sort((a, b) => a - b),
);

async function loadAgenturPeriods() {
  if (agenturPeriodsLoaded) return;
  agenturPeriodsLoaded = true;
  agenturPeriodsError.value = null;
  try {
    const body = await apiGet<AgenturbezirkPeriod[]>("/agenturbezirke/periods");
    agenturPeriods.value = (body.data ?? []).slice().sort((a, b) => b.year - a.year || b.month - a.month);
    const latest = agenturPeriods.value[0];
    if (latest) {
      suppressAgenturPeriodWatch = true;
      selectedAgenturYear.value = latest.year;
      selectedAgenturMonth.value = latest.month;
    }
  } catch (err) {
    agenturPeriodsError.value = err instanceof Error ? err.message : String(err);
  }
}

// Minimale Teilmenge von GeoRegionSummary, die buildLayer()/styleForFeature()
// tatsächlich brauchen - GeoRegionSummary[] (NUTS/LAU, aus /geo) ist
// strukturell kompatibel und damit weiterhin direkt zuweisbar.
type MapRegion = { geo_id: string; region_name: string; geometry_wgs84?: GeoJSON.Geometry | null };

const mapContainer = ref<HTMLDivElement | null>(null);
let map: L.Map | null = null;
let geoJsonLayer: L.GeoJSON | null = null;
let boundaryLayer: L.GeoJSON | null = null;
let lastBounds: L.LatLngBounds | null = null;

const isDark = ref(window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);

const geoError = ref<string | null>(null);
const regions = ref<MapRegion[]>([]);

const boundaryError = ref<string | null>(null);
const boundaries = ref<GeoRegionSummary[]>([]);

const inventoryError = ref<string | null>(null);
// dataset-slug -> verfügbare Jahre (absteigend, neuestes zuerst), getrennt je
// Geo-Ebene (/socioeconomic/inventory liefert NUTS_3- und LAU-Einträge
// getrennt, und /{slug}/timeseries erwartet je nach Ebene nuts_version oder
// lau_version).
const datasetYearsNuts = ref<Map<string, number[]>>(new Map());
const datasetYearsLau = ref<Map<string, number[]>>(new Map());
// "" = kein Datensatz gewählt -> Karte zeigt nur den Grenzverlauf, keine Werte.
const selectedDataset = ref<string>("");
const selectedYear = ref<number | null>(null);
const selectedNutsVersion = ref<number>(2024);
const selectedLauVersion = ref<number>(2024);
const selectedGeoType = ref<GeoType>("nuts");

const valuesLoading = ref(false);
const valuesError = ref<string | null>(null);
// source_region_count nur bei aggregierten Agentur-Datensätzen gesetzt.
type MapValue = TimeseriesPoint & {
  source_region_count?: number;
  aggregation_method?: string;
  value_kind?: string;
};
const valuesByRegion = ref<Map<string, MapValue>>(new Map());

const datasetYears = computed(() => (selectedGeoType.value === "lau" ? datasetYearsLau.value : datasetYearsNuts.value));
const datasetOptions = computed(() => Array.from(datasetYears.value.keys()).sort());
const yearOptions = computed(() => datasetYears.value.get(selectedDataset.value) ?? []);

const valueDomain = computed<[number, number] | null>(() => {
  const values = Array.from(valuesByRegion.value.values()).map((p) => p.value);
  if (!values.length) return null;
  return [Math.min(...values), Math.max(...values)];
});

function colorForValue(value: number): string {
  const domain = valueDomain.value;
  if (!domain) return isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT;
  const [min, max] = domain;
  const t = max === min ? 0.5 : (value - min) / (max - min);
  return sequentialColor(t, isDark.value);
}

// Cross-Link "Regionen -> Karte" (RegionBrowser.vue: viewOnMap()): Region-ID
// aus ?region=, um sie beim Ankommen anzufahren (siehe focusOnRegion()) und -
// solange nichts gehovert/angeklickt ist - in der Seitenbox zu zeigen (siehe
// sidebarRegionId).
const highlightedRegionId = computed(() => (route.query.region as string | undefined) || null);
const HIGHLIGHT_COLOR = "#e8a33d";

// Statt eines Leaflet-Hover-Tooltips (siehe Kommentar an buildLayer()) zeigt
// eine Seitenbox die Daten der "aktiven" Region: live beim Hovern, sonst die
// zuletzt angeklickte ("angepinnte") Region, sonst - direkt nach einem
// Cross-Link-Sprung von Regionen -> Karte - die per ?region= angefahrene
// Region. So bleibt die Box beim genauen Lesen/Verlinken stabil, statt beim
// Wegbewegen der Maus sofort zu verschwinden.
const hoveredRegionId = ref<string | null>(null);
const pinnedRegionId = ref<string | null>(null);
const sidebarRegionId = computed(() => hoveredRegionId.value ?? pinnedRegionId.value ?? highlightedRegionId.value);

function styleForFeature(feature?: GeoJSON.Feature): L.PathOptions {
  const geoId = feature?.properties?.geo_id as string | undefined;
  const point = geoId ? valuesByRegion.value.get(geoId) : undefined;
  const borderColor = isDark.value ? "#0d0d0d" : "#ffffff";
  const isHighlighted = geoId !== undefined && geoId === sidebarRegionId.value;

  if (selectedGeoType.value !== "agentur" && !selectedDataset.value) {
    // "Kein Datensatz (nur Karte)" bzw. LAU-Ansicht: eigener, kontrastreicher
    // Stil statt des dezenten "keine Daten"-Grautons weiter unten - der wäre
    // auf dem dunklen Kartenhintergrund kaum sichtbar, wenn ihn ausnahmslos
    // jede Region bekommt (siehe MAP_ONLY_FILL_* in mapColor.ts).
    // dashArray explizit zurücksetzen: Leaflet's setStyle() merged Optionen nur
    // (setOptions kopiert per for...in nur vorhandene Keys) statt sie zu
    // ersetzen - ohne diese Zeile bliebe der gestrichelte Rand von zuvor
    // interpolierten Regionen (aus einem vorher gewählten Datensatz) hängen.
    return {
      fillColor: isDark.value ? MAP_ONLY_FILL_DARK : MAP_ONLY_FILL_LIGHT,
      fillOpacity: 0.75,
      color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
      weight: isHighlighted ? 3 : 1,
      dashArray: undefined,
    };
  }

  if (!point) {
    return {
      fillColor: isDark.value ? NO_DATA_FILL_DARK : NO_DATA_FILL_LIGHT,
      fillOpacity: 0.6,
      color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
      weight: isHighlighted ? 3 : 1,
      dashArray: undefined,
    };
  }
  return {
    fillColor: colorForValue(point.value),
    fillOpacity: 0.85,
    color: isHighlighted ? HIGHLIGHT_COLOR : borderColor,
    weight: isHighlighted ? 3 : point.is_interpolated ? 1 : 1.5,
    dashArray: point.is_interpolated ? "3 2" : undefined,
  };
}

function restyleLayer() {
  geoJsonLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature };
    l.setStyle(styleForFeature(l.feature));
  });
}

// "3D"-Hebe-Effekt für die aktive Region (siehe sidebarRegionId): zwei
// gestapelte drop-shadows (eng+dunkel, weit+weich) lassen die Fläche wie eine
// angehobene Karte wirken - veranschaulicht sowohl die Hover-Vorschau als
// auch den Sprung über einen Cross-Link. L.PathOptions kennt kein
// CSS-Filter, deshalb direkter Zugriff auf das von Leaflets SVG-Renderer
// erzeugte <path>-Element (_path ist intern, aber wie an anderen
// Leaflet-Interna in dieser Datei - siehe _tryAnimatedZoom-Kommentare - der
// einzige Weg dafür).
const LIFT_SHADOW_LIGHT = "drop-shadow(0 4px 4px rgba(0,0,0,0.3)) drop-shadow(0 10px 14px rgba(0,0,0,0.2))";
const LIFT_SHADOW_DARK = "drop-shadow(0 4px 4px rgba(0,0,0,0.6)) drop-shadow(0 10px 14px rgba(0,0,0,0.4))";

function applyLiftEffect() {
  const activeId = sidebarRegionId.value;
  geoJsonLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature; _path?: SVGPathElement };
    const isActive = activeId !== null && l.feature?.properties?.geo_id === activeId;
    if (l._path) l._path.style.filter = isActive ? (isDark.value ? LIFT_SHADOW_DARK : LIFT_SHADOW_LIGHT) : "";
    if (isActive) l.bringToFront();
  });
  boundaryLayer?.bringToFront();
}

// Daten für die Seitenbox - dieselbe valuesByRegion-Map wie für die
// Einfärbung, deshalb identisch für NUTS/LAU (gewählter Datensatz) und
// Agentur (gewählte Metrik).
const sidebarRegion = computed(() => {
  const id = sidebarRegionId.value;
  if (!id) return null;
  const region = regions.value.find((r) => r.geo_id === id);
  if (!region) return null;
  return { id, name: region.region_name, point: valuesByRegion.value.get(id) ?? null };
});

const sidebarValueLabel = computed(() => {
  if (selectedGeoType.value !== "agentur") return "Wert";
  return selectedAgenturDataset.value
    ? formatDatasetLabel(selectedAgenturDataset.value)
    : AGENTUR_METRIC_LABELS[selectedAgenturMetric.value];
});

// Cross-Link "Karte -> Regionen" (Umkehrung von RegionBrowser.vue: viewOnMap()) -
// springt zur Detailansicht der in der Seitenbox gezeigten Region, mit dem
// gerade auf der Karte gewählten Datensatz vorausgewählt (siehe
// RegionBrowser.vue: nutsSelectedSlugs/lauSelectedSlugs via ?dataset=).
// Agenturbezirke haben keinen Datensatz-Picker (siehe dort).
const sidebarLinkTarget = computed<RouteLocationRaw | null>(() => {
  const id = sidebarRegionId.value;
  if (!id) return null;
  if (selectedGeoType.value === "agentur") {
    // Analog zu unten (dataset=) - übernimmt die auf der Karte gerade
    // gewählte Metrik, damit RegionBrowser.vue (agenturSelectedMetrics)
    // dieselbe Ansicht zeigt statt auf die dortige Standardauswahl
    // (beide Metriken) zurückzufallen.
    return {
      name: "regionen",
      params: { regionType: "agentur", regionId: id },
      query: selectedAgenturDataset.value ? {} : { metrics: selectedAgenturMetric.value },
    };
  }
  return {
    name: "regionen",
    params: { regionType: selectedGeoType.value, regionId: id },
    query: selectedDataset.value ? { dataset: selectedDataset.value } : {},
  };
});

function styleForBoundary(feature?: GeoJSON.Feature): L.PathOptions {
  const level = feature?.properties?.nuts_level as number | undefined;
  return {
    fill: false,
    color: isDark.value ? BOUNDARY_COLOR_DARK : BOUNDARY_COLOR_LIGHT,
    weight: level === 0 ? 2.5 : 1.25,
    opacity: level === 0 ? 0.9 : 0.6,
  };
}

function restyleBoundaryLayer() {
  boundaryLayer?.eachLayer((layer) => {
    const l = layer as L.Path & { feature?: GeoJSON.Feature };
    l.setStyle(styleForBoundary(l.feature));
  });
}

// Staats- (NUTS-0) und Bundeslandgrenzen (NUTS-1) laden - unabhängig von der
// gewählten Ebene (NUTS/LAU), damit die Karte auch in der LAU-Ansicht eine
// geografische Orientierung ohne Ortsnamen bietet.
async function loadBoundaries() {
  boundaryError.value = null;
  try {
    const [countries, states] = await Promise.all(
      [0, 1].map((level) =>
        apiGet<GeoRegionSummary[]>("/geo", {
          geo_type: "nuts",
          level,
          nuts_parent_id: "DE",
          include_geometry: true,
          geometry_strategy: "exact",
          geometry_year: selectedNutsVersion.value,
          limit: "null",
        }),
      ),
    );
    boundaries.value = [...(countries.data ?? []), ...(states.data ?? [])].filter((r) => r.geometry_wgs84);
  } catch (err) {
    boundaryError.value = err instanceof Error ? err.message : String(err);
  }
}

function buildBoundaryLayer() {
  if (!map) return;
  if (boundaryLayer) {
    map.removeLayer(boundaryLayer);
    boundaryLayer = null;
  }
  if (!boundaries.value.length) return;
  const featureCollection: GeoJSON.FeatureCollection = {
    type: "FeatureCollection",
    features: boundaries.value.map((r) => ({
      type: "Feature",
      properties: { nuts_level: r.nuts_level },
      geometry: r.geometry_wgs84 as GeoJSON.Geometry,
    })),
  };
  boundaryLayer = L.geoJSON(featureCollection, {
    style: (feature) => styleForBoundary(feature),
    interactive: false,
  }).addTo(map);
  // Muss über der (gefüllten) Regionen-Ebene liegen, sonst verschwinden die
  // Grenzlinien unter deren Füllfarbe.
  boundaryLayer.bringToFront();
}

async function loadGeometries() {
  geoError.value = null;
  try {
    if (selectedGeoType.value === "agentur") {
      // Jahr/Monat-Auswahl erst nach dem ersten Laden der verfügbaren
      // Berichtsmonate möglich (siehe loadAgenturPeriods) - deshalb hier
      // abwarten, bevor der gewählte (oder default: neueste) Zeitpunkt
      // abgefragt wird.
      await loadAgenturPeriods();
      if (selectedAgenturYear.value === null || selectedAgenturMonth.value === null) {
        agenturLatestRaw.value = [];
        regions.value = [];
        return;
      }
      // Ein Request liefert Geometrie UND den Wert des gewählten
      // Berichtsmonats in einem Schritt (anders als der getrennte /geo +
      // /{dataset}/timeseries-Pfad bei NUTS/LAU).
      const body = await apiGet<AgenturbezirkLatestValue[]>("/agenturbezirke/period", {
        year: selectedAgenturYear.value,
        month: selectedAgenturMonth.value,
      });
      agenturLatestRaw.value = (body.data ?? []).filter((a) => a.geometry_wgs84);
      regions.value = agenturLatestRaw.value.map((a) => ({
        geo_id: a.ba_schl,
        region_name: a.name,
        geometry_wgs84: a.geometry_wgs84,
      }));
      return;
    }

    if (selectedGeoType.value === "lau") {
      // geometry_year pinnt auf den Grenzverlauf der gewählten LAU-Version -
      // /geo?geo_type=lau interpretiert geometry_year dabei als Jahresfilter
      // auf lau_integrated (siehe backend/internal/handlers/geo.go).
      // country_code grenzt auf Deutschland ein.
      const body = await apiGet<GeoRegionSummary[]>("/geo", {
        geo_type: "lau",
        country_code: "DE",
        include_geometry: true,
        geometry_year: selectedLauVersion.value,
        limit: "null",
      });
      regions.value = (body.data ?? []).filter((r) => r.geometry_wgs84);
      return;
    }

    // country_code wird laut API nur für geo_type=lau ausgewertet; für NUTS
    // grenzt nuts_parent_id (Präfix-Filter) auf Deutschland ein.
    // geometry_strategy=exact+geometry_year pinnt auf den Grenzverlauf der
    // gewählten NUTS-Version (siehe mart_nuts_geometry_historized) - damit
    // liefert /geo genau die Kreise, die in dieser Version gültig waren.
    const body = await apiGet<GeoRegionSummary[]>("/geo", {
      geo_type: "nuts",
      level: 3,
      nuts_parent_id: "DE",
      include_geometry: true,
      geometry_strategy: "exact",
      geometry_year: selectedNutsVersion.value,
      limit: "null",
    });
    regions.value = (body.data ?? []).filter((r) => r.geometry_wgs84);
  } catch (err) {
    geoError.value = err instanceof Error ? err.message : String(err);
  }
}

// Lädt die Datensatz-Übersicht für eine Geo-Ebene und wählt (falls noch
// nichts gewählt ist oder die bisherige Auswahl in dieser Ebene nicht
// existiert) den ersten verfügbaren Datensatz vor.
async function loadInventoryFor(geoType: GeoType) {
  const geodataLevel = geoType === "lau" ? "LAU" : "NUTS_3";
  const target = geoType === "lau" ? datasetYearsLau : datasetYearsNuts;
  const body = await apiGet<InventoryEntry[]>("/socioeconomic/inventory", { geodata_level: geodataLevel });
  const byDataset = new Map<string, Set<number>>();
  for (const entry of body.data ?? []) {
    if (!byDataset.has(entry.dataset)) byDataset.set(entry.dataset, new Set());
    byDataset.get(entry.dataset)!.add(entry.year);
  }
  const sorted = new Map<string, number[]>();
  for (const [slug, years] of byDataset) {
    sorted.set(slug, Array.from(years).sort((a, b) => b - a));
  }
  target.value = sorted;
}

async function loadInventory() {
  try {
    await Promise.all([loadInventoryFor("nuts"), loadInventoryFor("lau")]);
    if (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value)) {
      const firstDataset = Array.from(datasetYears.value.keys()).sort()[0];
      selectedDataset.value = firstDataset ?? "";
      selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
    }
  } catch (err) {
    inventoryError.value = err instanceof Error ? err.message : String(err);
  }
}

// Leitet valuesByRegion aus dem gecachten /agenturbezirke/latest-Response
// und der gewählten Metrik ab - kein Netzwerk-Request, damit ein reiner
// Metrik-Wechsel sofort reagiert.
function applyAgenturValues() {
  valuesByRegion.value = new Map(
    agenturLatestRaw.value.flatMap((a) => {
      const raw = selectedAgenturMetric.value === "arbeitslose" ? a.arbeitslose : a.unterbeschaeftigung;
      const value = unwrapNull(raw);
      if (value === null) return [];
      // is_interpolated ist für Agenturbezirke immer false: es gibt (anders
      // als für NUTS/LAU) noch keine Gap-Filling-/Interpolations-Mart.
      const point: TimeseriesPoint = { year: a.year, value, is_interpolated: false };
      return [[a.ba_schl, point]] as [string, TimeseriesPoint][];
    }),
  );
  restyleLayer();
}

// Verfügbare aggregierte Datensätze/Jahre für Agenturbezirke; wählt (falls
// per Route ein Datensatz vorgegeben wurde) ein gültiges Jahr vor.
async function loadAgenturAggInventory() {
  const body = await apiGet<AgenturDatasetAggregateYear[]>("/agenturbezirke/dataset-aggregate");
  const byDataset = new Map<string, number[]>();
  for (const row of body.data ?? []) {
    if (!byDataset.has(row.dataset_slug)) byDataset.set(row.dataset_slug, []);
    byDataset.get(row.dataset_slug)!.push(row.year);
  }
  for (const years of byDataset.values()) years.sort((a, b) => b - a);
  agenturAggYears.value = byDataset;
  ensureAgenturAggSelection();
}

// Verwirft einen unbekannten Datensatz und ergänzt ein fehlendes/ungültiges
// Jahr durch das neueste verfügbare.
function ensureAgenturAggSelection() {
  const slug = selectedAgenturDataset.value;
  if (!slug) return;
  const years = agenturAggYears.value.get(slug);
  if (!years) {
    if (agenturAggYears.value.size > 0) selectedAgenturDataset.value = "";
    return;
  }
  if (selectedAgenturAggYear.value === null || !years.includes(selectedAgenturAggYear.value)) {
    selectedAgenturAggYear.value = years[0] ?? null;
  }
}

async function loadAgenturAggValues() {
  const slug = selectedAgenturDataset.value;
  const year = selectedAgenturAggYear.value;
  if (!slug || year === null) {
    valuesByRegion.value = new Map();
    restyleLayer();
    return;
  }
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    const body = await apiGet<AgenturDatasetAggregatePoint[]>(`/agenturbezirke/dataset-aggregate/${slug}`, { year });
    // Antwort könnte durch schnelle Auswahlwechsel überholt sein.
    if (slug !== selectedAgenturDataset.value || year !== selectedAgenturAggYear.value) return;
    valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.ba_schl, p]));
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

async function loadValues() {
  if (selectedGeoType.value === "agentur") {
    if (selectedAgenturDataset.value) await loadAgenturAggValues();
    else applyAgenturValues();
    return;
  }
  if (!selectedDataset.value || selectedYear.value === null) {
    // Kein Datensatz gewählt -> Karte bleibt ungefärbt, eventuell noch
    // vorhandene Werte des zuvor gewählten Datensatzes verwerfen.
    valuesError.value = null;
    if (valuesByRegion.value.size > 0) {
      valuesByRegion.value = new Map();
      restyleLayer();
    }
    return;
  }
  valuesLoading.value = true;
  valuesError.value = null;
  try {
    if (selectedGeoType.value === "lau") {
      const body = await apiGet<LauTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        lau_version: selectedLauVersion.value,
        limit: "null",
      });
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.gisco_id, p]));
    } else {
      const body = await apiGet<NutsTimeseriesPoint[]>(`/${selectedDataset.value}/timeseries`, {
        year: selectedYear.value,
        nuts_version: selectedNutsVersion.value,
        limit: "null",
      });
      valuesByRegion.value = new Map((body.data ?? []).map((p) => [p.nuts_id, p]));
    }
    restyleLayer();
  } catch (err) {
    valuesError.value = err instanceof Error ? err.message : String(err);
  } finally {
    valuesLoading.value = false;
  }
}

function buildLayer() {
  if (!map) return;
  // Alte Ebene immer zuerst entfernen, auch wenn die neue Region-Liste leer
  // ist - sonst bliebe bei einem Versions-/Ebenenwechsel ohne Treffer die
  // vorherige Kartenebene (Grenzen + Einfärbung) fälschlich stehen.
  if (geoJsonLayer) {
    map.removeLayer(geoJsonLayer);
    geoJsonLayer = null;
  }
  if (!regions.value.length) return;
  const featureCollection: GeoJSON.FeatureCollection = {
    type: "FeatureCollection",
    features: regions.value.map((r) => ({
      type: "Feature",
      properties: { geo_id: r.geo_id, region_name: r.region_name },
      geometry: r.geometry_wgs84 as GeoJSON.Geometry,
    })),
  };

  geoJsonLayer = L.geoJSON(featureCollection, {
    style: (feature) => styleForFeature(feature),
    // Statt eines Leaflet-Tooltips (siehe sidebarRegionId-Kommentar oben)
    // steuern Hover/Klick nur noch die Seitenbox; ein Klick "pinnt" die
    // Region, damit die Box beim Lesen/Verlinken stabil bleibt, statt beim
    // Verlassen der Fläche sofort zu verschwinden.
    onEachFeature: (feature, layer) => {
      const geoId = feature.properties?.geo_id as string | undefined;
      layer.on({
        mouseover: () => {
          hoveredRegionId.value = geoId ?? null;
        },
        mouseout: () => {
          hoveredRegionId.value = null;
        },
        click: () => {
          pinnedRegionId.value = geoId ?? null;
        },
      });
    },
  }).addTo(map);
  applyLiftEffect();

  const bounds = geoJsonLayer.getBounds();
  if (bounds.isValid()) {
    lastBounds = bounds;
    // Der Karten-Tab ist ggf. noch per v-show ausgeblendet (Container-Größe
    // 0x0) - fitBounds() würde dann mit einer Zoomstufe rechnen, die die
    // interne Kartenprojektion dauerhaft korrumpiert (SVG-Layer bleiben bei
    // "M0 0" hängen, spätere invalidateSize()-Aufrufe werfen). Der
    // ResizeObserver unten holt das fitBounds nach, sobald der Container
    // sichtbar wird.
    const size = map.getSize();
    if (size.x > 0 && size.y > 0) {
      // animate: false - siehe Kommentar in focusOnRegion(): läuft hier noch
      // eine animierte Zoom-Transition von einem vorherigen fitBounds() (z.B.
      // Ebenen-/Versionswechsel kurz hintereinander), würde Leaflet
      // (_tryAnimatedZoom: `if (this._animatingZoom) return true`) diesen
      // Aufruf sonst stillschweigend verwerfen, ohne die neue Ansicht je
      // anzuwenden.
      map.fitBounds(bounds, { padding: [8, 8], animate: false });
    }
  }
  boundaryLayer?.bringToFront();
}

let darkModeQuery: MediaQueryList | null = null;
function handleDarkModeChange(evt: MediaQueryListEvent) {
  isDark.value = evt.matches;
}

// Der Karten-Tab wird per v-show (nicht v-if) ein-/ausgeblendet, damit sein
// Zustand beim Tab-Wechsel erhalten bleibt - das Container-Div hat beim
// Leaflet-Init also ggf. noch Größe 0 (display:none). Ein ResizeObserver
// stößt invalidateSize() an, sobald der Container tatsächlich sichtbare
// Ausmaße bekommt (z.B. beim ersten Wechsel auf den Karte-Tab).
let resizeObserver: ResizeObserver | null = null;

// Übernimmt Ebene/Datensatz/Jahr/Version/Metrik aus der Route in die lokalen
// Refs, wenn ein Query-Parameter vorhanden ist (sonst bleibt der aktuelle
// Wert unangetastet) - für den initialen Deep-Link (onMounted) UND für einen
// erneuten Cross-Link-Sprung auf die bereits (via KeepAlive in App.vue)
// gemountete Karte (onActivated). Die eigentlichen Reload-Reaktionen laufen
// über die bestehenden watch(selected...)-Handler unten, sobald sich ein Ref
// dadurch tatsächlich ändert.
function initFromRoute() {
  const q = route.query;
  const geo = q.geo as GeoType | undefined;
  if (geo === "nuts" || geo === "lau" || geo === "agentur") selectedGeoType.value = geo;

  if (selectedGeoType.value === "agentur") {
    const metric = q.metric as AgenturMetric | undefined;
    if (metric === "arbeitslose" || metric === "unterbeschaeftigung") selectedAgenturMetric.value = metric;
    if (typeof q.dataset === "string") selectedAgenturDataset.value = q.dataset;
    if (q.year) {
      const y = Number(q.year);
      // Bei aggregiertem Datensatz bezeichnet ?year= das Datenjahr, sonst das Berichtsjahr.
      if (!Number.isNaN(y)) {
        if (typeof q.dataset === "string" && q.dataset) selectedAgenturAggYear.value = y;
        else selectedAgenturYear.value = y;
      }
    }
    if (q.month) {
      const m = Number(q.month);
      if (!Number.isNaN(m)) selectedAgenturMonth.value = m;
    }
  } else {
    if (typeof q.dataset === "string") selectedDataset.value = q.dataset;
    if (q.year) {
      const y = Number(q.year);
      if (!Number.isNaN(y)) selectedYear.value = y;
    }
  }
  if (q.nutsVersion) {
    const v = Number(q.nutsVersion);
    if ((NUTS_VERSIONS as readonly number[]).includes(v)) selectedNutsVersion.value = v;
  }
  if (q.lauVersion) {
    const v = Number(q.lauVersion);
    if ((LAU_VERSIONS as readonly number[]).includes(v)) selectedLauVersion.value = v;
  }
}

// Zentriert die Karte auf eine per ?region= angesprungene Region (Cross-Link
// "Regionen -> Karte") und öffnet deren Tooltip - die Hervorhebung selbst
// übernimmt styleForFeature()/highlightedRegionId weiter oben.
function focusOnRegion(id: string) {
  if (!map || !geoJsonLayer) return;
  // Container ggf. noch unsichtbar/größe 0x0 (z.B. beim allerersten
  // KeepAlive-activated direkt nach dem Mount, bevor Vue das Layout gemalt
  // hat) - ein fitBounds() darauf würde Leaflets interne Projektion dauerhaft
  // korrumpieren (siehe buildLayer()). Der ResizeObserver holt den Fokus
  // nach, sobald der Container eine echte Größe hat.
  const size = map.getSize();
  if (size.x === 0 || size.y === 0) return;
  geoJsonLayer.eachLayer((layer) => {
    const l = layer as L.Polygon & { feature?: GeoJSON.Feature };
    if (l.feature?.properties?.geo_id !== id) return;
    const bounds = l.getBounds?.();
    if (bounds && bounds.isValid()) {
      // lastBounds auch hier aktualisieren, sonst würde der ResizeObserver
      // oben (der bei einer späteren Größenänderung auf lastBounds re-fitted)
      // den Fokus auf diese Region wieder auf die volle Kartenausdehnung
      // zurücksetzen.
      lastBounds = bounds;
      // animate: false - bei einem großen Zoomsprung (z.B. von der
      // Deutschlandübersicht auf einen einzelnen Kreis) bleibt Leaflets
      // animierter Zoom (_tryAnimatedZoom) hier gelegentlich hängen (Zoom/
      // Center werden nie final gesetzt); ein sofortiger, unanimierter Sprung
      // ist für diese programmatische Fokussierung ohnehin passender als eine
      // Animation.
      map!.fitBounds(bounds, { padding: [40, 40], maxZoom: 10, animate: false });
    }
  });
}

watch(highlightedRegionId, (id) => {
  if (id) focusOnRegion(id);
});

// Hover/Pin/Cross-Link-Fokus ändern, welche Region als "aktiv" gilt (siehe
// sidebarRegionId) - Rand-Hervorhebung und 3D-Hebe-Effekt neu anwenden, ohne
// (anders als focusOnRegion() oben) den Kartenausschnitt zu verändern - ein
// bloßes Hovern soll nicht wegzoomen.
watch(sidebarRegionId, () => {
  restyleLayer();
  applyLiftEffect();
});

// Spiegelt den aktuellen Auswahlzustand in die Route zurück (router.replace,
// damit reine Auswahländerungen keine Chronik-Einträge erzeugen) - macht die
// Karten-Ansicht deep-linkbar/teilbar, siehe F12.1.
const currentQuery = computed<Record<string, string>>(() => {
  const q: Record<string, string> = { geo: selectedGeoType.value };
  if (selectedGeoType.value === "agentur") {
    if (selectedAgenturDataset.value) {
      q.dataset = selectedAgenturDataset.value;
      if (selectedAgenturAggYear.value !== null) q.year = String(selectedAgenturAggYear.value);
    } else {
      q.metric = selectedAgenturMetric.value;
      if (selectedAgenturYear.value !== null) q.year = String(selectedAgenturYear.value);
      if (selectedAgenturMonth.value !== null) q.month = String(selectedAgenturMonth.value);
    }
  } else {
    if (selectedDataset.value) q.dataset = selectedDataset.value;
    if (selectedYear.value !== null) q.year = String(selectedYear.value);
    if (selectedGeoType.value === "nuts") q.nutsVersion = String(selectedNutsVersion.value);
    if (selectedGeoType.value === "lau") q.lauVersion = String(selectedLauVersion.value);
  }
  if (highlightedRegionId.value) q.region = highlightedRegionId.value;
  return q;
});

watch(currentQuery, (q) => {
  // App.vue hält MapView über <KeepAlive> auch außerhalb der "karte"-Route
  // aktiv, dieser Watcher läuft also z.B. auch weiter, während der Nutzer
  // auf "regionen"/"katalog" ist. route (useRoute()) ist das global aktuelle
  // Route-Objekt, nicht "die Route, mit der diese Komponente zuletzt aktiv
  // war" - ohne diese Prüfung würde ein Query-Key, der zufällig mit einer
  // anderen Ansicht kollidiert, hierher zurücknavigieren (siehe derselbe Bug
  // in RegionBrowser.vue: applyRouteSelection()).
  if (route.name !== "karte") return;
  router.replace({ name: "karte", query: q });
});

onMounted(async () => {
  initFromRoute();

  if (mapContainer.value) {
    map = L.map(mapContainer.value, { center: [51.2, 10.4], zoom: 6, scrollWheelZoom: false });

    resizeObserver = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      if (width > 0 && height > 0) {
        map?.invalidateSize();
        if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
        else if (lastBounds) map?.fitBounds(lastBounds, { padding: [8, 8], animate: false });
      }
    });
    resizeObserver.observe(mapContainer.value);
  }

  darkModeQuery = window.matchMedia?.("(prefers-color-scheme: dark)") ?? null;
  darkModeQuery?.addEventListener("change", handleDarkModeChange);

  await Promise.all([loadGeometries(), loadBoundaries()]);
  buildLayer();
  buildBoundaryLayer();
  await loadInventory();
  try {
    await loadAgenturAggInventory();
  } catch (err) {
    agenturAggError.value = err instanceof Error ? err.message : String(err);
  }
  // Direkt per Deep-Link auf einen aggregierten Agentur-Datensatz gelandet:
  // Werte erst jetzt (nach Jahr-Validierung) laden.
  if (selectedGeoType.value === "agentur" && selectedAgenturDataset.value) await loadValues();
  if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
});

onUnmounted(() => {
  darkModeQuery?.removeEventListener("change", handleDarkModeChange);
  resizeObserver?.disconnect();
  map?.remove();
});

// App.vue hält die Karten-Ansicht per <KeepAlive> gemountet - ein erneuter
// Sprung hierher (z.B. per Cross-Link aus Regionen) remountet die Komponente
// also nicht, sondern reaktiviert sie nur. initFromRoute() übernimmt dabei
// neue Query-Parameter aus dem Cross-Link; danach die Karte per
// invalidateSize() neu vermessen (Container hatte während der Deaktivierung
// ggf. Größe 0) und auf eine mitgegebene Region fokussieren.
onActivated(() => {
  initFromRoute();
  // Bei der allerersten Aktivierung (direkt beim initialen Mount, noch vor
  // dem ersten Layout) hat der Container ggf. noch Größe 0x0 - dann hier
  // nichts tun und den ResizeObserver (s.o.) übernehmen lassen, statt mit
  // invalidateSize()/fitBounds() auf einer 0x0-Größe Leaflets interne
  // Projektion dauerhaft zu korrumpieren (siehe buildLayer()-Kommentar).
  const size = map?.getSize();
  if (!map || !size || size.x === 0 || size.y === 0) return;
  map.invalidateSize();
  if (highlightedRegionId.value) focusOnRegion(highlightedRegionId.value);
  else if (lastBounds) map.fitBounds(lastBounds, { padding: [8, 8], animate: false });
});

watch([selectedDataset, selectedYear], loadValues);
watch(selectedAgenturMetric, () => {
  if (selectedGeoType.value === "agentur" && !selectedAgenturDataset.value) applyAgenturValues();
});
// Datensatzwechsel (BA-Monatswerte <-> aggregierter Kreis-Datensatz): gültiges
// Jahr sicherstellen und Werte neu laden; Geometrie bleibt unverändert.
watch(selectedAgenturDataset, () => {
  ensureAgenturAggSelection();
  if (selectedGeoType.value === "agentur") loadValues();
});
watch(selectedAgenturAggYear, () => {
  if (selectedGeoType.value === "agentur" && selectedAgenturDataset.value) loadValues();
});
// Wenn der gewählte Monat im neuen Jahr nicht existiert (unterschiedliche
// Jahre können unterschiedliche Berichtsmonate abgedeckt haben), auf den
// neuesten verfügbaren Monat des neuen Jahres springen.
watch(selectedAgenturYear, () => {
  const months = agenturMonthOptions.value;
  if (selectedAgenturMonth.value !== null && !months.includes(selectedAgenturMonth.value)) {
    selectedAgenturMonth.value = months[months.length - 1] ?? null;
  }
});
// Beim Wechsel des Berichtsmonats: neue Geometrie+Werte für diesen
// Zeitpunkt laden (analog zum Versionswechsel bei NUTS/LAU).
watch([selectedAgenturYear, selectedAgenturMonth], async () => {
  if (suppressAgenturPeriodWatch) {
    suppressAgenturPeriodWatch = false;
    return;
  }
  if (selectedGeoType.value !== "agentur") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
watch(isDark, () => {
  restyleLayer();
  restyleBoundaryLayer();
  applyLiftEffect();
});
// Beim Wechsel der NUTS-Version müssen sowohl der Grenzverlauf (andere
// Vintage-Periode, ggf. andere Kreis-IDs) als auch die Werte (andere
// target_version im harmonisierten Mart) neu geladen werden. Die Staats-/
// Bundeslandgrenzen hängen an derselben Version und werden unabhängig von
// der gewählten Ebene (NUTS/LAU) mit aktualisiert.
watch(selectedNutsVersion, async () => {
  await loadBoundaries();
  buildBoundaryLayer();
  if (selectedGeoType.value !== "nuts") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// Analog für die LAU-Version (andere target_version im LAU-harmonisierten Mart).
watch(selectedLauVersion, async () => {
  if (selectedGeoType.value !== "lau") return;
  await loadGeometries();
  buildLayer();
  await loadValues();
});
// Beim Wechsel der Ebene: falls der bisher gewählte Datensatz in der neuen
// Ebene nicht existiert (NUTS-3- und LAU-Datensätze sind getrennte Mengen),
// auf den ersten verfügbaren Datensatz der neuen Ebene umschalten (oder
// "" ohne Datensatz-Auswahl in dieser Ebene).
watch(selectedGeoType, async () => {
  if (selectedGeoType.value !== "agentur" && (!selectedDataset.value || !datasetYears.value.has(selectedDataset.value))) {
    const firstDataset = datasetOptions.value[0];
    selectedDataset.value = firstDataset ?? "";
    selectedYear.value = firstDataset ? datasetYears.value.get(firstDataset)?.[0] ?? null : null;
  }
  await loadGeometries();
  buildLayer();
  await loadValues();
});
</script>

<template>
  <div class="map-view">
    <p v-if="geoError" class="error-text">Geometrien konnten nicht geladen werden: {{ geoError }}</p>
    <p v-if="boundaryError" class="error-text">Staats-/Bundeslandgrenzen konnten nicht geladen werden: {{ boundaryError }}</p>
    <p v-if="inventoryError" class="error-text">Datensatz-Übersicht konnte nicht geladen werden: {{ inventoryError }}</p>
    <p v-if="agenturAggError" class="error-text">Aggregierte Agenturbezirk-Datensätze konnten nicht geladen werden: {{ agenturAggError }}</p>
    <p v-if="agenturPeriodsError" class="error-text">Berichtsmonate der Agenturbezirke konnten nicht geladen werden: {{ agenturPeriodsError }}</p>

    <div class="dataset-picker">
      <label for="map-geo-type-select">Ebene</label>
      <select id="map-geo-type-select" v-model="selectedGeoType">
        <option value="nuts">NUTS-3 (Kreise)</option>
        <option value="lau">LAU (Gemeinden)</option>
        <option value="agentur">Agenturbezirke (BA)</option>
      </select>

      <template v-if="selectedGeoType === 'agentur'">
        <label for="map-agentur-dataset-select">Datensatz</label>
        <select id="map-agentur-dataset-select" v-model="selectedAgenturDataset">
          <option value="">BA-Monatswerte (Arbeitslose / Unterbeschäftigung)</option>
          <option v-for="slug in agenturAggDatasetOptions" :key="slug" :value="slug">
            {{ formatDatasetLabel(slug) }} (Kreise aggregiert)
          </option>
        </select>

        <template v-if="selectedAgenturDataset">
          <label for="map-agentur-agg-year-select">Jahr</label>
          <select id="map-agentur-agg-year-select" v-model.number="selectedAgenturAggYear">
            <option v-for="year in agenturAggYearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: selectedAgenturDataset } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
        <template v-else>
          <label for="map-agentur-metric-select">Metrik</label>
          <select id="map-agentur-metric-select" v-model="selectedAgenturMetric">
            <option value="arbeitslose">Arbeitslose</option>
            <option value="unterbeschaeftigung">Unterbeschäftigung</option>
          </select>

          <label for="map-agentur-year-select">Jahr</label>
          <select id="map-agentur-year-select" v-model.number="selectedAgenturYear">
            <option v-for="year in agenturYearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <label for="map-agentur-month-select">Monat</label>
          <select id="map-agentur-month-select" v-model.number="selectedAgenturMonth">
            <option v-for="month in agenturMonthOptions" :key="month" :value="month">{{ String(month).padStart(2, "0") }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: AGENTUR_DATASET_SLUG } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
      </template>
      <template v-else>
        <label for="map-dataset-select">Datensatz</label>
        <select id="map-dataset-select" v-model="selectedDataset">
          <option value="">Kein Datensatz (nur Karte)</option>
          <option v-for="slug in datasetOptions" :key="slug" :value="slug">{{ formatDatasetLabel(slug) }}</option>
        </select>

        <template v-if="selectedDataset">
          <label for="map-year-select">Jahr</label>
          <select id="map-year-select" v-model.number="selectedYear">
            <option v-for="year in yearOptions" :key="year" :value="year">{{ year }}</option>
          </select>

          <RouterLink :to="{ name: 'katalog', params: { slug: selectedDataset } }" class="link-button">
            Datensatz im Katalog ansehen ↗
          </RouterLink>
        </template>
      </template>

      <template v-if="selectedGeoType === 'nuts'">
        <label for="map-nuts-version-select">NUTS-Version</label>
        <select id="map-nuts-version-select" v-model.number="selectedNutsVersion">
          <option v-for="version in NUTS_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>
      <template v-else-if="selectedGeoType === 'lau'">
        <label for="map-lau-version-select">LAU-Version</label>
        <select id="map-lau-version-select" v-model.number="selectedLauVersion">
          <option v-for="version in LAU_VERSIONS" :key="version" :value="version">{{ version }}</option>
        </select>
      </template>

      <span v-if="valuesLoading" class="hint">Lade Werte…</span>
      <span v-if="valuesError" class="error-text">Fehler: {{ valuesError }}</span>
    </div>

    <div class="map-layout">
      <div class="map-wrap">
        <div ref="mapContainer" class="map-container"></div>
        <div class="map-legend">
          <template v-if="selectedGeoType === 'agentur'">
            <p v-if="selectedAgenturDataset" class="map-legend-title">
              {{ formatDatasetLabel(selectedAgenturDataset) }}
              <template v-if="selectedAgenturAggYear"> · {{ selectedAgenturAggYear }}</template>
            </p>
            <p v-else class="map-legend-title">
              {{ AGENTUR_METRIC_LABELS[selectedAgenturMetric] }}
              <template v-if="selectedAgenturYear && selectedAgenturMonth">
                · {{ formatMonthLabel(selectedAgenturYear, selectedAgenturMonth) }}
              </template>
            </p>
            <div class="map-legend-scale">
              <span v-for="step in sequentialSteps(isDark)" :key="step" class="map-legend-swatch" :style="{ background: step }"></span>
            </div>
            <div class="map-legend-labels">
              <span>{{ valueDomain ? formatNumber(valueDomain[0]) : "–" }}</span>
              <span>{{ valueDomain ? formatNumber(valueDomain[1]) : "–" }}</span>
            </div>
            <p class="map-legend-note">
              <span class="map-legend-swatch map-legend-swatch--nodata"></span>keine Daten
            </p>
            <AgenturAggregationNote v-if="selectedAgenturDataset" :points="Array.from(valuesByRegion.values())" />
          </template>
          <template v-else-if="selectedDataset">
            <p class="map-legend-title">{{ formatDatasetLabel(selectedDataset) }}<template v-if="selectedYear"> · {{ selectedYear }}</template></p>
            <div class="map-legend-scale">
              <span v-for="step in sequentialSteps(isDark)" :key="step" class="map-legend-swatch" :style="{ background: step }"></span>
            </div>
            <div class="map-legend-labels">
              <span>{{ valueDomain ? formatNumber(valueDomain[0]) : "–" }}</span>
              <span>{{ valueDomain ? formatNumber(valueDomain[1]) : "–" }}</span>
            </div>
            <p class="map-legend-note">
              <span class="map-legend-swatch map-legend-swatch--nodata"></span>keine Daten
              <span class="map-legend-swatch map-legend-swatch--interpolated"></span>interpoliert (gestrichelter Rand)
            </p>
          </template>
          <p v-else class="map-legend-title">
            <span
              class="map-legend-swatch"
              :style="{
                background: isDark ? MAP_ONLY_FILL_DARK : MAP_ONLY_FILL_LIGHT,
                display: 'inline-block',
                width: '10px',
                height: '10px',
                borderRadius: '2px',
                marginRight: '0.3rem',
                verticalAlign: 'middle',
              }"
            ></span>
            {{ selectedGeoType === "lau" ? "LAU-Gemeinden" : "Kein Datensatz ausgewählt" }}
          </p>
        </div>
      </div>

      <aside class="map-sidebar">
        <template v-if="sidebarRegion">
          <div class="map-sidebar-header">
            <h3>{{ sidebarRegion.name }}</h3>
            <span class="region-code">{{ sidebarRegion.id }}</span>
          </div>
          <dl class="map-sidebar-data">
            <template v-if="!sidebarRegion.point">
              <dt>{{ sidebarValueLabel }}</dt>
              <dd>keine Daten</dd>
            </template>
            <template v-else>
              <dt>Jahr</dt>
              <dd>{{ sidebarRegion.point.year }}</dd>
              <dt>{{ sidebarValueLabel }}</dt>
              <dd>{{ formatNumber(sidebarRegion.point.value) }}</dd>
              <template v-if="selectedGeoType !== 'agentur'">
                <dt>Status</dt>
                <dd>{{ sidebarRegion.point.is_interpolated ? "interpoliert" : "beobachtet" }}</dd>
              </template>
              <template v-else-if="sidebarRegion.point.source_region_count !== undefined">
                <dt>Status</dt>
                <dd>
                  aggregiert aus {{ sidebarRegion.point.source_region_count }} Kreis(en)
                  <template v-if="sidebarRegion.point.aggregation_method === 'sum'">(Summe)</template>
                  <template v-else-if="sidebarRegion.point.value_kind === 'median'">(Näherung, nur Schätzung)</template>
                  <template v-else>(Näherung: gewichteter Durchschnitt)</template>
                </dd>
              </template>
            </template>
          </dl>
          <RouterLink v-if="sidebarLinkTarget" :to="sidebarLinkTarget" class="link-button">
            Datensatz für diese Region ansehen ↗
          </RouterLink>
          <button
            v-if="pinnedRegionId && !hoveredRegionId"
            type="button"
            class="link-button map-sidebar-clear"
            @click="pinnedRegionId = null"
          >
            Auswahl aufheben
          </button>
        </template>
        <p v-else class="hint">Über eine Region auf der Karte fahren oder klicken, um Details zu sehen.</p>
      </aside>
    </div>
  </div>
</template>
