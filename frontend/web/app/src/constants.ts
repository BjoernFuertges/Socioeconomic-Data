// Slug of the Agenturbezirk dataset in the dataset catalog (GET /datasets/:slug).
// Unlike the other meta.yaml-based Kreis/Gemeinden datasets it has no generic
// /{slug} raw-data route (see DatasetCatalog.vue) and no DatasetPicker entry of its
// own (RegionBrowser.vue shows it directly in the Agenturbezirk panel). The slug is
// still needed in several places for catalog linking, so it lives here instead of
// being hard-coded repeatedly.
export const AGENTUR_DATASET_SLUG = "arbeitslosenzahlen-afa-agenturbezirke";

// Catalog slugs without a source-data download (GET /datasets/:slug/source returns
// 404 for them): derived slugs that only exist as dbt aggregation
// (beschaeftigungsquote-nuts3, lau_*, ba_*), and the Agenturbezirk dataset with
// its own monthly structure. All other meta.yaml-based datasets have source data
// (see mart_dataset_source.sql).
export function hasSourceData(slug: string): boolean {
  return (
    slug !== AGENTUR_DATASET_SLUG &&
    slug !== "beschaeftigungsquote-nuts3" &&
    !slug.startsWith("lau_") &&
    !slug.startsWith("ba_")
  );
}

// Common abbreviations of the 16 Bundesländer (NUTS-1), keyed by the real
// region_name from mart_nuts1_summary (Eurostat NUTS_NAME, e.g.
// "Nordrhein-Westfalen"). The Bundesland search in RegionBrowser.vue also matches
// against these aliases so that e.g. "NRW" actually finds a hit (the official
// name itself is not an abbreviation).
export const BUNDESLAND_ALIASES: Record<string, string[]> = {
  "Baden-Württemberg": ["BW", "BaWü"],
  "Bayern": ["BY"],
  "Berlin": ["BE"],
  "Brandenburg": ["BB"],
  "Bremen": ["HB"],
  "Hamburg": ["HH"],
  "Hessen": ["HE"],
  "Mecklenburg-Vorpommern": ["MV"],
  "Niedersachsen": ["NI", "NDS"],
  "Nordrhein-Westfalen": ["NRW"],
  "Rheinland-Pfalz": ["RLP", "RP"],
  "Saarland": ["SL"],
  "Sachsen": ["SN"],
  "Sachsen-Anhalt": ["ST", "LSA"],
  "Schleswig-Holstein": ["SH"],
  "Thüringen": ["TH", "THL"],
};
