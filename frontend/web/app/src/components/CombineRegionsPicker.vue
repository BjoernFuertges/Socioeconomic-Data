<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { apiGet } from "../api/client";
import type { RegionGeometry } from "../customRegion";
import type { GeoRegionSummary } from "../types/api";

// Combine a custom region from existing regions: click units (Länder, Kreise,
// Agenturbezirke, Gemeinden) on the map; the selection persists across level
// switches (levels may be mixed). Only a MultiPolygon is sent out - the backend
// unions overlaps (ST_UnaryUnion).

type LevelKey = "nuts1" | "nuts3" | "agentur" | "lau";
type Entry = { key: string; id: string; name: string; level: LevelKey; geometry: GeoJSON.Geometry };

const LEVELS: { key: LevelKey; label: string }[] = [
  { key: "nuts1", label: "Bundesländer (NUTS-1)" },
  { key: "nuts3", label: "Kreise (NUTS-3)" },
  { key: "agentur", label: "Agenturbezirke" },
  { key: "lau", label: "Gemeinden (LAU)" },
];

const emit = defineEmits<{ (e: "update:geometry", value: RegionGeometry | null): void }>();

const level = ref<LevelKey>("nuts3");
const lauParent = ref("");
const kreise = ref<{ id: string; name: string }[]>([]);
const loading = ref(false);
const loadError = ref<string | null>(null);
const search = ref("");

const selection = ref<Map<string, Entry>>(new Map());
const current = ref<Entry[]>([]);
const cache = new Map<string, Entry[]>();

const mapContainer = ref<HTMLElement | null>(null);
let map: L.Map | null = null;
let levelLayer: L.GeoJSON | null = null;
let otherLayer: L.GeoJSON | null = null;

const selectedList = computed(() => [...selection.value.values()]);
const levelLabel = (l: LevelKey) => LEVELS.find((x) => x.key === l)?.label ?? l;

const filteredCurrent = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return current.value;
  return current.value.filter((e) => e.name.toLowerCase().includes(q) || e.id.toLowerCase().includes(q));
});

// Map background is var(--page-plane) (no tiles), so outlines need lighter
// colors in dark mode.
const dark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;

function styleFor(selected: boolean): L.PathOptions {
  return selected
    ? { color: "#2a6fdb", weight: 2, fillColor: "#2a6fdb", fillOpacity: 0.5 }
    : { color: dark ? "#c4c7c5" : "#5f6368", weight: 1, fillColor: dark ? "#c4c7c5" : "#9aa0a6", fillOpacity: dark ? 0.15 : 0.1 };
}

function fitToData() {
  if (!map) return;
  // In a hidden tab the container has size 0 - fitBounds would then set an
  // invalid zoom (map stays empty/black). invalidate() catches up later.
  const size = map.getSize();
  if (!size.x || !size.y) return;
  const layer = selection.value.size && otherLayer ? otherLayer : levelLayer;
  const bounds = layer?.getBounds();
  if (bounds?.isValid()) map.fitBounds(bounds, { padding: [16, 16], animate: false });
}

function toggle(entry: Entry) {
  const next = new Map(selection.value);
  if (next.has(entry.key)) next.delete(entry.key);
  else next.set(entry.key, entry);
  selection.value = next;
}

function clearSelection() {
  selection.value = new Map();
}

function toEntry(l: LevelKey, r: GeoRegionSummary): Entry | null {
  if (!r.geometry_wgs84) return null;
  return { key: `${l}:${r.geo_id}`, id: r.geo_id, name: r.region_name, level: l, geometry: r.geometry_wgs84 };
}

async function loadLevel() {
  loadError.value = null;
  current.value = [];
  if (level.value === "lau" && !lauParent.value) {
    drawLevel();
    return;
  }
  const cacheKey = `${level.value}:${lauParent.value}`;
  const hit = cache.get(cacheKey);
  if (hit) {
    current.value = hit;
    drawLevel();
    return;
  }
  loading.value = true;
  try {
    const params: Record<string, string | number | boolean> = { include_geometry: true, limit: "null" };
    if (level.value === "nuts1") Object.assign(params, { geo_type: "nuts", level: 1, nuts_parent_id: "DE" });
    else if (level.value === "nuts3") Object.assign(params, { geo_type: "nuts", level: 3, nuts_parent_id: "DE" });
    else if (level.value === "agentur") Object.assign(params, { geo_type: "agentur" });
    else Object.assign(params, { geo_type: "lau", country_code: "DE", nuts_parent_id: lauParent.value });
    const res = await apiGet<GeoRegionSummary[]>("/geo", params);
    const entries = (res.data ?? []).map((r) => toEntry(level.value, r)).filter((e): e is Entry => e !== null);
    cache.set(cacheKey, entries);
    current.value = entries;
  } catch (err) {
    loadError.value = `Regionen konnten nicht geladen werden: ${err instanceof Error ? err.message : String(err)}`;
  } finally {
    loading.value = false;
  }
  drawLevel();
}

function drawLevel() {
  if (!map) return;
  levelLayer?.remove();
  levelLayer = null;
  if (!current.value.length) {
    drawOthers();
    return;
  }
  levelLayer = L.geoJSON(
    {
      type: "FeatureCollection",
      features: current.value.map((e) => ({ type: "Feature", properties: { key: e.key }, geometry: e.geometry })),
    } as GeoJSON.FeatureCollection,
    {
      style: (f) => styleFor(selection.value.has(f?.properties?.key)),
      onEachFeature: (f, layer) => {
        const entry = current.value.find((e) => e.key === f.properties.key);
        if (!entry) return;
        layer.bindTooltip(entry.name, { sticky: true });
        layer.on("click", () => toggle(entry));
      },
    },
  ).addTo(map);
  drawOthers();
  if (!selection.value.size) fitToData();
}

// Selection of other levels as a non-clickable overlay so it stays visible.
function drawOthers() {
  if (!map) return;
  otherLayer?.remove();
  otherLayer = null;
  const shown = new Set(current.value.map((e) => e.key));
  const others = selectedList.value.filter((e) => !shown.has(e.key));
  if (!others.length) return;
  otherLayer = L.geoJSON(
    {
      type: "FeatureCollection",
      features: others.map((e) => ({ type: "Feature", properties: {}, geometry: e.geometry })),
    } as GeoJSON.FeatureCollection,
    { interactive: false, style: () => styleFor(true) },
  ).addTo(map);
}

function toMultiPolygon(entries: Entry[]): RegionGeometry | null {
  const polygons: unknown[] = [];
  for (const e of entries) {
    const g = e.geometry;
    if (g.type === "Polygon") polygons.push(g.coordinates);
    else if (g.type === "MultiPolygon") polygons.push(...g.coordinates);
  }
  return polygons.length ? { type: "MultiPolygon", coordinates: polygons } : null;
}

watch(selection, () => {
  levelLayer?.setStyle((f) => styleFor(selection.value.has(f?.properties?.key)));
  drawOthers();
  emit("update:geometry", toMultiPolygon(selectedList.value));
});

watch([level, lauParent], loadLevel);

onMounted(async () => {
  if (mapContainer.value) {
    map = L.map(mapContainer.value, { center: [51.2, 10.4], zoom: 6, scrollWheelZoom: false });
  }
  try {
    const res = await apiGet<GeoRegionSummary[]>("/geo", { geo_type: "nuts", level: 3, nuts_parent_id: "DE", limit: "null" });
    kreise.value = (res.data ?? [])
      .map((r) => ({ id: r.geo_id, name: r.region_name }))
      .sort((a, b) => a.name.localeCompare(b.name, "de"));
  } catch {
    // The Kreis list is only needed for the Gemeinde level
  }
  loadLevel();
});

onBeforeUnmount(() => {
  map?.remove();
  map = null;
});

// When the tab is shown (v-show), Leaflet must recompute its size.
defineExpose({
  invalidate() {
    // The map is initialized in the hidden tab (size 0) - after it is shown,
    // recompute the size and re-apply the view.
    map?.invalidateSize();
    fitToData();
  },
});
</script>

<template>
  <div class="combine">
    <p class="hint">
      Einheiten auf der Karte oder in der Liste anklicken, um sie zur Region hinzuzufügen oder zu entfernen. Die
      Auswahl bleibt beim Wechsel der Ebene erhalten; überlappende Einheiten werden nur einmal gezählt.
    </p>

    <div class="combine-controls">
      <label>
        Ebene
        <select v-model="level">
          <option v-for="l in LEVELS" :key="l.key" :value="l.key">{{ l.label }}</option>
        </select>
      </label>
      <label v-if="level === 'lau'">
        Kreis
        <select v-model="lauParent">
          <option value="">– Kreis wählen –</option>
          <option v-for="k in kreise" :key="k.id" :value="k.id">{{ k.name }} ({{ k.id }})</option>
        </select>
      </label>
      <label>
        Suche
        <input v-model="search" type="search" placeholder="Name oder ID" />
      </label>
    </div>

    <p v-if="loading" class="hint">Lade Regionen …</p>
    <p v-else-if="level === 'lau' && !lauParent" class="hint">Für Gemeinden zuerst einen Kreis wählen.</p>
    <p v-if="loadError" class="error-text" role="alert">{{ loadError }}</p>

    <div ref="mapContainer" class="map-container combine-map" aria-label="Regionen auswählen"></div>

    <div class="combine-lists">
      <ul class="combine-candidates" aria-label="Verfügbare Regionen">
        <li v-for="e in filteredCurrent" :key="e.key">
          <label>
            <input type="checkbox" :checked="selection.has(e.key)" @change="toggle(e)" />
            {{ e.name }} <span class="hint">{{ e.id }}</span>
          </label>
        </li>
      </ul>

      <div class="combine-selected">
        <h4>Auswahl ({{ selectedList.length }})</h4>
        <p v-if="!selectedList.length" class="hint">Noch nichts ausgewählt.</p>
        <ul v-else>
          <li v-for="e in selectedList" :key="e.key">
            {{ e.name }} <span class="hint">{{ levelLabel(e.level) }}</span>
            <button type="button" class="link-button" :aria-label="`${e.name} entfernen`" @click="toggle(e)">✕</button>
          </li>
        </ul>
        <button v-if="selectedList.length" type="button" class="link-button" @click="clearSelection">Auswahl leeren</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.combine-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}

.combine-controls label {
  display: grid;
  gap: 0.15rem;
  font-size: 0.85rem;
}

.combine-map {
  height: 380px;
  min-height: 0;
  border: 1px solid var(--border);
  border-radius: 6px;
}

.combine-lists {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  gap: 1rem;
  margin-top: 0.75rem;
}

.combine-candidates,
.combine-selected ul {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 14rem;
  overflow-y: auto;
  display: grid;
  gap: 0.2rem;
  font-size: 0.85rem;
}
</style>
