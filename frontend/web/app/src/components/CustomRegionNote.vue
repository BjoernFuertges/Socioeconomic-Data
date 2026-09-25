<script setup lang="ts">
import { computed } from "vue";
import { formatPercent } from "../format";

// Kennzeichnet, wie ein Datensatz über eine eigene Region aggregiert wurde
// (Felder aus POST /custom-regions/dataset-aggregate): genutzte Ebene,
// Methode, Näherungen und Abdeckung.
type NotePoint = {
  aggregation_method?: string;
  value_kind?: string;
  level_used?: string;
  coverage?: number;
  source_region_count?: number;
  matched_unit?: string;
  is_crosswalked?: boolean;
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
    // Exakter Treffer: keine Gewichtung, der Einheitenwert wird 1:1 übernommen.
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
  }
  if (methods.has("weighted_avg_area")) {
    out.push("Teilweise flächengewichtet (für mindestens eine Einheit fehlt die Bevölkerung) – Näherung.");
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
