// Builds the download/API URLs for the combined region export
// (GET /regions/{region_type}/{region_id}/data) and the catalog raw data.
// Separate from client.ts because nothing is fetched here, only a URL is built:
// downloads go straight to the API as <a href> (streaming, no blob in memory),
// and the same URL is shown as a copyable curl command.
import { API_BASE } from "./client";
import type { QueryParams } from "./client";

export type DownloadFormat = "csv" | "json";

// raw = the region's original values, aggregated = computed from other regions.
export type DownloadScope = "raw" | "aggregated" | "both";

export type RegionType = "nuts" | "lau" | "agentur";

export const FORMAT_MIME: Record<DownloadFormat, string> = {
  csv: "text/csv",
  json: "application/json",
};

// API_BASE is relative in production ("/api/v1") because frontend and API share
// an origin. Links to copy and curl commands need an absolute URL that also
// works outside the page.
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
  // Empty = all datasets available for the region.
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

// URL for the browser download: `format` instead of an Accept header because an
// <a href> download can't set headers; `download=1` yields
// Content-Disposition: attachment.
export function regionDownloadUrl(query: RegionDataQuery, format: DownloadFormat): string {
  return absoluteApiUrl(`/regions/${query.regionType}/${encodeURIComponent(query.regionId)}/data`, {
    ...regionDataParams(query),
    format,
    download: 1,
  });
}

// URL for curl/scripts: without format/download, since the format is negotiated
// there via the standard Accept header.
export function regionApiUrl(query: RegionDataQuery): string {
  return absoluteApiUrl(`/regions/${query.regionType}/${encodeURIComponent(query.regionId)}/data`, regionDataParams(query));
}

export function curlCommand(url: string, format: DownloadFormat, filename: string): string {
  return `curl -H 'Accept: ${FORMAT_MIME[format]}' -o '${filename}.${format}' '${url}'`;
}

// File name for the downloaded file or the curl target.
export function regionFilename(query: RegionDataQuery): string {
  const scope = query.scope && query.scope !== "both" ? `_${query.scope}` : "";
  return `${query.regionType}_${query.regionId}${scope}`.toLowerCase().replace(/[^a-z0-9_-]+/g, "_");
}

// URL for the simultaneous metadata export (origin/license/description of the
// datasets in the data export) via GET /datasets, or GET /datasets/{slug} for
// exactly one slug (saves the array join there).
export function datasetMetadataUrl(slugs: string[], format: DownloadFormat): string {
  if (slugs.length === 1) {
    return absoluteApiUrl(`/datasets/${encodeURIComponent(slugs[0])}`, { format, download: 1 });
  }
  return absoluteApiUrl("/datasets", { datasets: slugs.join(","), format, download: 1 });
}

// Counterpart to datasetMetadataUrl for curl/scripts: without format/download,
// the format is negotiated via the Accept header.
export function datasetMetadataApiUrl(slugs: string[]): string {
  if (slugs.length === 1) {
    return absoluteApiUrl(`/datasets/${encodeURIComponent(slugs[0])}`);
  }
  return absoluteApiUrl("/datasets", { datasets: slugs.join(",") });
}

// Source data of a dataset (GET /datasets/{slug}/source): the values exactly as
// delivered by the geo-mapper, without harmonization.
export function datasetSourceUrl(slug: string, format: DownloadFormat): string {
  return absoluteApiUrl(`/datasets/${encodeURIComponent(slug)}/source`, { format, download: 1 });
}

export function datasetSourceApiUrl(slug: string): string {
  return absoluteApiUrl(`/datasets/${encodeURIComponent(slug)}/source`);
}

export function datasetMetadataFilename(baseFilename: string): string {
  return `${baseFilename}_metadata`;
}

// Triggers a browser download for each given URL (temporary <a download>, like a
// single-file download), so one click fetches both the data and (optionally) the
// metadata file, still as streaming downloads without blob/fetch. Chromium aborts
// a running <a download> click when a second one fires in the same tick (observed:
// of two links clicked together only the second arrived as a file), so wait a tick
// between clicks instead of firing them in a synchronous loop.
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

// Copies text to the clipboard; the return value tells whether it worked
// (navigator.clipboard is missing in insecure contexts, e.g. http:// on a
// foreign host).
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
