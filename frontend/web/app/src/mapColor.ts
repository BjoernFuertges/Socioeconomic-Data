// Sequential single-hue ramp (blue, steps 100–700) from the project-wide
// color scheme (see styles.css --series-1*). Magnitude data (choropleth)
// is quantized into 7 classes instead of interpolated continuously, which makes
// the color differences between regions actually distinguishable.
const SEQUENTIAL_STEPS = ["#cde2fb", "#9ec5f4", "#6da7ec", "#3987e5", "#256abf", "#184f95", "#0d366b"];

// In dark mode the ramp "flips": low values should fade into the dark map
// surface, high values must stand out brightly - the opposite of the light-mode
// order.
export function sequentialColor(t: number, isDark: boolean): string {
  const steps = isDark ? [...SEQUENTIAL_STEPS].reverse() : SEQUENTIAL_STEPS;
  const clamped = Math.min(1, Math.max(0, t));
  const idx = Math.round(clamped * (steps.length - 1));
  return steps[idx];
}

export function sequentialSteps(isDark: boolean): string[] {
  return isDark ? [...SEQUENTIAL_STEPS].reverse() : SEQUENTIAL_STEPS;
}

// Neutral gray for regions without a value in the selected year - deliberately outside
// the sequential ramp so "no data" is never confused with a genuinely low
// value.
export const NO_DATA_FILL_LIGHT = "#e1e0d9";
export const NO_DATA_FILL_DARK = "#2c2c2a";

// Dedicated color for the "Kein Datensatz (nur Karte)" mode, where ALL regions get
// this style - NO_DATA_FILL_DARK is too close to the dark map background
// (Carto dark_all) and would make the custom GeoJSON layer practically invisible.
// Deliberately a blue-gray rather than a plain gray, so it doesn't read as a neutral
// "no data" within a choropleth but as a distinct, clearly visible state.
export const MAP_ONLY_FILL_LIGHT = "#8b95a6";
export const MAP_ONLY_FILL_DARK = "#6b7893";
