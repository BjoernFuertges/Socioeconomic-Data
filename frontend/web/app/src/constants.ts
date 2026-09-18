// Slug des Agenturbezirk-Datensatzes im Datenkatalog (GET /datasets/:slug).
// Anders als die übrigen, meta.yaml-basierten Kreis-/Gemeinden-Datensätze hat
// er keine generische /{slug}-Rohdatenroute (siehe DatasetCatalog.vue) und
// keinen eigenen DatasetPicker-Eintrag (RegionBrowser.vue zeigt ihn direkt im
// Agenturbezirk-Panel) - der Slug wird trotzdem an mehreren Stellen für
// Katalog-Verlinkung gebraucht, deshalb hier zentral statt mehrfach hart codiert.
export const AGENTUR_DATASET_SLUG = "arbeitslosenzahlen-afa-agenturbezirke";

// Gängige Abkürzungen der 16 Bundesländer (NUTS-1), keyed nach dem echten
// region_name aus mart_nuts1_summary (Eurostat NUTS_NAME, z.B.
// "Nordrhein-Westfalen") - die Bundesland-Suche in RegionBrowser.vue matcht
// zusätzlich gegen diese Aliase, damit z.B. "NRW" tatsächlich einen Treffer
// findet (der amtliche Name selbst ist keine Abkürzung).
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
