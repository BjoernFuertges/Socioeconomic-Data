// Kopiere diese Datei zu "config.js" und passe die Werte an.
// config.js wird nicht eingecheckt (siehe .gitignore), damit jede
// Umgebung (lokal, Server) ihre eigene API-/Docs-URL setzen kann.
window.APP_CONFIG = {
  // Basis-URL der Go-API, inkl. Versionspfad.
  apiBaseUrl: "http://localhost:8081/api/v1",

  // URL der veröffentlichten mkdocs-Dokumentation (z.B. GitLab Pages).
  // Leer lassen, um den Dokumentations-Link im Header auszublenden.
  docsUrl: "",
};
