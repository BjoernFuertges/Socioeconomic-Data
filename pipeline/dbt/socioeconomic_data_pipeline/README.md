# socioeconomic_data_pipeline (dbt)

dbt-Projekt, das `loaded_data` (dlt) in das Schema `transformed_data` transformiert
(Staging, Intermediate, Marts, Snapshots). Hintergrund und Konventionen:

- [pipeline/dbt/AGENTS.md](../AGENTS.md) – Kommandos, Modell-Schichten, Regeln
- [docs/pipeline/dbt.md](../../../docs/pipeline/dbt.md) – ausführliche Beschreibung der Modelle

## Kernbefehle

Voraussetzung: `profiles.yml` (Vorlage `profiles.example.yml`, bzw. `DBT_PROFILES_DIR`)
und eine erreichbare Postgres/PostGIS-Instanz, in die dlt bereits geladen hat.

```bash
dbt deps       # Pakete installieren
dbt snapshot   # Historisierung (snap_results_*)
dbt run        # Modelle bauen
dbt test       # Tests ausführen
```

Im Betrieb ruft `pipeline/dlt/run_all_pipelines.sh` diese Befehle nach den dlt-Ladeläufen auf.
