<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type { TimeseriesPoint } from "../types/api";
import { formatMonthLabel, formatNumber } from "../format";

type PeriodSeries = {
  key: string;
  label: string;
  points: { label: string; value: number | null }[];
};

// For comparing several datasets with different units/scales (combined view in
// DatasetPicker) - each series keeps its own years (no shared period-grid length as
// with `periodSeries` needed); the x-axis positions by the actual year. See
// drawIndexedChart(): values are indexed to base year = 100, because a shared value
// y-axis would be misleading for e.g. "Durchschnittsalter" (~40) vs.
// "Arbeitslosenquote" (~5%) (no dual-axis chart, see dataviz skill: two scales ->
// one chart only indexed to a base).
//
// Points with `month` (1-12) are monthly values (e.g. BA Arbeitslose of an
// Agenturbezirk), points without `month` yearly values. If at least one series has
// monthly values, the x-axis runs in time (year + month fraction): monthly values
// sit in the middle of their month, yearly values in the middle of their year, and
// the index base is the yearly average (monthly series) or yearly value in the first
// year in which ALL series have data - a per-series "first value" base would not be
// comparable when the series start at different times.
type IndexedPoint = TimeseriesPoint & { month?: number };
type IndexedSeries = {
  key: string;
  label: string;
  points: IndexedPoint[];
};

// Pass either `points` (yearly series with interpolation status, NUTS/LAU),
// `periodSeries` (one or more monthly/period series without interpolation, e.g.
// Agenturbezirk Arbeitslose/Unterbeschäftigung) or `indexedSeries` (several yearly
// series of different datasets, combined/indexed). All series in `periodSeries`
// must share the same period-grid length/order (values may be `null` where a series
// has no value for that period) - the x-axis is positioned by index, not by label.
const props = defineProps<{
  points?: TimeseriesPoint[];
  periodSeries?: PeriodSeries[];
  indexedSeries?: IndexedSeries[];
}>();

// Order matches the validated slot order of the dataviz skill (blue/orange/aqua/
// yellow) - fixed categorical assignment, not cyclically reshuffled. Caps the
// combined view at 4 series (see DatasetPicker.vue).
const SERIES_COLORS = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)"];

const svgRef = ref<SVGSVGElement | null>(null);
const tooltip = ref<{ visible: boolean; x: number; y: number; rows: [string, string | number][] }>({
  visible: false,
  x: 0,
  y: 0,
  rows: [],
});

const svgNS = "http://www.w3.org/2000/svg";

// Third status besides observed/interpolated - see the is_computed comment on
// TimeseriesPoint (types/api.ts): NUTS-0/1/2 values aggregated up are neither a
// single observation nor a gap fill, but a sum/weighted average over several Kreise.
function statusLabel(point: TimeseriesPoint): string {
  if (point.is_computed) return "berechnet";
  return point.is_interpolated ? "interpoliert" : "beobachtet";
}

// Legend shows only the statuses actually present in `points` - an aggregate series
// (NUTS-0/1/2) consists e.g. solely of "berechnet" points, so "beobachtet"/
// "interpoliert" would be misleading there.
const legendItems = computed(() => {
  const pts = props.points ?? [];
  const items: { key: string; markerClass: string; label: string }[] = [];
  if (pts.some((p) => !p.is_computed && !p.is_interpolated)) {
    items.push({ key: "observed", markerClass: "marker-observed", label: "beobachtet" });
  }
  if (pts.some((p) => !p.is_computed && p.is_interpolated)) {
    items.push({ key: "interpolated", markerClass: "marker-interpolated", label: "interpoliert" });
  }
  if (pts.some((p) => p.is_computed)) {
    items.push({ key: "computed", markerClass: "marker-computed", label: "berechnet" });
  }
  return items;
});

// Status legend of the combined view (shape instead of color, since color identifies
// the series): only statuses that actually occur for yearly values.
// Monthly values are drawn as a line without markers and don't count.
const indexedLegendItems = computed(() => {
  const pts = (props.indexedSeries ?? []).flatMap((s) => s.points).filter((p) => p.month == null);
  const items: { key: string; markerClass: string; label: string }[] = [];
  if (pts.some((p) => !p.is_computed && !p.is_interpolated)) {
    items.push({ key: "observed", markerClass: "marker-observed", label: "beobachtet" });
  }
  if (pts.some((p) => !p.is_computed && p.is_interpolated)) {
    items.push({ key: "interpolated", markerClass: "marker-interpolated", label: "interpoliert" });
  }
  if (pts.some((p) => p.is_computed)) {
    items.push({ key: "computed", markerClass: "marker-computed", label: "berechnet" });
  }
  return items;
});

function showTooltip(evt: MouseEvent, point: TimeseriesPoint) {
  const rows: [string, string | number][] = [
    ["Jahr", point.year],
    ["Wert", formatNumber(point.value)],
    ["Status", statusLabel(point)],
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

// Tooltip shows the values of ALL visible series at the hovered period (not just
// the series whose marker was hovered) - so e.g. Arbeitslose/Unterbeschäftigung
// stay comparable at the same period.
function showPeriodTooltip(evt: MouseEvent, idx: number, series: PeriodSeries[]) {
  const label = series[0]?.points[idx]?.label ?? "";
  const rows: [string, string | number][] = [["Periode", label]];
  for (const s of series) {
    const value = s.points[idx]?.value;
    rows.push([s.label, value !== null && value !== undefined ? formatNumber(value) : "–"]);
  }
  tooltip.value = { visible: true, x: evt.clientX + 12, y: evt.clientY + 12, rows };
}

// Simple index-based line chart variant for one or more period series (e.g.
// Agenturbezirk monthly values) without interpolation status.
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

    // Path skips null values (gap in the line instead of false interpolation
    // across a missing period).
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

// Base year of the index for mixed monthly/yearly series (null otherwise) - for the caption.
const indexBaseYear = ref<number | null>(null);
const hasMonthlySeries = computed(() => !!props.indexedSeries?.some((s) => s.points.some((p) => p.month !== undefined)));

function showIndexedTooltip(evt: MouseEvent, seriesLabel: string, point: IndexedPoint, index: number) {
  const isMonthly = point.month !== undefined;
  const rows: [string, string | number][] = [
    ["Datensatz", seriesLabel],
    isMonthly ? ["Monat", formatMonthLabel(point.year, point.month!)] : ["Jahr", point.year],
    ["Wert", formatNumber(point.value)],
    [
      indexBaseYear.value !== null ? `Index (${indexBaseYear.value}=100)` : "Index (Basisjahr=100)",
      index.toLocaleString("de-DE", { maximumFractionDigits: 1 }),
    ],
    ["Status", isMonthly ? "Monatswert" : statusLabel(point)],
  ];
  tooltip.value = { visible: true, x: evt.clientX + 12, y: evt.clientY + 12, rows };
}

// Combined view of several datasets: each series is indexed to its first existing
// value = 100 (base-year index, as in stock price comparisons), so different
// units/scales stay comparable on a shared y-axis - see the type comment above.
function drawIndexedChart(svg: SVGSVGElement, series: IndexedSeries[]) {
  const visible = series.filter((s) => s.points.length);
  if (!visible.length) return;

  const padding = { top: 16, right: 16, bottom: 28, left: 56 };
  const width = 640;
  const height = 260;
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const mixed = visible.some((s) => s.points.some((p) => p.month !== undefined));

  // Yearly reference value per series and year (monthly series: average of the
  // existing months) - only for choosing the shared base year.
  const annualByYear = visible.map((s) => {
    const acc = new Map<number, { sum: number; n: number }>();
    for (const p of s.points) {
      const a = acc.get(p.year) ?? { sum: 0, n: 0 };
      a.sum += p.value;
      a.n += 1;
      acc.set(p.year, a);
    }
    return new Map(Array.from(acc, ([year, { sum, n }]) => [year, sum / n] as [number, number]));
  });
  let commonBaseYear: number | null = null;
  if (mixed) {
    const commonYears = Array.from(annualByYear[0].keys())
      .filter((y) => annualByYear.every((m) => (m.get(y) ?? 0) !== 0))
      .sort((a, b) => a - b);
    commonBaseYear = commonYears[0] ?? null;
  }
  indexBaseYear.value = commonBaseYear;

  // Time position: for purely yearly series the year itself (unchanged behavior),
  // mixed: year + fraction (month middle or year middle).
  const timeOf = (p: IndexedPoint) => (!mixed ? p.year : p.year + (p.month !== undefined ? (p.month - 0.5) / 12 : 0.5));

  const indexed = visible.map((s, i) => {
    const sorted = s.points.slice().sort((a, b) => a.year - b.year || (a.month ?? 0) - (b.month ?? 0));
    const base = commonBaseYear !== null ? annualByYear[i].get(commonBaseYear) : sorted[0]?.value;
    const points = base ? sorted.map((p) => ({ year: timeOf(p), index: (p.value / base) * 100, point: p })) : [];
    return { key: s.key, label: s.label, points };
  });

  const allYears = Array.from(new Set(indexed.flatMap((s) => s.points.map((p) => p.year)))).sort((a, b) => a - b);
  if (!allYears.length) return;
  const minYear = mixed ? Math.floor(allYears[0]) : allYears[0];
  const maxYear = mixed ? Math.floor(allYears[allYears.length - 1]) + 1 : allYears[allYears.length - 1];

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

  // Baseline at index 100 (dashed) - reference point "unchanged versus the first
  // year" to which all series are normalized.
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

  // Mixed: year ticks centered under the respective year (where the yearly values
  // also sit); otherwise first/middle/last value as before.
  const tickYears = mixed
    ? [minYear, Math.floor((minYear + maxYear - 1) / 2), maxYear - 1]
    : [0, Math.floor(allYears.length / 2), allYears.length - 1].filter((idx) => idx >= 0 && idx < allYears.length).map((idx) => allYears[idx]);
  tickYears.forEach((year) => {
    const text = document.createElementNS(svgNS, "text");
    text.setAttribute("x", String(xFor(mixed ? year + 0.5 : year)));
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
      const isMonthly = point.month !== undefined;
      // Monthly series: line only, no markers (hundreds of points would be
      // unreadable) - hover target remains.
      if (isMonthly) {
        // no visible marker
      } else if (point.is_computed) {
        const size = 6;
        const rect = document.createElementNS(svgNS, "rect");
        rect.setAttribute("x", String(cx - size / 2));
        rect.setAttribute("y", String(cy - size / 2));
        rect.setAttribute("width", String(size));
        rect.setAttribute("height", String(size));
        rect.setAttribute("rx", "1");
        rect.setAttribute("fill", color);
        svg.appendChild(rect);
      } else {
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
      }

      const hitTarget = document.createElementNS(svgNS, "circle");
      hitTarget.setAttribute("cx", String(cx));
      hitTarget.setAttribute("cy", String(cy));
      hitTarget.setAttribute("r", isMonthly ? "4" : "9");
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

  // Gridlines + y-ticks
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

  // X-axis year labels (first, middle, last)
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

  // Line. Years without a value (e.g. invalid source values that could not be
  // interpolated) are shown as a gap: the solid line breaks and a dashed muted
  // segment bridges it. The regular step is the smallest distance between
  // neighbouring years, so series with a coarser grid (e.g. every 2 years) do not break
  // everywhere and a series with many missing years still shows its gaps.
  const steps = points
    .slice(1)
    .map((p, i) => p.year - points[i].year)
    .sort((a, b) => a - b);
  const regularStep = steps.length ? Math.max(1, steps[0]) : 1;
  let solidPath = "";
  let gapPath = "";
  points.forEach((p, i) => {
    const x = xFor(p.year);
    const y = yFor(p.value);
    const isGap = i > 0 && p.year - points[i - 1].year > regularStep;
    if (i === 0 || isGap) {
      if (isGap) {
        gapPath += `M${xFor(points[i - 1].year)} ${yFor(points[i - 1].value)} L${x} ${y} `;
      }
      solidPath += `M${x} ${y} `;
    } else {
      solidPath += `L${x} ${y} `;
    }
  });
  if (gapPath) {
    const gap = document.createElementNS(svgNS, "path");
    gap.setAttribute("d", gapPath.trim());
    gap.setAttribute("fill", "none");
    gap.setAttribute("stroke", "var(--text-muted)");
    gap.setAttribute("stroke-width", "1.5");
    gap.setAttribute("stroke-dasharray", "3 4");
    gap.setAttribute("opacity", "0.7");
    const gapTitle = document.createElementNS(svgNS, "title");
    gapTitle.textContent = "Keine Daten in diesem Zeitraum";
    gap.appendChild(gapTitle);
    svg.appendChild(gap);
  }
  const path = document.createElementNS(svgNS, "path");
  path.setAttribute("d", solidPath.trim());
  path.setAttribute("fill", "none");
  path.setAttribute("stroke", "var(--series-1)");
  path.setAttribute("stroke-width", "2");
  path.setAttribute("stroke-linecap", "round");
  path.setAttribute("stroke-linejoin", "round");
  svg.appendChild(path);

  // Markers + hover targets
  points.forEach((p) => {
    const cx = xFor(p.year);
    const cy = yFor(p.value);
    if (p.is_computed) {
      const size = 8;
      const rect = document.createElementNS(svgNS, "rect");
      rect.setAttribute("x", String(cx - size / 2));
      rect.setAttribute("y", String(cy - size / 2));
      rect.setAttribute("width", String(size));
      rect.setAttribute("height", String(size));
      rect.setAttribute("rx", "1.5");
      rect.setAttribute("fill", "var(--series-1)");
      svg.appendChild(rect);
    } else {
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
    }

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
    <span v-for="item in indexedLegendItems" :key="item.key">
      <span class="marker marker-neutral" :class="item.markerClass"></span>{{ item.label }}
    </span>
    <span v-if="hasMonthlySeries" class="hint">
      Indexiert{{ indexBaseYear !== null ? `, ${indexBaseYear} = 100 (Jahresdurchschnitt bzw. Jahreswert)` : "" }}.
      Linie ohne Marker = Monatswerte, Punkte/Quadrate = Jahreswerte (Jahresmitte).
    </span>
    <span v-else class="hint">Indexiert, Basisjahr = 100 (Wert im ersten verfügbaren Jahr je Datensatz)</span>
  </p>
  <p v-else-if="!periodSeries && legendItems.length" class="chart-caption">
    <span v-for="item in legendItems" :key="item.key"><span class="marker" :class="item.markerClass"></span>{{ item.label }}</span>
  </p>
  <p v-else-if="periodSeries && periodSeries.length" class="chart-caption">
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
