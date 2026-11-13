# `/githooks`

Versionierte Git-Hooks für das Backend.

## Installation

Damit Git die Hooks in diesem Verzeichnis automatisch verwendet, einmalig im Repository-Root ausführen:

```bash
git config core.hooksPath backend/githooks
```

## Hooks

### `pre-commit`

Wird vor jedem Commit ausgeführt und prüft/generiert folgendes:

1. **Swagger-Dokumentation** – führt `swag init -g cmd/soziodata/main.go -o ./docs --parseDependency --parseInternal` aus (schlägt das fehl, bricht der Commit ab; es gibt keinen Lauf ohne die Flags, weil swag v1.16.4 sonst alle Definitionen umbenennt. Ausweg: swag beheben oder mit `--no-verify` committen und das in der Commit-Message nennen) und staged `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`.
2. **Go-Models aus dbt-Schemas** – wenn sich Dateien nach dem Muster `pipeline/dbt/.../staging/stg_*_kreise_schema.yml` im Staging-Bereich befinden, wird `go generate ./internal/models/...` ausgeführt. Die drei generierten Dateien werden anschließend automatisch gestaged:
   - `internal/models/models_soziodata_generated.go`
   - `internal/handlers/handlers_soziodata_generated.go`
   - `internal/routes/routes_soziodata_generated.go`
3. **Go-Tests** – führt `go test ./...` aus. Schlägt ein Test fehl, wird der Commit abgebrochen.

### Tipps

| Zweck | Befehl |
|---|---|
| Hook temporär überspringen | `git commit --no-verify` |
| Hook manuell ausführen | `bash backend/githooks/pre-commit` |
