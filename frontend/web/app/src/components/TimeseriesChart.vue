<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import type { TimeseriesPoint } from "../types/api";
import { formatNumber } from "../format";

// Entweder `points` (Jahreszeitreihe mit Interpolationsstatus, NUTS/LAU)
// oder `periodPoints` (einfache Monats-/Periodenreihe ohne Interpolation,
// z.B. Agenturbezirke) übergeben.
const props = defineProps<{
  points?: TimeseriesPoint[];
  periodPoints?: { label: string; value: number }[];
}>();

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

function showPeriodTooltip(evt: MouseEvent, point: { label: string; value: number }) {
  tooltip.value = {
    visible: true,
    x: evt.clientX + 12,
    y: evt.clientY + 12,
    rows: [
      ["Periode", point.label],
      ["Wert", formatNumber(point.value)],
    ],
  };
}

// Einfache, indexbasierte Linienchart-Variante für Perioden-Reihen (z.B.
// Agenturbezirk-Monatswerte) ohne Interpolationsstatus.
function drawPeriodChart(svg: SVGSVGElement, points: { label: string; value: number }[]) {
  if (!points.length) return;

  const padding = { top: 16, right: 16, bottom: 28, left: 72 };
  const width = 640;
  const height = 260;
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const values = points.map((p) => p.value);
  let minValue = Math.min(...values);
  let maxValue = Math.max(...values);
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  const valuePad = (maxValue - minValue) * 0.1;
  minValue -= valuePad;
  maxValue += valuePad;

  function xFor(idx: number) {
    if (points.length <= 1) return padding.left + plotW / 2;
    return padding.left + (idx / (points.length - 1)) * plotW;
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

  [0, Math.floor(points.length / 2), points.length - 1].forEach((idx) => {
    if (idx < 0 || idx >= points.length) return;
    const text = document.createElementNS(svgNS, "text");
    text.setAttribute("x", String(xFor(idx)));
    text.setAttribute("y", String(height - padding.bottom + 18));
    text.setAttribute("text-anchor", "middle");
    text.setAttribute("font-size", "10");
    text.setAttribute("fill", "var(--text-muted)");
    text.textContent = points[idx].label;
    svg.appendChild(text);
  });

  const pathData = points.map((p, i) => `${i === 0 ? "M" : "L"}${xFor(i)} ${yFor(p.value)}`).join(" ");
  const path = document.createElementNS(svgNS, "path");
  path.setAttribute("d", pathData);
  path.setAttribute("fill", "none");
  path.setAttribute("stroke", "var(--series-1)");
  path.setAttribute("stroke-width", "2");
  path.setAttribute("stroke-linecap", "round");
  path.setAttribute("stroke-linejoin", "round");
  svg.appendChild(path);

  points.forEach((p, i) => {
    const cx = xFor(i);
    const cy = yFor(p.value);
    const circle = document.createElementNS(svgNS, "circle");
    circle.setAttribute("cx", String(cx));
    circle.setAttribute("cy", String(cy));
    circle.setAttribute("r", "3");
    circle.setAttribute("fill", "var(--series-1)");
    svg.appendChild(circle);

    const hitTarget = document.createElementNS(svgNS, "circle");
    hitTarget.setAttribute("cx", String(cx));
    hitTarget.setAttribute("cy", String(cy));
    hitTarget.setAttribute("r", "8");
    hitTarget.setAttribute("fill", "transparent");
    hitTarget.style.cursor = "pointer";
    hitTarget.addEventListener("mouseenter", (evt) => showPeriodTooltip(evt, p));
    hitTarget.addEventListener("mousemove", (evt) => showPeriodTooltip(evt, p));
    hitTarget.addEventListener("mouseleave", hideTooltip);
    svg.appendChild(hitTarget);
  });
}

function drawChart() {
  const svg = svgRef.value;
  if (!svg) return;
  while (svg.firstChild) svg.removeChild(svg.firstChild);

  if (props.periodPoints) {
    drawPeriodChart(svg, props.periodPoints);
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
watch(() => [props.points, props.periodPoints], drawChart, { deep: true });
</script>

<template>
  <div class="chart-wrap">
    <svg ref="svgRef" viewBox="0 0 640 260" preserveAspectRatio="none"></svg>
  </div>
  <p v-if="!periodPoints" class="chart-caption">
    <span><span class="marker marker-observed"></span>beobachtet</span>
    <span><span class="marker marker-interpolated"></span>interpoliert</span>
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
