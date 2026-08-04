export function formatDatasetLabel(routeSlug: string): string {
  return routeSlug.replace(/[-_]/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function formatNumber(value: unknown): string {
  if (typeof value !== "number") return String(value);
  return value.toLocaleString("de-DE", { maximumFractionDigits: 2 });
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
