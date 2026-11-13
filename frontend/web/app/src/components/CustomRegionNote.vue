<script setup lang="ts">
import { computed } from "vue";
import { formatPercent } from "../format";
import { employeeBasis } from "../employeeWeight";

// Describes how a dataset was aggregated over a custom region
// (fields from POST /custom-regions/dataset-aggregate): level used,
// method, approximations and coverage.
type NotePoint = {
  aggregation_method?: string;
  value_kind?: string;
  level_used?: string;
  coverage?: number;
  source_region_count?: number;
  matched_unit?: string;
  is_crosswalked?: boolean;
  employee_slug?: string | null;
};
const props = defineProps<{ points: NotePoint[] }>();

const LEVEL_LABEL: Record<string, string> = { lau: "Gemeinden (LAU)", nuts3: "Kreise (NUTS-3)", agentur: "Agenturbezirke" };

const notes = computed(() => {
  const pts = props.points;
  if (!pts.length) return [];
  const out: string[] = [];
  const matched = pts.find((p) => p.matched_unit)?.matched_unit;
  const levels = [...new Set(pts.map((p) => p.level_used).filter(Boolean))] as string[];
  const levelText = levels.map((l) => LEVEL_LABEL[l] ?? l).join(", ");
  if (matched) {
    // Exact match: no weighting, the unit's value is taken as-is.
    out.push(
      `Die Region entspricht exakt der Einheit ${matched} (${levelText}) – dargestellt sind deren Werte, ohne Anteile von Nachbareinheiten.` +
        (pts.some((p) => p.is_crosswalked) ? " Teilweise über Gebietsstände umgerechnet." : ""),
    );
    return out;
  }
  const maxUnits = Math.max(...pts.map((p) => p.source_region_count ?? 0));
  out.push(`Ebene: ${levelText}, bis zu ${maxUnits} ${maxUnits === 1 ? "Einheit überlappt" : "Einheiten überlappen"} die Region.`);
  const methods = new Set(pts.map((p) => p.aggregation_method));
  if (methods.has("sum")) out.push("Summe der Einheitenwerte, flächenanteilig.");
  if (methods.has("weighted_avg_population")) {
    out.push("Quoten/Durchschnitte sind bevölkerungsgewichtete Mittel der Einheitenwerte – eine Näherung.");
    if (pts.some((p) => p.aggregation_method === "weighted_avg_population" && p.employee_slug)) {
      out.push("Wo die Beschäftigtenzahl fehlt (z. B. SV-Beschäftigte vor 2007), wird stattdessen nach der Bevölkerung gewichtet.");
    }
  }
  if (methods.has("weighted_avg_employees")) {
    const bases = [...new Set(pts.filter((p) => p.aggregation_method === "weighted_avg_employees").map((p) => employeeBasis(p.employee_slug)))];
    out.push(`Mittel der Einheitenwerte, gewichtet nach der Zahl der ${bases.join(" bzw. ")} – eine Näherung.`);
  }
  if (methods.has("weighted_avg_area")) {
    out.push("Teilweise flächengewichtet (für mindestens eine Einheit fehlt die Gewichtung) – Näherung.");
  }
  if (pts.some((p) => p.value_kind === "median")) {
    out.push("Mediane lassen sich nicht aus Einheitenwerten zusammensetzen – der Wert ist nur eine Schätzung.");
  }
  const coverage = Math.min(...pts.map((p) => p.coverage ?? 1));
  if (coverage < 0.95) {
    out.push(`Für einen Teil der Fläche liegen keine Daten vor (Abdeckung ab ${formatPercent(coverage)}) – der Wert gilt nur für den abgedeckten Teil.`);
  }
  return out;
});
</script>

<template>
  <p v-if="notes.length" class="hint aggregation-note"><strong>Aggregation:</strong> {{ notes.join(" ") }}</p>
</template>
