// Sequentielle Ein-Hue-Rampe (Blau, Stufen 100–700) aus dem projektweiten
// Farbschema (siehe styles.css --series-1*). Für Magnitude-Daten (Choroplethe)
// wird in 7 Klassen quantisiert statt kontinuierlich interpoliert – das macht
// die Farbunterschiede zwischen Regionen tatsächlich unterscheidbar.
const SEQUENTIAL_STEPS = ["#cde2fb", "#9ec5f4", "#6da7ec", "#3987e5", "#256abf", "#184f95", "#0d366b"];

// Im Dark Mode "flippt" die Rampe: niedrige Werte sollen zur dunklen
// Kartenoberfläche hin verblassen, hohe Werte müssen hell hervorstechen -
// das Gegenteil der Anordnung im Light Mode.
export function sequentialColor(t: number, isDark: boolean): string {
  const steps = isDark ? [...SEQUENTIAL_STEPS].reverse() : SEQUENTIAL_STEPS;
  const clamped = Math.min(1, Math.max(0, t));
  const idx = Math.round(clamped * (steps.length - 1));
  return steps[idx];
}

export function sequentialSteps(isDark: boolean): string[] {
  return isDark ? [...SEQUENTIAL_STEPS].reverse() : SEQUENTIAL_STEPS;
}

// Neutrales Grau für Regionen ohne Wert im gewählten Jahr - bewusst außerhalb
// der Sequential-Rampe, damit "keine Daten" nie mit einem echten niedrigen
// Wert verwechselt wird.
export const NO_DATA_FILL_LIGHT = "#e1e0d9";
export const NO_DATA_FILL_DARK = "#2c2c2a";
