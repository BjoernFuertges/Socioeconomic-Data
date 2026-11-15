// Client-side pre-check of a pasted/uploaded GeoJSON region before it goes to
// POST /custom-regions/dataset-aggregate. The backend validates again
// (ST_MakeValid, size limits); this only provides understandable error messages for
// the typical mistakes (wrong geometry type, swapped coordinates, projected
// instead of WGS84 coordinates).

export const MAX_GEOJSON_BYTES = 5 * 1024 * 1024; // matches customRegionMaxBodyBytes in the backend

export type RegionGeometry = { type: "Polygon" | "MultiPolygon"; coordinates: unknown };

export type ParsedRegion = { geometry: RegionGeometry } | { error: string };

function extractGeometry(payload: any): any {
  if (payload?.type === "Feature") return payload.geometry;
  if (payload?.type === "FeatureCollection") {
    const features = payload.features;
    if (!Array.isArray(features) || features.length === 0) throw new Error("Die FeatureCollection enthält keine Features.");
    if (features.length > 1) {
      throw new Error("Die FeatureCollection enthält mehrere Features – bitte auf ein Feature reduzieren.");
    }
    return features[0]?.geometry;
  }
  return payload;
}

// Walks all coordinate pairs and checks the WGS84 value range.
// Germany is at lon ≈ 5–16, lat ≈ 47–55: a pair with |x| > 90 in the
// second position indicates swapped axes or projected coordinates.
function checkCoordinates(coords: unknown): string | null {
  if (!Array.isArray(coords)) return "coordinates ist ungültig.";
  if (typeof coords[0] === "number") {
    const [lon, lat] = coords as number[];
    if (typeof lon !== "number" || typeof lat !== "number" || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      return "Ungültige Koordinate.";
    }
    if (Math.abs(lon) > 180 || Math.abs(lat) > 90) {
      return "Koordinaten außerhalb des WGS84-Bereichs (erwartet [lon, lat] in EPSG:4326, keine projizierten Koordinaten).";
    }
    return null;
  }
  for (const c of coords) {
    const err = checkCoordinates(c);
    if (err) return err;
  }
  return null;
}

export function parseRegionGeoJSON(text: string): ParsedRegion {
  const trimmed = text.trim();
  if (!trimmed) return { error: "Bitte ein GeoJSON einfügen oder eine Datei auswählen." };
  if (new Blob([trimmed]).size > MAX_GEOJSON_BYTES) {
    return { error: `Das GeoJSON ist größer als ${MAX_GEOJSON_BYTES / 1024 / 1024} MB.` };
  }

  let payload: unknown;
  try {
    payload = JSON.parse(trimmed);
  } catch (err) {
    return { error: `Kein gültiges JSON: ${err instanceof Error ? err.message : String(err)}` };
  }

  let geometry: any;
  try {
    geometry = extractGeometry(payload);
  } catch (err) {
    return { error: err instanceof Error ? err.message : String(err) };
  }
  if (!geometry || typeof geometry !== "object") return { error: "Das GeoJSON enthält keine Geometrie." };
  if (geometry.type !== "Polygon" && geometry.type !== "MultiPolygon") {
    return { error: `Erwartet wird ein Polygon oder MultiPolygon, erhalten: ${String(geometry.type)}.` };
  }
  const coordError = checkCoordinates(geometry.coordinates);
  if (coordError) return { error: coordError };
  return { geometry: { type: geometry.type, coordinates: geometry.coordinates } };
}
