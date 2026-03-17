# Data Exchange

Dieser Ordner dient dem Datenaustausch im Produktivsystem.  
Die Verzeichnisstruktur liegt im Git — alle Datendateien sind via `.gitignore` ausgeschlossen und dürfen **nicht** eingecheckt werden.

---

## 0_toan-friedrich

Datenaustausch von **Toan → Friedrich**.

---

## 1_friedrich-bjoern

Datenaustausch von **Friedrich → Björn**.

Dieser Ordner wird vom `pipeline`-Container als Datenquelle für die dlt-Pipelines gemountet (`DATA_EXCHANGE_PATH=/app/data-exchange/1_friedrich-bjoern`).

### Erwartete Verzeichnisstruktur

```
1_friedrich-bjoern/
│
├── arbeitslosenzahlen/
│   ├── mapped_pairs.csv          # Arbeitslosenzahlen (gemappte Regionen)
│   └── meta.json                 # Metadaten zum Arbeitslosenzahlen-Datensatz
│
├── geodata/
│   └── lau/
│       └── output/
│           └── geojson/
│               ├── LAU_RG_01M_2011_3035_cleaned.geojson.gz
│               ├── LAU_RG_01M_2012_3035_cleaned.geojson.gz
│               └── ...           # je Jahr eine Datei
│
└── statistics/
    └── lokale_verwaltungseinheiten_eu/
        └── output/
            ├── lau_population_2011.csv
            ├── lau_population_2012.csv
            └── ...               # je Jahr eine Datei
```

### Dateiformat-Details

| Datei | Format | Trennzeichen | Beschreibung |
|---|---|---|---|
| `mapped_pairs.csv` | CSV | `,` | Arbeitslosenzahlen mit gemappten Geodata-IDs — liegt in `arbeitslosenzahlen/` |
| `meta.json` | JSON | — | Metadaten (title, series, publisher, time_resolution, …) — liegt in `arbeitslosenzahlen/` |
| `LAU_RG_01M_{YEAR}_3035_cleaned.geojson.gz` | GeoJSON (gzip) | — | LAU-Geometrien in EPSG:3035, bereinigt |
| `lau_population_{YEAR}.csv` | CSV | `;` | LAU-Bevölkerungsdaten je Jahr |

### Namenskonvention

- GeoJSON-Dateien: `LAU_RG_01M_{YYYY}_3035_cleaned.geojson.gz` — das Jahr muss an Position 4 (0-indexed) im `_`-getrennten Dateinamen stehen.
- Bevölkerungs-CSVs: `lau_population_{YYYY}.csv` — das Jahr steht im Dateinamen-Stem nach dem letzten `_`.

---

## Automatischer Nextcloud-Sync (exaktes Spiegeln)

Für einen regelmäßigen Sync inklusive Löschungen gibt es ein minimales Skript auf Basis von `rclone sync`:

```bash
./backend/scripts/sync_nextcloud_data_exchange.sh
```

1. rclone konfigurieren
     - `rclone config`
     - Lege ein Remote fuer Nextcloud an, z. B. `NC-HTW-FP`.
     - Nutze danach einen Quellpfad wie `NC-HTW-FP:1_friedrich-bjoern`.

2. Sync-Konfiguration anlegen
     - `cp backend/deployments/.env.nextcloud-sync.example backend/deployments/.env.nextcloud-sync`
     - Pflichtvariable:
         - `RCLONE_SOURCE`, z. B. `NC-HTW-FP:1_friedrich-bjoern`
     - Wichtige optionale Variablen:
         - `RCLONE_DEST` (Default ist `<repo>/data-exchange/1_friedrich-bjoern`)
         - `RCLONE_CONFIG_FILE`

3. Testlauf ausführen
     - `./backend/scripts/sync_nextcloud_data_exchange.sh`

4. Alle 30 Minuten automatisch ausführen (systemd User Timer)
     - `mkdir -p ~/.config/systemd/user`
     - `cp backend/init/systemd/nextcloud-data-exchange-sync.service ~/.config/systemd/user/`
     - `cp backend/init/systemd/nextcloud-data-exchange-sync.timer ~/.config/systemd/user/`
     - `systemctl --user daemon-reload`
     - `systemctl --user enable --now nextcloud-data-exchange-sync.timer`
     - `systemctl --user list-timers | grep nextcloud-data-exchange-sync`
     - Wenn dein Repo an einem anderen Ort liegt, lege optional `~/.config/systemd/user/nextcloud-data-exchange-sync.env` an.
     - Inhalt der optionalen Env-Datei:
         - `SYNC_SCRIPT_PATH=/dein/pfad/zum/repo/backend/scripts/sync_nextcloud_data_exchange.sh`
     - Logs ansehen:
         - `journalctl --user -u nextcloud-data-exchange-sync.service -n 100 --no-pager`
     - Optional, falls der User-Timer auch ohne Login laufen soll:
         - `loginctl enable-linger "$USER"`
