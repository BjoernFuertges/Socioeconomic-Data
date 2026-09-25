<script setup lang="ts">
import { computed } from "vue";
import type { GenericDatasetRow, TimeseriesPoint } from "../types/api";
import { formatCell, formatNumber } from "../format";

// Entweder `points` (Jahreszeitreihe mit Interpolationsstatus, NUTS/LAU),
// `periodRows` (Monats-/Periodenreihe ohne Interpolation, z.B.
// Agenturbezirke) oder `rows` (generische Fallback-Tabelle für Datensätze
// ohne /timeseries-Unterstützung) übergeben.
const props = defineProps<{
  points?: TimeseriesPoint[];
  periodRows?: { label: string; arbeitslose: number | null; unterbeschaeftigung: number | null }[];
  // Welche periodRows-Spalten angezeigt werden - z.B. nur "Unterbeschäftigung",
  // wenn im RegionBrowser-Agenturpanel nur diese Metrik ausgewählt ist. Ohne
  // Angabe (andere periodRows-Konsumenten, falls es sie künftig gibt) werden
  // beide Spalten gezeigt.
  periodColumns?: { key: "arbeitslose" | "unterbeschaeftigung"; label: string }[];
  rows?: GenericDatasetRow[];
}>();

const DEFAULT_PERIOD_COLUMNS: { key: "arbeitslose" | "unterbeschaeftigung"; label: string }[] = [
  { key: "arbeitslose", label: "Arbeitslose" },
  { key: "unterbeschaeftigung", label: "Unterbeschäftigung" },
];
const effectivePeriodColumns = computed(() => props.periodColumns ?? DEFAULT_PERIOD_COLUMNS);

const sortedPoints = computed(() =>
  (props.points ?? []).slice().sort((a, b) => a.year - b.year),
);

// Dritter Status neben beobachtet/interpoliert - siehe is_computed-Kommentar
// auf TimeseriesPoint (types/api.ts).
function statusLabel(point: TimeseriesPoint): string {
  if (point.is_computed) return "berechnet";
  return point.is_interpolated ? "interpoliert" : "beobachtet";
}

const skipKeys = new Set(["mapper"]);
const columns = computed(() => {
  const rows = props.rows ?? [];
  if (!rows.length) return [];
  return Object.keys(rows[0]).filter((key) => !skipKeys.has(key));
});
</script>

<template>
  <div class="data-table-wrap">
    <table v-if="points" class="data-table">
      <thead>
        <tr>
          <th>Jahr</th>
          <th>Wert</th>
          <th>Status</th>
          <th>Linear</th>
          <th>Peer-Trend</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="p in sortedPoints"
          :key="p.year"
          :class="{ 'is-interpolated': p.is_interpolated && !p.is_computed, 'is-computed': p.is_computed }"
        >
          <td>{{ p.year }}</td>
          <td>{{ formatNumber(p.value) }}</td>
          <td>{{ statusLabel(p) }}</td>
          <td>{{ p.value_linear !== undefined && p.value_linear !== null ? formatNumber(p.value_linear) : "–" }}</td>
          <td>
            {{
              p.value_peer_trend !== undefined && p.value_peer_trend !== null
                ? formatNumber(p.value_peer_trend)
                : "–"
            }}
          </td>
        </tr>
      </tbody>
    </table>

    <table v-else-if="periodRows" class="data-table">
      <thead>
        <tr>
          <th>Periode</th>
          <th v-for="col in effectivePeriodColumns" :key="col.key">{{ col.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in periodRows" :key="row.label">
          <td>{{ row.label }}</td>
          <td v-for="col in effectivePeriodColumns" :key="col.key">
            {{ row[col.key] !== null ? formatNumber(row[col.key]) : "–" }}
          </td>
        </tr>
      </tbody>
    </table>

    <table v-else class="data-table">
      <thead>
        <tr>
          <th v-for="c in columns" :key="c">{{ c }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, idx) in rows ?? []" :key="idx">
          <td v-for="c in columns" :key="c">{{ formatCell(row[c], c) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
