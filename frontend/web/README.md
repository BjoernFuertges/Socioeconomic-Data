# `/web`

Web application specific components: static web assets, server side templates and SPAs.

## `app/` – Sozioökonomische-Daten-Frontend

Vue 3 + TypeScript-Single-Page-App (Vite) zum Browsen der NUTS-3-Regionen und
LAU-Gemeinden samt ihrer sozioökonomischen Zeitreihen sowie einer Karte mit
(interpolierten) Werten pro NUTS-3-Region. Es spricht die Go-API per
`fetch()` an (CORS ist serverseitig bereits offen) und läuft komplett
unabhängig vom Go-Server.

### Starten

1. Node (siehe `.nvmrc`/`package.json` – aktuell getestet mit Node 22/23)
   aktivieren, z.B. mit nvm:

   ```bash
   cd app
   nvm use
   npm install
   ```

2. Backend starten (siehe [`cmd/soziodata`](../cmd/soziodata/main.go) bzw.
   [Local development](../../docs/backend/development.md)).

3. Dev-Server starten:

   ```bash
   npm run dev
   ```

   und die ausgegebene URL (Standard `http://localhost:5173`) öffnen. Die
   API-Basis-URL wird über die Vite-Env-Variable `VITE_API_BASE_URL`
   konfiguriert (Default in `.env.development`:
   `http://localhost:8081/api/v1`, passend zu `cmd/soziodata/main.go`).
   Für abweichende lokale Werte eine `.env.local` (gitignored) anlegen.
   Optional kann `VITE_DOCS_URL` gesetzt werden (Link zur veröffentlichten
   mkdocs-Doku, z.B. GitLab Pages) – ohne die Variable bleibt der
   Dokumentations-Link im Header ausgeblendet.

### Produktions-Build

```bash
npm run build
```

erzeugt ein statisches Bundle in `dist/`. Für den Produktivbetrieb wird das
Bundle in `Dockerfile` per Multi-Stage-Build (Node → nginx) gebaut und über
den `frontend`-Service in
[`backend/deployments/docker-compose.yml`](../deployments/docker-compose.yml)
hinter Traefik ausgeliefert (siehe dortige Labels für das Routing). Dort läuft
das Frontend auf derselben Domain wie die API, daher ist `VITE_API_BASE_URL`
im Produktions-Build nicht gesetzt – der Client fällt dann auf den relativen
Pfad `/api/v1` zurück.
