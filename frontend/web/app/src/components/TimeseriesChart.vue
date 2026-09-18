<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import type { TimeseriesPoint } from "../types/api";
import { formatNumber } from "../format";

type PeriodSeries = {
  key: string;
  label: string;
  points: { label: string; value: number | null }[];
};

// Für den Vergleich mehrerer Datensätze mit unterschiedlichen Einheiten/
// Skalen (kombinierte Ansicht im DatasetPicker) - jede Serie behält ihre
// eigenen Jahre (keine gemeinsame Periodenraster-Länge wie bei
// `periodSeries` nötig), die x-Achse positioniert über das tatsächliche
// Jahr. Siehe drawIndexedChart(): Werte werden auf Basisjahr=100 indexiert,
// weil ein gemeinsamer Wert-Y-Achse bei z.B. "Durchschnittsalter" (~40) vs.
// "Arbeitslosenquote" (~5%) sonst irreführend wäre (kein Dual-Axis-Chart,
// siehe dataviz-Skill: zwei Skalen -> ein Chart nur indexiert auf Basis).
type IndexedSeries = {
  key: string;
  label: string;
  points: TimeseriesPoint[];
};

// Entweder `points` (Jahreszeitreihe mit Interpolationsstatus, NUTS/LAU),
// `periodSeries` (eine oder mehrere Monats-/Periodenreihen ohne
// Interpolation, z.B. Agenturbezirk Arbeitslose/Unterbeschäftigung) oder
// `indexedSeries` (mehrere Jahreszeitreihen unterschiedlicher Datensätze,
// kombiniert/indexiert) übergeben. Alle Serien in `periodSeries` müssen
// dieselbe Periodenraster-Länge/-Reihenfolge teilen (Werte dürfen `null`
// sein, wo eine Serie an dieser Periode keinen Wert hat) - die x-Achse wird
// über den Index positioniert, nicht über das Label.
const props = defineProps<{
  points?: TimeseriesPoint[];
  periodSeries?: PeriodSeries[];
  indexedSeries?: IndexedSeries[];
}>();

// Reihenfolge entspricht der validierten Slot-Reihenfolge des dataviz-Skills
// (blau/orange/aqua/gelb) - feste kategoriale Zuordnung, nicht zyklisch neu
// gemischt. Deckelt die kombinierte Ansicht auf 4 Serien (siehe DatasetPicker.vue).
const SERIES_COLORS = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)"];

const svgRef = ref<SVGSVGElement | null>(null);
const tooltip = ref<{ visible: boolean; x: number; y: number; rows: [string, string | number][] }>({
  visible: false,
  x: 0,
  y: 0,
  rows: [],
});

const svgNS = "http://www.w3.org/2000/svg";

function showTooltip(evt: MouseEvent, point: TimeseriesPoint) {
  const rows: [string, string | number][] = [
    ["Jahr", point.year],
    ["Wert", formatNumber(point.value)],
    ["Status", point.is_interpolated ? "interpoliert" : "beobachtet"],
  ];
  if (point.is_interpolated) {
    if (point.value_linear !== undefined && point.value_linear !== null) {
      rows.push(["linear", formatNumber(point.value_linear)]);
    }
    if (point.value_peer_trend !== undefined && point.value_peer_trend !== null) {
      rows.push(["Peer-Trend", formatNumber(point.value_peer_trend)]);
    }
    if (point.n_peer_regions !== undefined && point.n_peer_regions !== null) {
      rows.push(["Peer-Regionen", point.n_peer_regions]);
    }
  }
  tooltip.value = { visible: true, x: evt.clientX + 12, y: evt.clientY + 12, rows };
}

function hideTooltip() {
  tooltip.value.visible = false;
}

// Tooltip zeigt die Werte ALLER sichtbaren Serien an der gehoverten
// Periode (nicht nur der Serie, deren Marker gehovert wurde) - so bleiben
// z.B. Arbeitslose/Unterbeschäftigung an derselben Periode vergleichbar.
function showPeriodTooltip(evt: MouseEvent, idx: number, series: PeriodSeries[]) {
  const label = series[0]?.points[idx]?.label ?? "";
  const rows: [string, string | number][] = [["Periode", label]];
  for (const s of series) {
    const value = s.points[idx]?.value;
    rows.push([s.label, value !== null && value !== undefined ? formatNumber(value) : "–"]);
  }
  tooltip.value = { visible: true, x: evt.clientX + 12, y: evt.clientY + 12, rows };
}

// Einfache, indexbasierte Linienchart-Variante für eine oder mehrere
// Perioden-Reihen (z.B. Agenturbezirk-Monatswerte) ohne Interpolationsstatus.
function drawPeriodChart(svg: SVGSVGElement, series: PeriodSeries[]) {
  const visible = series.filter((s) => s.points.length);
  if (!visible.length) return;
  const periodCount = visible[0].points.length;

  const padding = { top: 16, right: 16, bottom: 28, left: 72 };
  const width = 640;
  const height = 260;
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const allValues = visible.flatMap((s) => s.points.map((p) => p.value).filter((v): v is number => v !== null));
  if (!allValues.length) return;
  let minValue = Math.min(...allValues);
  let maxValue = Math.max(...allValues);
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  const valuePad = (maxValue - minValue) * 0.1;
  minValue -= valuePad;
  maxValue += valuePad;

  function xFor(idx: number) {
    if (periodCount <= 1) return padding.left + plotW / 2;
    return padding.left + (idx / (periodCount - 1)) * plotW;
  }
  function yFor(value: number) {
    return padding.top + plotH - ((value - minValue) / (maxValue - minValue)) * plotH;
  }

  const tickCount = 4;
  for (let i = 0; i <= tickCount; i++) {
    const value = minValue + ((maxValue - minValue) * i) / tickCount;
    const y = yFor(value);
    const line = document.createElementNS(svgNS, "line");
    line.setAttribute("x1", String(padding.left));
    line.setAttribute("x2", String(width - padding.right));
    line.setAttribute("y1", String(y));
    line.setAttribute("y2", String(y));
    line.setAttribute("stroke", "var(--gridline)");
    line.setAttribute("stroke-width", "1");
    svg.appendChild(line);

    const label = document.createElementNS(svgNS, "text");
    label.setAttribute("x", String(padding.left - 8));
    label.setAttribute("y", String(y + 3));
    label.setAttribute("text-anchor", "end");
    label.setAttribute("font-size", "10");
    label.setAttribute("fill", "var(--text-muted)");
    label.textContent = value.toLocaleString("de-DE", { maximumFractionDigits: 0 });
    svg.appendChild(label);
  }

  const baseline = document.createElementNS(svgNS, "line");
  baseline.setAttribute("x1", String(padding.left));
  baseline.setAttribute("x2", String(width - padding.right));
  baseline.setAttribute("y1", String(height - padding.bottom));
  baseline.setAttribute("y2", String(height - padding.bottom));
  baseline.setAttribute("stroke", "var(--baseline)");
  baseline.setAttribute("stroke-width", "1");
  svg.appendChild(baseline);

  [0, Math.floor(periodCount / 2), periodCount - 1].forEach((idx) => {
    if (idx < 0 || idx >= periodCount) return;
    const text = document.createElementNS(svgNS, "text");
    text.setAttribute("x", String(xFor(idx)));
    text.setAttribute("y", String(height - padding.bottom + 18));
    text.setAttribute("text-anchor", "middle");
    text.setAttribute("font-size", "10");
    text.setAttribute("fill", "var(--text-muted)");
    text.textContent = visible[0].points[idx].label;
    svg.appendChild(text);
  });

  visible.forEach((s, seriesIdx) => {
    const color = SERIES_COLORS[seriesIdx % SERIES_COLORS.length];

    // Pfad überspringt null-Werte (Lücke in der Linie statt falscher
    // Interpolation über eine fehlende Periode hinweg).
    let pathData = "";
    s.points.forEach((p, i) => {
      if (p.value === null) return;
      const startsSegment = i === 0 || s.points[i - 1].value === null;
      pathData += `${startsSegment ? "M" : "L"}${xFor(i)} ${yFor(p.value)} `;
    });
    if (pathData) {
      const path = document.createElementNS(svgNS, "path");
      path.setAttribute("d", pathData.trim());
      path.setAttribute("fill", "none");
      path.setAttribute("stroke", color);
      path.setAttribute("stroke-width", "2");
      path.setAttribute("stroke-linecap", "round");
      path.setAttribute("stroke-linejoin", "round");
      svg.appendChild(path);
    }

    s.points.forEach((p, i) => {
      if (p.value === null) return;
      const cx = xFor(i);
      const cy = yFor(p.value);
      const circle = document.createElementNS(svgNS, "circle");
      circle.setAttribute("cx", String(cx));
      circle.setAttribute("cy", String(cy));
      circle.setAttribute("r", "3");
      circle.setAttribute("fill", color);
      svg.appendChild(circle);

      const hitTarget = document.createElementNS(svgNS, "circle");
      hitTarget.setAttribute("cx", String(cx));
      hitTarget.setAttribute("cy", String(cy));
      hitTarget.setAttribute("r", "8");
      hitTarget.setAttribute("fill", "transparent");
      hitTarget.style.cursor = "pointer";
      hitTarget.addEventListener("mouseenter", (evt) => showPeriodTooltip(evt, i, visible));
      hitTarget.addEventListener("mousemove", (evt) => showPeriodTooltip(evt, i, visible));
      hitTarget.addEventListener("mouseleave", hideTooltip);
      svg.appendChild(hitTarget);
    });
  });
}

function showIndexedTooltip(evt: MouseEvent, seriesLabel: string, point: TimeseriesPoint, index: number) {
  const rows: [string, string | number][] = [
    ["Datensatz", seriesLabel],
    ["Jahr", point.year],
    ["Wert", formatNumber(point.value)],
    ["Index (Basisjahr=100)", index.toLocaleString("de-DE", { maximumFractionDigits: 1 })],
    ["Status", point.is_interpolated ? "interpoliert" : "beobachtet"],
  ];
  tooltip.value = { visible: true, x: evt.clientX + 12, y: evt.clientY + 12, rows };
}

// Kombinierte Ansicht mehrerer Datensätze: jede Serie wird auf ihren ersten
// vorhandenen Wert = 100 indexiert (Basisjahr-Index, wie bei
// Aktienkurs-Vergleichen), damit unterschiedliche Einheiten/Skalen auf einer
// gemeinsamen Y-Achse vergleichbar bleiben - siehe Typ-Kommentar oben.
function drawIndexedChart(svg: SVGSVGElement, series: IndexedSeries[]) {
  const visible = series.filter((s) => s.points.length);
  if (!visible.length) return;

  const padding = { top: 16, right: 16, bottom: 28, left: 56 };
  const width = 640;
  const height = 260;
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const indexed = visible.map((s) => {
    const sorted = s.points.slice().sort((a, b) => a.year - b.year);
    const base = sorted[0]?.value;
    const points = base ? sorted.map((p) => ({ year: p.year, index: (p.value / base) * 100, point: p })) : [];
    return { key: s.key, label: s.label, points };
  });

  const allYears = Array.from(new Set(indexed.flatMap((s) => s.points.map((p) => p.year)))).sort((a, b) => a - b);
  if (!allYears.length) return;
  const minYear = allYears[0];
  const maxYear = allYears[allYears.length - 1];

  const allIndexValues = indexed.flatMap((s) => s.points.map((p) => p.index));
  if (!allIndexValues.length) return;
  let minValue = Math.min(100, ...allIndexValues);
  let maxValue = Math.max(100, ...allIndexValues);
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  const valuePad = (maxValue - minValue) * 0.1;
  minValue -= valuePad;
  maxValue += valuePad;

  function xFor(year: number) {
    if (maxYear === minYear) return padding.left + plotW / 2;
    return padding.left + ((year - minYear) / (maxYear - minYear)) * plotW;
  }
  function yFor(value: number) {
    return padding.top + plotH - ((value - minValue) / (maxValue - minValue)) * plotH;
  }

  const tickCount = 4;
  for (let i = 0; i <= tickCount; i++) {
    const value = minValue + ((maxValue - minValue) * i) / tickCount;
    const y = yFor(value);
    const line = document.createElementNS(svgNS, "line");
    line.setAttribute("x1", String(padding.left));
    line.setAttribute("x2", String(width - padding.right));
    line.setAttribute("y1", String(y));
    line.setAttribute("y2", String(y));
    line.setAttribute("stroke", "var(--gridline)");
    line.setAttribute("stroke-width", "1");
    svg.appendChild(line);

    const label = document.createElementNS(svgNS, "text");
    label.setAttribute("x", String(padding.left - 8));
    label.setAttribute("y", String(y + 3));
    label.setAttribute("text-anchor", "end");
    label.setAttribute("font-size", "10");
    label.setAttribute("fill", "var(--text-muted)");
    label.textContent = value.toLocaleString("de-DE", { maximumFractionDigits: 0 });
    svg.appendChild(label);
  }

  // Basislinie bei Index 100 (gestrichelt) - Referenzpunkt "unverändert
  // gegenüber dem ersten Jahr", auf den alle Serien normiert sind.
  const refLine = document.createElementNS(svgNS, "line");
  refLine.setAttribute("x1", String(padding.left));
  refLine.setAttribute("x2", String(width - padding.right));
  refLine.setAttribute("y1", String(yFor(100)));
  refLine.setAttribute("y2", String(yFor(100)));
  refLine.setAttribute("stroke", "var(--baseline)");
  refLine.setAttribute("stroke-width", "1");
  refLine.setAttribute("stroke-dasharray", "3 3");
  svg.appendChild(refLine);

  const baseline = document.createElementNS(svgNS, "line");
  baseline.setAttribute("x1", String(padding.left));
  baseline.setAttribute("x2", String(width - padding.right));
  baseline.setAttribute("y1", String(height - padding.bottom));
  baseline.setAttribute("y2", String(height - padding.bottom));
  baseline.setAttribute("stroke", "var(--baseline)");
  baseline.setAttribute("stroke-width", "1");
  svg.appendChild(baseline);

  [0, Math.floor(allYears.length / 2), allYears.length - 1].forEach((idx) => {
    if (idx < 0 || idx >= allYears.length) return;
    const year = allYears[idx];
    const text = document.createElementNS(svgNS, "text");
    text.setAttribute("x", String(xFor(year)));
    text.setAttribute("y", String(height - padding.bottom + 18));
    text.setAttribute("text-anchor", "middle");
    text.setAttribute("font-size", "10");
    text.setAttribute("fill", "var(--text-muted)");
    text.textContent = String(year);
    svg.appendChild(text);
  });

  indexed.forEach((s, seriesIdx) => {
    const color = SERIES_COLORS[seriesIdx % SERIES_COLORS.length];

    const pathData = s.points.map((p, i) => `${i === 0 ? "M" : "L"}${xFor(p.year)} ${yFor(p.index)}`).join(" ");
    if (pathData) {
      const path = document.createElementNS(svgNS, "path");
      path.setAttribute("d", pathData);
      path.setAttribute("fill", "none");
      path.setAttribute("stroke", color);
      path.setAttribute("stroke-width", "2");
      path.setAttribute("stroke-linecap", "round");
      path.setAttribute("stroke-linejoin", "round");
      svg.appendChild(path);
    }

    s.points.forEach(({ year, index, point }) => {
      const cx = xFor(year);
      const cy = yFor(index);
      const circle = document.createElementNS(svgNS, "circle");
      circle.setAttribute("cx", String(cx));
      circle.setAttribute("cy", String(cy));
      circle.setAttribute("r", "3");
      if (point.is_interpolated) {
        circle.setAttribute("fill", "var(--surface-1)");
        circle.setAttribute("stroke", color);
        circle.setAttribute("stroke-width", "2");
      } else {
        circle.setAttribute("fill", color);
      }
      svg.appendChild(circle);

      const hitTarget = document.createElementNS(svgNS, "circle");
      hitTarget.setAttribute("cx", String(cx));
      hitTarget.setAttribute("cy", String(cy));
      hitTarget.setAttribute("r", "9");
      hitTarget.setAttribute("fill", "transparent");
      hitTarget.style.cursor = "pointer";
      hitTarget.addEventListener("mouseenter", (evt) => showIndexedTooltip(evt, s.label, point, index));
      hitTarget.addEventListener("mousemove", (evt) => showIndexedTooltip(evt, s.label, point, index));
      hitTarget.addEventListener("mouseleave", hideTooltip);
      svg.appendChild(hitTarget);
    });
  });
}

function drawChart() {
  const svg = svgRef.value;
  if (!svg) return;
  while (svg.firstChild) svg.removeChild(svg.firstChild);

  if (props.indexedSeries) {
    drawIndexedChart(svg, props.indexedSeries);
    return;
  }

  if (props.periodSeries) {
    drawPeriodChart(svg, props.periodSeries);
    return;
  }

  const points = (props.points ?? []).slice().sort((a, b) => a.year - b.year);
  if (!points.length) return;

  const padding = { top: 16, right: 16, bottom: 28, left: 72 };
  const width = 640;
  const height = 260;
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const years = points.map((p) => p.year);
  const values = points.map((p) => p.value);
  const minYear = Math.min(...years);
  const maxYear = Math.max(...years);
  let minValue = Math.min(...values);
  let maxValue = Math.max(...values);
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  const valuePad = (maxValue - minValue) * 0.1;
  minValue -= valuePad;
  maxValue += valuePad;

  function xFor(year: number) {
    if (maxYear === minYear) return padding.left + plotW / 2;
    return padding.left + ((year - minYear) / (maxYear - minYear)) * plotW;
  }
  function yFor(value: number) {
    return padding.top + plotH - ((value - minValue) / (maxValue - minValue)) * plotH;
  }

  // Gridlines + y-Ticks
  const tickCount = 4;
  for (let i = 0; i <= tickCount; i++) {
    const value = minValue + ((maxValue - minValue) * i) / tickCount;
    const y = yFor(value);
    const line = document.createElementNS(svgNS, "line");
    line.setAttribute("x1", String(padding.left));
    line.setAttribute("x2", String(width - padding.right));
    line.setAttribute("y1", String(y));
    line.setAttribute("y2", String(y));
    line.setAttribute("stroke", "var(--gridline)");
    line.setAttribute("stroke-width", "1");
    svg.appendChild(line);

    const label = document.createElementNS(svgNS, "text");
    label.setAttribute("x", String(padding.left - 8));
    label.setAttribute("y", String(y + 3));
    label.setAttribute("text-anchor", "end");
    label.setAttribute("font-size", "10");
    label.setAttribute("fill", "var(--text-muted)");
    label.textContent = value.toLocaleString("de-DE", { maximumFractionDigits: 0 });
    svg.appendChild(label);
  }

  // Baseline
  const baseline = document.createElementNS(svgNS, "line");
  baseline.setAttribute("x1", String(padding.left));
  baseline.setAttribute("x2", String(width - padding.right));
  baseline.setAttribute("y1", String(height - padding.bottom));
  baseline.setAttribute("y2", String(height - padding.bottom));
  baseline.setAttribute("stroke", "var(--baseline)");
  baseline.setAttribute("stroke-width", "1");
  svg.appendChild(baseline);

  // X-Achsen-Jahreslabels (erstes, mittleres, letztes)
  [0, Math.floor(points.length / 2), points.length - 1].forEach((idx) => {
    if (idx < 0 || idx >= points.length) return;
    const p = points[idx];
    const text = document.createElementNS(svgNS, "text");
    text.setAttribute("x", String(xFor(p.year)));
    text.setAttribute("y", String(height - padding.bottom + 18));
    text.setAttribute("text-anchor", "middle");
    text.setAttribute("font-size", "10");
    text.setAttribute("fill", "var(--text-muted)");
    text.textContent = String(p.year);
    svg.appendChild(text);
  });

  // Linie
  const pathData = points.map((p, i) => `${i === 0 ? "M" : "L"}${xFor(p.year)} ${yFor(p.value)}`).join(" ");
  const path = document.createElementNS(svgNS, "path");
  path.setAttribute("d", pathData);
  path.setAttribute("fill", "none");
  path.setAttribute("stroke", "var(--series-1)");
  path.setAttribute("stroke-width", "2");
  path.setAttribute("stroke-linecap", "round");
  path.setAttribute("stroke-linejoin", "round");
  svg.appendChild(path);

  // Marker + Hover-Ziele
  points.forEach((p) => {
    const cx = xFor(p.year);
    const cy = yFor(p.value);
    const circle = document.createElementNS(svgNS, "circle");
    circle.setAttribute("cx", String(cx));
    circle.setAttribute("cy", String(cy));
    circle.setAttribute("r", "4");
    if (p.is_interpolated) {
      circle.setAttribute("fill", "var(--surface-1)");
      circle.setAttribute("stroke", "var(--series-1)");
      circle.setAttribute("stroke-width", "2");
    } else {
      circle.setAttribute("fill", "var(--series-1)");
    }
    svg.appendChild(circle);

    const hitTarget = document.createElementNS(svgNS, "circle");
    hitTarget.setAttribute("cx", String(cx));
    hitTarget.setAttribute("cy", String(cy));
    hitTarget.setAttribute("r", "10");
    hitTarget.setAttribute("fill", "transparent");
    hitTarget.style.cursor = "pointer";
    hitTarget.addEventListener("mouseenter", (evt) => showTooltip(evt, p));
    hitTarget.addEventListener("mousemove", (evt) => showTooltip(evt, p));
    hitTarget.addEventListener("mouseleave", hideTooltip);
    svg.appendChild(hitTarget);
  });
}

onMounted(drawChart);
watch(() => [props.points, props.periodSeries, props.indexedSeries], drawChart, { deep: true });
</script>

<template>
  <div class="chart-wrap">
    <svg ref="svgRef" viewBox="0 0 640 260" preserveAspectRatio="none"></svg>
  </div>
  <p v-if="indexedSeries && indexedSeries.length" class="chart-caption">
    <span v-for="(s, i) in indexedSeries" :key="s.key">
      <span class="marker" :style="{ backgroundColor: SERIES_COLORS[i % SERIES_COLORS.length] }"></span>{{ s.label }}
    </span>
    <span class="hint">Indexiert, Basisjahr = 100 (Wert im ersten verfügbaren Jahr je Datensatz)</span>
  </p>
  <p v-else-if="!periodSeries" class="chart-caption">
    <span><span class="marker marker-observed"></span>beobachtet</span>
    <span><span class="marker marker-interpolated"></span>interpoliert</span>
  </p>
  <p v-else-if="periodSeries.length" class="chart-caption">
    <span v-for="(s, i) in periodSeries" :key="s.key">
      <span class="marker" :style="{ backgroundColor: SERIES_COLORS[i % SERIES_COLORS.length] }"></span>{{ s.label }}
    </span>
  </p>
  <div v-if="tooltip.visible" class="chart-tooltip" :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }">
    <dl>
      <template v-for="row in tooltip.rows" :key="row[0]">
        <dt>{{ row[0] }}</dt>
        <dd>{{ row[1] }}</dd>
      </template>
    </dl>
  </div>
</template>
