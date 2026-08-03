# `/web`

Web application specific components: static web assets, server side templates and SPAs.

## `static/` – Sozioökonomische-Daten-Frontend

Schlichtes, statisches Frontend (HTML/CSS/vanilla JS, kein Build-Schritt) zum
Browsen der NUTS-3-Regionen und LAU-Gemeinden samt ihrer sozioökonomischen
Zeitreihen. Es spricht die Go-API per `fetch()` an (CORS ist serverseitig
bereits offen) und läuft komplett unabhängig vom Go-Server.

### Starten

1. Konfiguration anlegen:

   ```bash
   cp static/config.example.js static/config.js
   ```

   In `static/config.js` `apiBaseUrl` (Standard: `http://localhost:8081/api/v1`,
   passend zu `cmd/soziodata/main.go`) und optional `docsUrl` (Link zur
   veröffentlichten mkdocs-Doku, z.B. GitLab Pages) setzen. `config.js` ist
   bewusst in `.gitignore`, damit jede Umgebung ihre eigene URL setzen kann,
   ohne dass sie fest im Repo landet.

2. Backend starten (siehe [`cmd/soziodata`](../cmd/soziodata/main.go) bzw.
   [Local development](../../docs/backend/development.md)).

3. Die statischen Dateien mit einem beliebigen Webserver ausliefern, z.B.:

   ```bash
   cd static && python3 -m http.server 8082
   ```

   und `http://localhost:8082` öffnen.
