<script setup lang="ts">
import { computed } from "vue";

// Kennzeichnet, wie ein auf Agenturbezirke aggregierter Kreis-Datensatz
// entstanden ist und welche Grenzen das hat (aggregation_method/value_kind aus
// GET /agenturbezirke/.../dataset-aggregate..., siehe mart_agentur_dataset_aggregate).
const props = defineProps<{
  points: { aggregation_method?: string; value_kind?: string }[];
}>();

const notes = computed(() => {
  const methods = new Set(props.points.map((p) => p.aggregation_method).filter(Boolean));
  const kinds = new Set(props.points.map((p) => p.value_kind).filter(Boolean));
  const out: string[] = [];
  if (methods.has("sum")) {
    out.push("Summe der Kreis-Werte (Berlin: ein Kreis, flächenanteilig auf drei Agenturbezirke verteilt).");
  }
  if (methods.has("weighted_avg_population")) {
    out.push(
      "Bevölkerungsgewichteter Durchschnitt der Kreis-Werte – eine Näherung, keine exakte Neuberechnung " +
        "(Zähler/Nenner liegen nicht vor, die Gesamtbevölkerung ersetzt die eigentliche Bezugsgröße).",
    );
  }
  if (methods.has("weighted_avg_area")) {
    out.push(
      "Teilweise flächengewichteter Durchschnitt (für mindestens einen Kreis fehlt die Bevölkerung) – Näherung.",
    );
  }
  if (kinds.has("median")) {
    out.push("Mediane lassen sich nicht aus Kreis-Medianen zusammensetzen – der Wert ist nur eine Schätzung.");
  }
  return out;
});
</script>

<template>
  <p v-if="notes.length" class="hint aggregation-note">
    <strong>Aggregation:</strong> {{ notes.join(" ") }}
  </p>
</template>
