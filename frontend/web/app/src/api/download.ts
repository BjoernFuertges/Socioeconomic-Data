// Baut die Download-/API-URLs für den kombinierten Regions-Export
// (GET /regions/{region_type}/{region_id}/data) und die Katalog-Rohdaten.
// Getrennt von client.ts, weil hier keine Antwort geholt, sondern nur eine
// URL erzeugt wird: Downloads laufen direkt als <a href> gegen die API
// (Streaming, kein Blob im Speicher), und dieselbe URL wird als curl-Befehl
// zum Kopieren angezeigt.
import { API_BASE } from "./client";
import type { QueryParams } from "./client";

export type DownloadFormat = "csv" | "json";

// raw = Ursprungswerte der Region, aggregated = aus anderen Regionen berechnet.
export type DownloadScope = "raw" | "aggregated" | "both";

export type RegionType = "nuts" | "lau" | "agentur";

export const FORMAT_MIME: Record<DownloadFormat, string> = {
  csv: "text/csv",
  json: "application/json",
};

// API_BASE ist in Produktion relativ ("/api/v1"), weil Frontend und API
// hinter demselben Origin liegen. Für Links zum Kopieren und curl-Befehle
// muss daraus eine absolute URL werden, die auch außerhalb der Seite
// funktioniert.
export function absoluteApiUrl(path: string, params?: QueryParams): string {
  const base = API_BASE.startsWith("http")
    ? API_BASE
    : new URL(API_BASE, window.location.origin).toString().replace(/\/+$/, "");
  const url = new URL(base + path);
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value === undefined || value === null || value === "") continue;
    url.searchParams.set(key, String(value));
  }
  return url.toString();
}

export interface RegionDataQuery {
  regionType: RegionType;
  regionId: string;
  // Leer = alle für die Region verfügbaren Datensätze.
  datasets?: string[];
  scope?: DownloadScope;
  fromYear?: number | null;
  toYear?: number | null;
}

function regionDataParams(query: RegionDataQuery): QueryParams {
  return {
    datasets: query.datasets?.length ? query.datasets.join(",") : undefined,
    scope: query.scope && query.scope !== "both" ? query.scope : undefined,
    from_year: query.fromYear ?? undefined,
    to_year: query.toYear ?? undefined,
  };
}

// URL für den Browser-Download: `format` statt Accept-Header, weil ein
// <a href>-Download keine Header setzen kann; `download=1` liefert
// Content-Disposition: attachment.
export function regionDownloadUrl(query: RegionDataQuery, format: DownloadFormat): string {
  return absoluteApiUrl(`/regions/${query.regionType}/${encodeURIComponent(query.regionId)}/data`, {
    ...regionDataParams(query),
    format,
    download: 1,
  });
}

// URL für curl/Skripte: ohne format/download, weil das Format dort über den
// HTTP-konformen Accept-Header ausgehandelt wird
export function regionApiUrl(query: RegionDataQuery): string {
  return absoluteApiUrl(`/regions/${query.regionType}/${encodeURIComponent(query.regionId)}/data`, regionDataParams(query));
}

export function curlCommand(url: string, format: DownloadFormat, filename: string): string {
  return `curl -H 'Accept: ${FORMAT_MIME[format]}' -o '${filename}.${format}' '${url}'`;
}

// Dateiname für die heruntergeladene Datei bzw. das curl-Ziel.
export function regionFilename(query: RegionDataQuery): string {
  const scope = query.scope && query.scope !== "both" ? `_${query.scope}` : "";
  return `${query.regionType}_${query.regionId}${scope}`.toLowerCase().replace(/[^a-z0-9_-]+/g, "_");
}

// URL für den gleichzeitigen Metadaten-Export (Herkunft/Lizenz/Beschreibung
// der im Daten-Export enthaltenen Datensätze) über GET /datasets bzw.
// GET /datasets/{slug} bei genau einem Slug (spart dort den Array-Join).
export function datasetMetadataUrl(slugs: string[], format: DownloadFormat): string {
  if (slugs.length === 1) {
    return absoluteApiUrl(`/datasets/${encodeURIComponent(slugs[0])}`, { format, download: 1 });
  }
  return absoluteApiUrl("/datasets", { datasets: slugs.join(","), format, download: 1 });
}

// Pendant zu datasetMetadataUrl für curl/Skripte: ohne format/download, das
// Format wird dort über den Accept-Header ausgehandelt.
export function datasetMetadataApiUrl(slugs: string[]): string {
  if (slugs.length === 1) {
    return absoluteApiUrl(`/datasets/${encodeURIComponent(slugs[0])}`);
  }
  return absoluteApiUrl("/datasets", { datasets: slugs.join(",") });
}

export function datasetMetadataFilename(baseFilename: string): string {
  return `${baseFilename}_metadata`;
}

// Löst für jede der übergebenen URLs einen Browser-Download aus (temporäres
// <a download>, wie beim einzelnen Datei-Download) - damit ein Klick sowohl
// die Daten- als auch (optional) die Metadaten-Datei anstößt, weiterhin ohne
// Blob/Fetch, rein als Streaming-Downloads gegen die API. Chromium bricht
// einen bereits laufenden <a download>-Klick ab, sobald im selben Tick ein
// zweiter ausgelöst wird (beobachtet: von zwei gleichzeitig geklickten Links
// kam nur der zweite tatsächlich als Datei an) - deshalb zwischen den Klicks
// einen Tick warten statt sie in einer synchronen Schleife abzufeuern.
export function triggerDownloads(urls: string[]): void {
  urls.forEach((url, i) => {
    setTimeout(() => {
      const link = document.createElement("a");
      link.href = url;
      link.download = "";
      document.body.appendChild(link);
      link.click();
      link.remove();
    }, i * 300);
  });
}

// Kopiert Text in die Zwischenablage; meldet per Rückgabewert, ob es geklappt
// hat (navigator.clipboard fehlt in unsicheren Kontexten, z.B. http:// mit
// fremdem Host).
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
