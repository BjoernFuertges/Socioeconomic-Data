<script setup lang="ts">
import { computed } from "vue";
import { AGENTUR_EMPLOYEE_SLUG, employeeBasis } from "../employeeWeight";

// Describes how a Kreis dataset aggregated to Agenturbezirke came about and what
// its limits are (aggregation_method/value_kind from
// GET /agenturbezirke/.../dataset-aggregate..., see mart_agentur_dataset_aggregate).
const props = defineProps<{
  points: { aggregation_method?: string; value_kind?: string }[];
  datasetSlug?: string; // base slug, selects the weighting wording
}>();

const notes = computed(() => {
  const methods = new Set(props.points.map((p) => p.aggregation_method).filter(Boolean));
  const kinds = new Set(props.points.map((p) => p.value_kind).filter(Boolean));
  const out: string[] = [];
  const employeeSlug = props.datasetSlug ? AGENTUR_EMPLOYEE_SLUG[props.datasetSlug] : undefined;
  if (methods.has("sum")) {
    out.push("Summe der Kreis-Werte (Berlin: ein Kreis, flächenanteilig auf drei Agenturbezirke verteilt).");
  }
  if (methods.has("weighted_avg_population")) {
    out.push(
      "Bevölkerungsgewichteter Durchschnitt der Kreis-Werte – eine Näherung, keine exakte Neuberechnung " +
        "(Zähler/Nenner liegen nicht vor, die Gesamtbevölkerung ersetzt die eigentliche Bezugsgröße).",
    );
    if (employeeSlug) {
      out.push(
        `Wo die Zahl der ${employeeBasis(employeeSlug)} fehlt (SV-Beschäftigte gibt es erst ab 2007), wird stattdessen nach der Bevölkerung gewichtet.`,
      );
    }
  }
  if (methods.has("weighted_avg_employees")) {
    out.push(
      `Nach der Zahl der ${employeeBasis(employeeSlug)} gewichteter Durchschnitt der Kreis-Werte – eine Näherung, ` +
        "keine exakte Neuberechnung.",
    );
  }
  if (methods.has("weighted_avg_area")) {
    out.push(
      "Teilweise flächengewichteter Durchschnitt (für mindestens einen Kreis fehlt die Gewichtung) – Näherung.",
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
