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

### Verzeichnisstruktur

```
1_friedrich-bjoern/
│
├── <dataset>/                              # z. B. bevoelkerung, schuldnerquote, arbeitslose, ...
│   ├── <dataset>_kreise.csv                # Rohexport (Eingabe des geo-mapper, nicht von der Pipeline gelesen)
│   ├── <dataset>_kreise_meta.yaml          # Mapper-Konfiguration (ebenfalls nicht von der Pipeline gelesen)
│   └── results_<dataset>_kreise/           # Ausgabe des geo-mapper = Eingabe der Pipeline
│       ├── mapped_pairs.csv                # gemappte Werte je Region und Jahr
│       ├── meta.yaml                       # Metadaten des Datensatzes
│       └── unmapped*.csv                   # nicht zuordenbare Regionen (nur zur Kontrolle)
│
├── sv_beschaeftigte/results_sv_beschaeftigte_kreise/   # wird von fetch_ba_sv_beschaeftigte.py erzeugt
├── beschaeftigungsquote/results_beschaeftigungsquote_gemeinden/  # Gemeinde-Ebene, von run_all_pipelines.sh nicht geladen
│
├── erwerbsfaehig/, erwerbstaetige/         # nur Rohdateien ("Tabelle Abfrage1.csv"), von der Pipeline nicht gelesen
│
└── Geodaten/                               # bevorzugte Geodatenquelle für load_geodata_to_postgis.py
    ├── geojson/LAU/<jahr>/lau_<jahr>_level_0.geojson
    ├── geojson/NUTS_<0-3>/<jahr>/nuts_<jahr>_level_<0-3>.geojson
    └── csv/...                             # Namens-/ID-Tabellen zu den Geometrien
```

Die Pipeline liest nur `results_<dataset>_kreise/{mapped_pairs.csv,meta.yaml}`
(über `pipeline/dlt/load_soziodata_generic.py <dataset>/results_<dataset>_kreise`, die Liste der
geladenen Datensätze steht in `pipeline/dlt/run_all_pipelines.sh`) sowie den Ordner `Geodaten/`.
Fehlt eine der beiden Dateien, bricht der Loader mit Fehler ab. Welche Datensätze tatsächlich vorhanden sind,
hängt vom Nextcloud-Stand ab; im Git-Checkout sind alle Daten ausgeschlossen.

Weitere Ordner neben `1_friedrich-bjoern/` (`eurostat_lau_population/`, `geodata/`, `geo-mapper-output/`) sind
lokale Arbeits-/Download-Verzeichnisse. `eurostat_lau_population/` ist das Ziel von `fetch_lau_population.py`
(`GEODATA_BASE_PATH`, Dateien `statistics/lokale_verwaltungseinheiten_eu/output/lau_population_<jahr>.csv`).

### Dateiformat-Details

| Datei | Format | Beschreibung |
|---|---|---|
| `mapped_pairs.csv` | CSV (`,`) | Spalten `original_id_1`, `original_name`, `geodata_name`, `geodata_id_nuts`, `geodata_id_ars`, `mapper`, `parameter`, danach je Jahr eine Wertespalte |
| `meta.yaml` | YAML | `geodata_level` (z. B. `NUTS_3`), `geodata_year`, `number_type` (`absolute`/`relative`), `name`, `description`, `description_long`, `unit`, `source`, `algorithm`; optional `license`/`license_url` (Override, sonst automatisch für BBSR-Quellen). Zusätzlich enthält die Datei die Mapper-Konfiguration (`id_columns`, `name_column`, `value_columns`, `value_rename`), die die Pipeline ignoriert |
| `Geodaten/geojson/.../*.geojson` | GeoJSON | Geometrien je Ebene und Jahr; Dateiname muss dem Muster `lau_<jahr>_level_<n>.geojson` bzw. `nuts_<jahr>_level_<n>.geojson` folgen |
| `lau_population_<jahr>.csv` | CSV (`;`) | LAU-Bevölkerung je Jahr, erzeugt von `fetch_lau_population.py` |

Details zu den Loadern: [docs/pipeline/dlt.md](../docs/pipeline/dlt.md).

---

## Automatischer Nextcloud-Sync (exaktes Spiegeln)

Für einen regelmäßigen Sync inklusive Löschungen gibt es ein minimales Skript auf Basis von `rclone sync`:

```bash
./backend/scripts/sync_nextcloud_data_exchange.sh
```

1. rclone konfigurieren
    - `sudo apt-get install rclone`
    - `rclone config`
    - Lege ein Remote für Nextcloud an, z. B. `HTW-FP-Data` (für Public-Share) oder `HTW-FP-Auth` (für Benutzerkonto).
    - **Für öffentlichen Share-Link:**
        - Typ: `webdav`
        - URL: `https://cloud.htw-berlin.de/public.php/dav/files/<SHARE_TOKEN>` (beachte: `public.php`, nicht `remote.php`!)
        - Vendor: `nextcloud`
        - User: `<SHARE_TOKEN>`
        - Passwort: Share-Passwort, falls gesetzt, sonst leer lassen.
    - **Für authentifizierten Zugriff (Nextcloud-Benutzerkonto):**
        - Typ: `webdav`
        - URL: `https://cloud.htw-berlin.de/remote.php/dav/files/<BENUTZERNAME>`
        - Vendor: `nextcloud`
        - User: `<BENUTZERNAME>`
        - Passwort: App-Passwort oder Benutzerpasswort
    - **Wichtig:** Überprüfe die Ordnerstruktur im Remote mit `rclone lsd <REMOTE_NAME>:` — nutze die exakten Ordnernamen, wie sie dort angezeigt werden (z. B. `1_friedrich_bjoern` mit Unterstrichen).

2. Sync-Konfiguration anlegen
    - `cp backend/deployments/.env.nextcloud-sync.example backend/deployments/.env.nextcloud-sync`
    - Pflichtvariable:
        - `RCLONE_SOURCE`: Remote-Name und Ordnerpfad, z. B. `HTW-FP-Data:1_friedrich_bjoern` (exakte Schreibweise beachten!)
        - Falls der Share nur einen Ordner enthält: `HTW-FP-Data:` (ohne Pfad)
    - Wichtige optionale Variablen:
        - `RCLONE_DEST` (Default ist `<repo>/data-exchange/1_friedrich-bjoern`; bei root liegt das Repo oft unter `/root/...`, nicht unter `/home/root/...`)
        - `RCLONE_CONFIG_FILE` (Default ist `~/.config/rclone/rclone.conf`)
    - Das Sync-Skript, der `systemd`-Service und der Timer müssen nicht angepasst werden.

3. Testlauf ausführen
    - `./backend/scripts/sync_nextcloud_data_exchange.sh`

4. Alle 10 Minuten automatisch ausführen (systemd User Timer)
    - `mkdir -p ~/.config/systemd/user`
    - `cp backend/init/systemd/nextcloud-data-exchange-sync.service ~/.config/systemd/user/`
    - `cp backend/init/systemd/nextcloud-data-exchange-sync.timer ~/.config/systemd/user/`
    - *Hinweis:* `cp` überschreibt vorhandene Dateien standardmäßig (falls kein `cp -i` Alias aktiv ist).
    - Prüfe den Skriptpfad: Standard ist `~/soziooekonomische-daten/backend/scripts/sync_nextcloud_data_exchange.sh`:
    - Wenn dein Repo woanders liegt, lege die Env-Datei an (sonst endet der Service oft mit `status=127`).
    - `cat > ~/.config/systemd/user/nextcloud-data-exchange-sync.env <<'EOF'`
    - `SYNC_SCRIPT_PATH=/dein/pfad/zum/repo/backend/scripts/sync_nextcloud_data_exchange.sh`
    - `EOF`
    - Beispiel für root auf dem Server: `SYNC_SCRIPT_PATH=/root/soziooekonomische-daten/backend/scripts/sync_nextcloud_data_exchange.sh`
    - `systemctl --user daemon-reload`
    - `systemctl --user enable --now nextcloud-data-exchange-sync.timer`
    - `systemctl --user list-timers | grep nextcloud-data-exchange-sync`
    - Logs ansehen:
        - `systemctl --user status nextcloud-data-exchange-sync.service --no-pager`
        - `journalctl --user -u nextcloud-data-exchange-sync.service -n 100 --no-pager`
    - Optional, falls der User-Timer auch ohne Login laufen soll (möchtest du i.d.R., weil der Timer mit dem letzten Logout stoppt):
        - `loginctl enable-linger "$USER"`

5. Bei Änderungen aktualisieren (z. B. neues Intervall im Timer)

    ```bash
        cp backend/init/systemd/nextcloud-data-exchange-sync.service ~/.config/systemd/user/
        cp backend/init/systemd/nextcloud-data-exchange-sync.timer ~/.config/systemd/user/
        systemctl --user daemon-reload
        systemctl --user reset-failed nextcloud-data-exchange-sync.service
        systemctl --user restart nextcloud-data-exchange-sync.timer
        systemctl --user start nextcloud-data-exchange-sync.service
        systemctl --user cat nextcloud-data-exchange-sync.timer | grep OnCalendar
    ```
