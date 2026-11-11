# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers the `frontend/` directory only (the actual app
lives under `frontend/web/app/`). See the repository root
[AGENTS.md](../AGENTS.md) for cross-component architecture and
conventions shared by all modules.

## Commands

To run the frontend together with the backend, pipeline, and database in
one go instead of setting each up by hand, use `compose.local.yml` at the
repo root (`docker compose -f compose.local.yml up -d --build`) — it runs
the frontend as a Vite dev server with hot reload too, see the root
[README.md](../README.md#lokal-testen-docker-compose). Prefer this over a
bare local `npm run build` to verify a UI change actually works end to end
against a real backend/DB — it also sidesteps a local `dist/` permission
trap (see below). The commands below are for running the frontend
standalone.

All commands run from `frontend/web/app/`:

```bash
nvm use && npm install   # Node version pinned in .nvmrc

npm run dev       # Vite dev server, default http://localhost:5173
npm run build     # type-check (vue-tsc -b) then production build to dist/
npm run preview   # serve the built dist/ locally
npm run test:e2e  # Playwright smoke test (e2e/) against a running instance
```

There is no lint or unit-test script. `npm run build` is the only static
verification step (fails on TypeScript errors via `vue-tsc -b`). The
Playwright E2E test (`e2e/smoke.spec.ts`, read-only, no mocks) runs against
an already deployed instance given by `E2E_BASE_URL` (default
`http://localhost:5174`, i.e. `compose.local.yml`); CI runs it after every
deployment against production (set `E2E_EXPECT_DATA=false` to skip the
"region list has entries" check on a server that has no data yet). First
local run: `npx playwright install chromium`. If a
previous production-image build (e.g. a plain `docker build` of
`frontend/web/app/`, run as root, outside of `compose.local.yml`) ever
wrote into this `dist/`, a later local `npm run build` can fail with
`EACCES` when Vite tries to clear that root-owned directory — the
`compose.local.yml` frontend service itself never hits this, since it only
runs `npm run dev` (no `dist/` output) into a container-local named
volume, not a bind-mounted build output. Fix by removing/chowning the
stale `dist/` (e.g. `sudo rm -rf dist` or `sudo chown -R $USER dist`)
rather than working around it another way.

The dev server needs the Go backend running separately (see
[backend/AGENTS.md](../backend/AGENTS.md)); the API base URL comes from
the Vite env var `VITE_API_BASE_URL` (default in `.env.development`:
`http://localhost:8081/api/v1`). For a different local backend port, add a
gitignored `.env.local`. In production the frontend is served from the
same origin as the API (behind Traefik), so `VITE_API_BASE_URL` is unset
there and the client falls back to the relative path `/api/v1`.

## Architecture

Vue 3 + TypeScript SPA (Vite). Routing is handled by **Vue Router**
(`src/router.ts`) — the only additional dependency beyond Vue/Leaflet; there
is still no separate state-management library (Pinia etc.), because the
route (path + query params) *is* the shared cross-view state. `App.vue`
renders a `<RouterView>` wrapped in `<KeepAlive>` (instead of the old
`v-show`-based `activeView` tab switch) so each of the three top-level views
below keeps its loaded data/selection when the user switches away and back,
same as before. Routes deliberately mirror the backend's URI shape (see
`backend/internal/routes/routes.go`) so frontend and API paths stay easy to
compare, and every view/sub-state is deep-linkable/bookmarkable (FAIR
Findability, requirement F12.1):

- `/regionen/:regionType(nuts|lau|agentur)?/:regionId?` — selected region
  type + region, e.g. `/regionen/nuts/DE300`; `?dataset=<slug>` optionally
  preselects a dataset once a region is picked (see below).
- `/karte?geo=&nutsLevel=&dataset=&year=&nutsVersion=&lauVersion=&metric=&month=&region=`
  — the map's full selection state lives in the query string; `region=`
  optionally highlights and centers on one region, `nutsLevel=` (0-3,
  default 3/Kreise) picks which NUTS level is the interactive layer when
  `geo=nuts`.
- `/eigene-region` — "Eigene Region": paste/upload a GeoJSON
  Polygon/MultiPolygon, or combine existing regions by clicking them
  (`components/CombineRegionsPicker.vue`: own Leaflet map, levels
  NUTS-1/NUTS-3/Agentur/LAU (LAU only per chosen Kreis, because of size),
  selection persists across level switches and is emitted as one
  MultiPolygon - overlaps are unioned by the backend), and query datasets
  aggregated over it. Results reuse the Regionen layout: `DatasetPicker.vue`
  with `kind="custom"` (+ `geometry` prop, per-slug `POST`, notes via
  `CustomRegionNote.vue`; multi-indicator datasets appear as `slug::indicator`,
  monthly Agentur series render via the period chart/table)
  (`components/CustomRegionView.vue`, `POST /custom-regions/dataset-aggregate`
  via `apiPost` in `api/client.ts`). `customRegion.ts` does the client-side
  GeoJSON precheck; the region is never stored server-side (only the last
  GeoJSON in localStorage as a convenience). Not part of the query-string
  state model of the other views.
- `/katalog/:slug?` — optionally deep-links to and highlights one catalog
  entry; `?q=&level=&license=` mirror the search/filter state.

The three views are cross-linked (each direction implemented as a
`router.push`/`RouterLink` navigation, not ad-hoc props/refs):

- **Datenkatalog → Regionen**: a catalog card's "In Regionen anzeigen" link
  jumps to the right `regionType` and arms `?dataset=<slug>`; `RegionBrowser`
  shows a hint and applies it as the `DatasetPicker`'s `initial-slug` once a
  region is picked (generalizes the older `jumpToInkarArbeitslose`/
  `initialSlug` pattern, which still exists for the Agenturbezirk → INKAR
  "Siehe auch" link).
- **Karte → Datenkatalog**: the map's dataset/metric picker links to the
  matching `/katalog/:slug` entry.
- **Regionen → Karte**: a region's detail panel has an "Auf der Karte
  anzeigen" button that navigates to `/karte?geo=...&region=...`; `MapView`
  highlights and fits bounds to that region (`focusOnRegion()`). Leaflet's
  animated `fitBounds()` (`_tryAnimatedZoom`) can silently no-op a follow-up
  call while a previous one is still transitioning, so all programmatic
  `fitBounds()` calls in `MapView.vue` pass `animate: false`.

- **Regionen** → `components/RegionBrowser.vue` — a single combined,
  searchable region list covering NUTS, LAU and Agenturbezirke, filterable
  by type via toggle chips (`activeTypes`, `?types=`) instead of separate
  top-level tabs; all three types share one `components/RegionList.vue`
  instance and one search input (`combinedSearch`). The *detail* panel
  below still branches per type (`regionType`, set by whichever
  `selectNuts`/`selectLau`/`selectAgentur` was last called, mirrored to
  `?type=`) since the three types' data shapes genuinely differ - NUTS-3
  and LAU browse yearly socioeconomic time series
  (`components/TimeseriesChart.vue`, `components/DataTable.vue`,
  `components/DatasetPicker.vue`), while Agentur shows a monthly series
  via `periodPoints`/`periodRows` props (index-based x-axis, no
  interpolation status, unlike the year-based `points` prop used by
  NUTS/LAU). The Agentur panel also shows a breadcrumb up its NUTS parent
  chain, and NUTS-1/2/3 panels list contained/partially overlapping
  Agenturbezirke (`GET /agenturbezirke/{ba_schl}/nuts-hierarchy`,
  `GET /nuts/{nuts_id}/agenturbezirke`). Plus an "Aggregierte Kreis-Daten" section
  (`DatasetPicker` with `kind="agentur"`, reading `GET /agenturbezirke/
  {ba_schl}/dataset-aggregate/{slug}`, Summe bzw.
  bevölkerungsgewichteter Durchschnitt aus `mart_agentur_dataset_aggregate`,
  Näherungen werden per `AgenturAggregationNote.vue` gekennzeichnet; ohne
  gewählten Datensatz zeigt das Diagramm die echten BA-Werte, mit Datensatz
  automatisch das kombinierte, indexierte Diagramm; die Karte kann die
  Aggregate ebenfalls einfärben) analogous to NUTS-0/1/2's
  `dataset-aggregate` and LAU's Kreis-linked dataset catalog.
  The route is `/regionen/:regionId?` (no more `:regionType` path
  segment - `regionId` formats are disjoint across NUTS/LAU/Agentur, so a
  single path param plus explicit `?type=` disambiguates); a lightweight
  client-side `inferRegionType()` only kicks in as a fallback for deep
  links without `?type=`.
  The NUTS portion of the list additionally has a level sub-switcher
  (Land/Bundesland/Regierungsbezirk/Kreis = NUTS 0-3, `nutsBrowseLevel`,
  mirrored to `?level=`, only shown while "nuts" is an active type filter);
  breadcrumb (ancestor chain) and, for NUTS 0-2, a child-region list are
  derived purely client-side from one unfiltered `GET /nuts` call (NUTS
  codes are prefix-hierarchical: `DE` → `DE3` → `DE30` → `DE300`), no
  dedicated backend endpoint needed. A Kreis (NUTS-3) detail additionally
  shows its member Gemeinden (`GET /lau?nuts_parent_id=`) and NUTS-3-level
  LAU rollups (`GET /nuts/{id}/lau-aggregate`, backed by
  `mart_nuts3_lau_aggregate` in dbt) as a cross-check against the native
  Kreis-level population. "Auf der Karte anzeigen" is shown for every NUTS
  level (`viewOnMap` passes the region's `nuts_level` as `?nutsLevel=`, see
  below) as well as LAU.
- **Karte** → `components/MapView.vue` — Leaflet map colored by a
  selected metric's (interpolated) values per region, using `mapColor.ts`
  for the color scale. A level switcher (`selectedNutsLevel`, mirrored to
  `?nutsLevel=`) lets the NUTS geo type show any of NUTS 0-3 as the
  interactive layer (`GET /geo?geo_type=nuts&level=`), not just Kreise.
  Kreise (NUTS-3) and LAU color via the bulk `GET /{slug}/timeseries?year=`;
  NUTS-0/1/2 have no bulk-over-a-level counterpart, only the per-region `GET
  /nuts/{nuts_id}/dataset-aggregate/{slug}` (all years in one response, no
  `year` filter), so `usesNutsAggregate` (`MapView.vue`) switches
  `loadValues()` to fan out one parallel request per currently visible
  region instead (1/~16/~30 requests for Land/Bundesland/
  Regierungsbezirk - cheap enough not to need a bulk endpoint) and picks the
  selected year client-side; the dataset catalog for that path comes from a
  single `GET /nuts/DE/datasets` call (DE is a superset of every
  lower-level dataset, see `regionNutsDatasets()` in
  `backend/internal/handlers/datasets.go`), not the Kreis-level
  `/socioeconomic/inventory` used for NUTS-3/LAU. Agenturbezirk map
  coloring is fully implemented too (own code path: `GET
  /agenturbezirke/periods` + `/agenturbezirke/period` for geometry+values
  by chosen month, since Agentur has a monthly rather than yearly time
  axis; `styleForFeature`/`colorForValue`/`MapRegion` are geo_type-agnostic
  and need no Agentur-specific branching for the coloring itself).
- **Datenkatalog** → `components/DatasetCatalog.vue` — lists `GET
  /datasets`, with free-text search, Ebenen-/Lizenz-Filter, CSV export, a
  raw-data link, a per-entry download section (full dataset as CSV/JSON plus
  curl/link snippets, see `ApiSnippet.vue` above, plus a "Metadaten mit
  exportieren" checkbox - see `download.ts` below) and (see cross-links
  above) a permalink/highlight per entry and a link into the matching
  Regionen view.

Cross-cutting pieces:

- `src/api/client.ts` — the only place that talks to the backend
  (`apiGet`). All API responses are wrapped (`{success, data, error}`);
  `apiGet` throws on `success === false` or a non-2xx status, so callers
  just `await` and catch. `unwrapNull` exists because the Go backend
  serializes some nullable LAU fields as `database/sql.Null*` structs
  (`{"Int64":1,"Valid":true}`) instead of plain scalars/`null` — always
  route those fields through it rather than accessing them directly.
- `src/types/api.ts` — hand-written TypeScript types mirroring the Go API
  response shapes (there is no shared schema/codegen between backend and
  frontend; if a Go model or generated handler changes shape, update this
  file by hand).
- `src/api/download.ts` — builds the download/API URLs (it never fetches):
  `absoluteApiUrl` resolves the possibly relative `API_BASE` against the
  current origin so copied links and curl commands work outside the page,
  `regionDownloadUrl`/`regionApiUrl` target the combined export
  `GET /regions/{type}/{id}/data`. Downloads run as a plain `<a href>`
  against the API (streamed, no in-memory Blob), so the CSV is produced by
  the backend, not here — the only client-side CSV left is the catalog's
  own metadata export in `DatasetCatalog.vue`. `datasetMetadataUrl`/
  `datasetMetadataApiUrl` target `GET /datasets` (or `GET /datasets/{slug}`
  for exactly one slug) to export the provenance/license/description of the
  datasets included in a data download as a *separate* file at the same
  time - CSV can't mix the two row shapes into one file, so this stays two
  coordinated downloads rather than one combined file/response.
  `datasetSourceUrl`/`datasetSourceApiUrl` target `GET /datasets/{slug}/source`
  (unharmonized source values as delivered by geo-mapper); the download
  lives only in the Datenkatalog card (`DatasetCatalog.vue`), gated by
  `hasSourceData(slug)` in `constants.ts` (derived slugs and the Agentur
  dataset have none). `DownloadDialog.vue` just links there.
  `triggerDownloads(urls)` fires one `<a download>` per URL, staggered by
  300ms: Chromium aborts an in-flight `<a download>` click if a second one
  fires in the same tick (observed while building this - only the later of
  two synchronously-clicked links actually arrived), so the URLs can't just
  be clicked in a plain loop.
- `components/DownloadDialog.vue` — per-region download dialog (datasets,
  scope raw/aggregated/both, format, year range), preselected from what the
  panel currently shows; rendered once in `RegionBrowser.vue` and fed by its
  `downloadTarget` computed, which also encodes that NUTS-0/1/2 have no raw
  values and LAU no aggregated ones. A "Metadaten ... separat mit
  exportieren" checkbox (default on) additionally downloads the matching
  `GET /datasets` rows via `triggerDownloads`; resolves "alle verfügbaren
  Datensätze" (empty selection) to the concrete slug list first, since
  `GET /datasets` has no such "all" shorthand of its own.
- `components/ApiSnippet.vue` — shows the same request as a curl command
  (format via `Accept`) and a link (format via `?format=`), with copy
  buttons; used by the download dialog and by each catalog card for the
  full dataset.
- `src/format.ts` — shared number/date formatting helpers used across
  components.
- `src/constants.ts` — small set of values shared across otherwise
  independent components (currently just the Agenturbezirk dataset slug,
  needed by `RegionBrowser.vue`, `DatasetCatalog.vue`, and `MapView.vue` for
  the catalog cross-link).

`frontend/web/README.md` and `frontend/web/static/`,
`frontend/web/template/` are leftover from an earlier server-templated
approach; the current app is entirely the Vite SPA in `app/`.
