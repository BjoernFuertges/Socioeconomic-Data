// Slug des Agenturbezirk-Datensatzes im Datenkatalog (GET /datasets/:slug).
// Anders als die übrigen, meta.yaml-basierten Kreis-/Gemeinden-Datensätze hat
// er keine generische /{slug}-Rohdatenroute (siehe DatasetCatalog.vue) und
// keinen eigenen DatasetPicker-Eintrag (RegionBrowser.vue zeigt ihn direkt im
// Agenturbezirk-Panel) - der Slug wird trotzdem an mehreren Stellen für
// Katalog-Verlinkung gebraucht, deshalb hier zentral statt mehrfach hart codiert.
export const AGENTUR_DATASET_SLUG = "arbeitslosenzahlen-afa-agenturbezirke";
