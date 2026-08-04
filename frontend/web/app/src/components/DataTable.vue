<script setup lang="ts">
import { computed } from "vue";
import type { GenericDatasetRow, TimeseriesPoint } from "../types/api";
import { formatCell, formatNumber } from "../format";

// Entweder `points` (Zeitreihen-Tabelle mit fixen Spalten inkl. Interpolations-
// Status) oder `rows` (generische Fallback-Tabelle für Datensätze ohne
// /timeseries-Unterstützung) übergeben.
const props = defineProps<{
  points?: TimeseriesPoint[];
  rows?: GenericDatasetRow[];
}>();

const sortedPoints = computed(() =>
  (props.points ?? []).slice().sort((a, b) => a.year - b.year),
);

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
        <tr v-for="p in sortedPoints" :key="p.year" :class="{ 'is-interpolated': p.is_interpolated }">
          <td>{{ p.year }}</td>
          <td>{{ formatNumber(p.value) }}</td>
          <td>{{ p.is_interpolated ? "interpoliert" : "beobachtet" }}</td>
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
