# Infrastruktur (Hetzner Cloud, Terraform)

| Verzeichnis | Zweck | State |
|---|---|---|
| `bootstrap/` | Legt **einmalig** die privaten Buckets in Hetzner Object Storage an: State-Bucket und Backup-Bucket | lokal (nicht im Repo) |
| `hetzner/` | Server, Firewall, Volume für die PostgreSQL-Daten, cloud-init | Bucket aus `bootstrap/` |

Ausgerollt wird `hetzner/` durch [`.github/workflows/infra.yml`](../.github/workflows/infra.yml)
(Push nach `main` bzw. manuell, `apply` hinter dem GitHub-Environment `infra`).
Die Anwendung selbst deployt [`.github/workflows/main.yml`](../.github/workflows/main.yml).

## Erst-Einrichtung (Reihenfolge)

1. **Hetzner-Projekt + API-Token** (Read & Write) in der Cloud-Konsole anlegen.
2. **S3-Access-Key** für Object Storage in der Konsole erzeugen (per API/Terraform nicht möglich).
3. **Buckets anlegen** (State + Backup, einmalig vom Laptop, lokaler State):
   ```bash
   export AWS_ACCESS_KEY_ID=...        # S3-Key aus Schritt 2
   export AWS_SECRET_ACCESS_KEY=...
   cd infra/bootstrap
   terraform init && terraform apply   # Variablen: -var bucket=... -var region=fsn1
   ```
   Der lokale State (`terraform.tfstate`) ist nicht im Repo (`.gitignore`); geht er verloren, lässt sich der Bucket
   per `terraform import aws_s3_bucket.state <bucket>` wieder aufnehmen.
4. **Deploy-Key erzeugen**: `ssh-keygen -t ed25519 -f deploy_key -C soziodata-deploy` (ohne Passphrase; `deploy_key` → Secret `SSH_PRIVATE_KEY`, `deploy_key.pub` → Secret `DEPLOY_SSH_PUBLIC_KEY`).
5. **GitHub Secrets/Variablen** setzen (siehe unten).
6. **Environments** `infra` und `production` anlegen (empfohlen: Required reviewers).
7. **`infra.yml` ausführen** (Actions → „Infrastruktur (Terraform)“ → Run workflow, danach `apply` freigeben).
   Die Zusammenfassung des Laufs zeigt die IPv4/IPv6 des Servers.
8. **DNS setzen**: A-Record (und optional AAAA) für `APP_HOST` auf die Server-IP.
9. **Repo-Variable `SSH_HOST`** auf die Server-IP setzen und Host-Key holen:
   ```bash
   ssh-keyscan -H <SSH_HOST>   # Ausgabe als Secret SSH_KNOWN_HOSTS speichern
   ```
   Der erste Boot (cloud-init: Docker, rclone, Repo-Checkout, Volume) braucht einige Minuten;
   `ssh deploy@<SSH_HOST> 'cloud-init status --wait'` wartet darauf.
10. **Erster Deploy**: `main.yml` per „Run workflow“ mit **initial_load = true** starten. Das startet nach dem Rollout
    im Hintergrund auf dem Server alle Daten neu zu laden (Nextcloud-Sync → dlt → dbt); der Workflow wartet nicht darauf.
    Fortschritt: `ssh deploy@<SSH_HOST> 'bash /opt/soziooekonomische-daten/backend/scripts/deploy_remote.sh initial-load-status'`. Historie aus früheren dbt-Snapshots ist bei einem Neuaufbau verloren.

## GitHub Secrets und Variablen

| Name | Art | Verwendung |
|---|---|---|
| `HCLOUD_TOKEN` | Secret | Hetzner-Cloud-API (Terraform) |
| `TF_STATE_ACCESS_KEY`, `TF_STATE_SECRET_KEY` | Secret | S3-Key für den State-Bucket |
| `DEPLOY_SSH_PUBLIC_KEY` | Secret | öffentlicher Deploy-Key (landet per cloud-init beim User `deploy`) |
| `SSH_PRIVATE_KEY` | Secret | privater Deploy-Key |
| `SSH_KNOWN_HOSTS` | Secret | Ausgabe von `ssh-keyscan -H <SSH_HOST>` |
| `DB_PASSWORD`, `LE_EMAIL` | Secret | Compose-`.env` auf dem Server |
| `RCLONE_CONF` | Secret | Inhalt der `rclone.conf` für den Nextcloud-Sync (optional) |
| `PAT_SEMANTIC_RELEASE` | Secret | Personal Access Token für Semantic Release (Push des Release-Commits) |
| `TF_STATE_BUCKET`, `TF_STATE_REGION` | Variable | z. B. `soziodata-tfstate`, `fsn1` |
| `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` | Secret | S3-Key für das nächtliche DB-Backup (darf derselbe Key wie oben sein) |
| `BACKUP_S3_BUCKET`, `BACKUP_S3_REGION` | Variable | z. B. `soziodata-backup`, `fsn1` (optional; ohne diese vier Werte wird kein Backup eingerichtet) |
| `SSH_HOST`, `SSH_USER` | Variable | Server-IP, `deploy` |
| `APP_HOST` | Variable | öffentlicher Hostname (DNS!) |
| `RCLONE_SOURCE` | Variable | z. B. `HTW-FP-Data:1_friedrich_bjoern` (optional, zusammen mit `RCLONE_CONF`) |
| `HCLOUD_SERVER_TYPE`, `HCLOUD_LOCATION` | Variable | optional, Default `cpx12` / `nbg1` |
| `PGDATA_DIR` | Variable | optional, Default `/mnt/pgdata/data` (Hetzner-Volume) |

Die GHCR-Images sind öffentlich zu stellen (Package-Einstellungen → Visibility), damit der Server sie ohne Login ziehen kann.

## Datenbank-Backup

Der Deploy-Job richtet für den Deploy-User einen Cron-Job ein (täglich 02:17 Serverzeit, Log `~/backup.log`), sobald die vier
`BACKUP_S3_*`-Werte gesetzt sind. `backend/scripts/backup_db.sh` sichert per `pg_dump` das Schema `snapshots`
(dbt-Historie) und `loaded_data.dataset_metadata` in den Backup-Bucket und löscht Dumps, die älter als 30 Tage sind
(`BACKUP_RETENTION_DAYS`). Alles andere baut die Pipeline aus den Quellen neu. Wiederherstellung: siehe
`docs/operations.md` (Abschnitt „Database backup and restore“).

## Härtung

cloud-init sperrt SSH auf Key-Login des Users `deploy` (kein Root-Login, kein Passwort), richtet `fail2ban` und
automatische Sicherheitsupdates ein. Das wirkt nur bei **neu angelegten** Servern (`user_data` ist per
`ignore_changes` ausgenommen); für nachträgliche Änderungen den Server neu aufbauen (`terraform taint hcloud_server.main`,
die Datenbank bleibt auf dem Volume). Notzugang bei ausgesperrtem SSH (laut Hetzner-Docs): Console → Server → *Rescue* → *Reset Root Password*, danach in der Console (VNC)
als root anmelden; Alternativen: Rescue-System oder Server neu aufbauen. `infra.yml` bricht ab, wenn `SSH_PRIVATE_KEY` und
`DEPLOY_SSH_PUBLIC_KEY` nicht zusammenpassen.
Adminer ist nicht öffentlich, sondern nur per SSH-Tunnel erreichbar, siehe `docs/operations.md`
(Abschnitt „Server hardening and Adminer“).

## Hinweise

- Das Volume `soziodata-pgdata` hat `prevent_destroy`; ein Server-Neuaufbau (`terraform taint hcloud_server.main`) behält die Datenbank.
- SSH steht für die wechselnden IPs der GitHub-Runner offen (Key-Login). Über `ssh_allowed_cidrs` lässt sich das einschränken.
- Port 5432 ist per Firewall zu und im Compose-File nur an `127.0.0.1` gebunden; Zugriff per `ssh -L 5432:localhost:5432 deploy@<SSH_HOST>`.
