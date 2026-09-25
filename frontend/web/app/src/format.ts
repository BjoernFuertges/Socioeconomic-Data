export function formatDatasetLabel(routeSlug: string): string {
  return routeSlug.replace(/[-_]/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

// Erwartet ein ISO-Datum (z.B. "2026-09-11" oder "2026-09-11T00:00:00Z") -> "11.09.2026".
export function formatDateDE(value: string): string {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (!match) return value;
  const [, year, month, day] = match;
  return `${day}.${month}.${year}`;
}

// "MM/YYYY" für Monatszeitreihen (z.B. Agenturbezirke), analog zur
// Jahresdarstellung bei NUTS/LAU.
export function formatMonthLabel(year: number, month: number): string {
  return `${String(month).padStart(2, "0")}/${year}`;
}

export function formatNumber(value: unknown): string {
  if (typeof value !== "number") return String(value);
  return value.toLocaleString("de-DE", { maximumFractionDigits: 2 });
}

// Erwartet einen Anteil als Bruch (0..1, z.B. aus mart_agentur_nuts3_crosswalk)
// -> "42 %". null/undefined ergeben ein Fallback-Zeichen statt "NaN %".
export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return "–";
  return `${(value * 100).toLocaleString("de-DE", { maximumFractionDigits: 0 })} %`;
}

// Spalten mit Kalenderjahren/IDs sollen keine Tausendertrennzeichen bekommen.
const UNGROUPED_NUMBER_COLUMNS = new Set(["year", "geodata_year"]);

export function formatCell(value: unknown, column: string): string {
  if (value === null || value === undefined) return "–";
  if (typeof value === "boolean") return value ? "Ja" : "Nein";
  if (typeof value === "number") {
    return UNGROUPED_NUMBER_COLUMNS.has(column) ? String(value) : formatNumber(value);
  }
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
