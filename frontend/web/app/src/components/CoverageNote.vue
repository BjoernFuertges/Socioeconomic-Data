<script setup lang="ts">
import { computed } from "vue";

// Hint for aggregated series (NUTS-0/1/2, Agenturbezirke): is_complete=false means
// not every expected Kreis contributed a value in that year, so the aggregate is only
// a partial result (sums too small, see mart_nuts012_dataset_aggregate /
// mart_agentur_dataset_aggregate). The value itself is not corrected. Points without
// is_complete (older API, other region types) are ignored.
const props = defineProps<{
  points: { year: number; is_complete?: boolean; source_region_count?: number; expected_region_count?: number }[];
}>();

const note = computed(() => {
  const incomplete = props.points.filter((p) => p.is_complete === false);
  if (!incomplete.length) return "";
  const latest = incomplete.reduce((a, b) => (b.year > a.year ? b : a));
  const detail =
    latest.source_region_count !== undefined && latest.expected_region_count !== undefined
      ? ` (${latest.year}: ${latest.source_region_count} von ${latest.expected_region_count} Kreisen)`
      : "";
  const years = incomplete.length === 1 ? "einem Jahr" : `${incomplete.length} Jahren`;
  return `Summe unvollständig: In ${years} liegen nicht für alle Kreise Werte vor${detail} – der Wert ist ein Teilergebnis, bei Summen entsprechend zu niedrig.`;
});
</script>

<template>
  <p v-if="note" class="hint coverage-note"><strong>Hinweis:</strong> {{ note }}</p>
</template>
