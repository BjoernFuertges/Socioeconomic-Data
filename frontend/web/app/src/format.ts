export function formatDatasetLabel(routeSlug: string): string {
  return routeSlug.replace(/[-_]/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

// Expects an ISO date (e.g. "2026-09-11" or "2026-09-11T00:00:00Z") -> "11.09.2026".
export function formatDateDE(value: string): string {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (!match) return value;
  const [, year, month, day] = match;
  return `${day}.${month}.${year}`;
}

// "MM/YYYY" for monthly series (e.g. Agenturbezirke), analogous to the
// year display for NUTS/LAU.
export function formatMonthLabel(year: number, month: number): string {
  return `${String(month).padStart(2, "0")}/${year}`;
}

export function formatNumber(value: unknown): string {
  if (typeof value !== "number") return String(value);
  return value.toLocaleString("de-DE", { maximumFractionDigits: 2 });
}

// Expects a share as a fraction (0..1, e.g. from mart_agentur_nuts3_crosswalk)
// -> "42 %". null/undefined yield a fallback character instead of "NaN %".
export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return "–";
  return `${(value * 100).toLocaleString("de-DE", { maximumFractionDigits: 0 })} %`;
}

// Columns with calendar years/IDs should not get thousands separators.
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
