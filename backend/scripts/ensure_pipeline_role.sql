-- Idempotent: legt die Schreib-Rolle der Pipeline (dlt + dbt) an bzw. zieht Rechte/Passwort/Owner nach.
-- Aufruf als Superuser (POSTGRES_USER), NACH ensure_api_role.sql (deploy_remote.sh apply macht beides);
-- Passwort per Umgebung (docker exec -e NAME ohne Wert -> nicht in argv/Prozessliste):
--   docker exec -i -e PIPELINE_DB_PASSWORD postgis psql -q -U sozio_data_compose_user -d soziooekonomische-daten < ensure_pipeline_role.sql
-- Neue Schemas der Pipeline gehoeren HIERHER (CREATE SCHEMA ... AUTHORIZATION pipeline_rw + Owner-Liste
-- unten), nicht in dlt/dbt - pipeline_rw darf keine Schemas anlegen.
-- Die Rolle besitzt die Schemas loaded_data, loaded_data_staging, transformed_data, snapshots und
-- alles darin (dbt/dlt brauchen Owner-Rechte fuer DROP/ALTER/RENAME). Sie hat KEIN CREATE auf der
-- Datenbank: Schemas und Extensions legt dieses Skript als Superuser an.
\set ON_ERROR_STOP on
\getenv pipeline_pw PIPELINE_DB_PASSWORD
\if :{?pipeline_pw}
\else
  DO $$BEGIN RAISE EXCEPTION 'PIPELINE_DB_PASSWORD fehlt (docker exec -e PIPELINE_DB_PASSWORD ...)'; END$$;
\endif

-- 1) Rolle (cluster-global)
SELECT 'CREATE ROLE pipeline_rw' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pipeline_rw') \gexec
ALTER ROLE pipeline_rw WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 10;
ALTER ROLE pipeline_rw PASSWORD :'pipeline_pw';
-- Grosse Spatial-Joins (int_lau_spatial_crosswalk) brauchen lange; Timeout nur gegen Endlos-Laeufe.
ALTER ROLE pipeline_rw SET statement_timeout = '2h';
ALTER ROLE pipeline_rw SET idle_in_transaction_session_timeout = '30min';
-- dbt-Tabellentausch (RENAME) braucht ACCESS EXCLUSIVE; lieber Lauf scheitern lassen als API lange blockieren
ALTER ROLE pipeline_rw SET lock_timeout = '5min';

-- 2) Datenbank: nur CONNECT + TEMP (dbt-Snapshots legen temporaere Tabellen an), kein CREATE
SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO pipeline_rw', current_database()) \gexec
SELECT format('REVOKE CREATE ON DATABASE %I FROM pipeline_rw', current_database()) \gexec

-- 3) Extensions als Superuser (dbt-Post-Hook "create extension if not exists pg_trgm" ist dann No-op)
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public;

-- 4) Schemas: gehoeren pipeline_rw (loaded_data_staging = dlt-Staging fuer merge auf dataset_metadata)
CREATE SCHEMA IF NOT EXISTS loaded_data         AUTHORIZATION pipeline_rw;
CREATE SCHEMA IF NOT EXISTS loaded_data_staging AUTHORIZATION pipeline_rw;
CREATE SCHEMA IF NOT EXISTS transformed_data    AUTHORIZATION pipeline_rw;
CREATE SCHEMA IF NOT EXISTS snapshots           AUTHORIZATION pipeline_rw;
ALTER SCHEMA loaded_data         OWNER TO pipeline_rw;
ALTER SCHEMA loaded_data_staging OWNER TO pipeline_rw;
ALTER SCHEMA transformed_data    OWNER TO pipeline_rw;
ALTER SCHEMA snapshots           OWNER TO pipeline_rw;
REVOKE ALL ON SCHEMA loaded_data, loaded_data_staging, snapshots FROM PUBLIC;

-- 5) Migration/Selbstheilung: alle Relationen (Tabellen, Views, MatViews, Sequenzen, Fremdtabellen)
--    in diesen Schemas pipeline_rw uebereignen. Owner-Wechsel ist reine Katalog-Aenderung (keine
--    Datenkopie), braucht aber kurz ACCESS EXCLUSIVE -> lock_timeout, damit API-Reads nicht lange
--    hinter einer wartenden Sperre haengen. Indizes/Toast/Row-Types folgen ihrer Tabelle.
SET lock_timeout = '30s';
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT n.nspname, c.relname, c.relkind
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname IN ('loaded_data', 'loaded_data_staging', 'transformed_data', 'snapshots')
      AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
      AND c.relowner <> 'pipeline_rw'::regrole
      -- Sequenzen, die einer Tabellenspalte gehoeren, wandern mit der Tabelle
      AND NOT (c.relkind = 'S' AND EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = c.oid AND d.deptype IN ('a', 'i')))
    ORDER BY c.relkind = 'v' DESC  -- Reihenfolge egal, Views zuerst nur fuer lesbare Logs
  LOOP
    EXECUTE format('ALTER %s %I.%I OWNER TO pipeline_rw',
      CASE r.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' WHEN 'S' THEN 'SEQUENCE'
                     WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END,
      r.nspname, r.relname);
    RAISE NOTICE 'Owner -> pipeline_rw: %.%', r.nspname, r.relname;
  END LOOP;
END$$;
RESET lock_timeout;

-- 6) api_ro: kuenftige dbt-Objekte entstehen jetzt als pipeline_rw -> Default Privileges FOR ROLE pipeline_rw
--    (die alten FOR ROLE <superuser>-Eintraege bleiben harmlos bestehen). Bestehende Relationen nachziehen.
SELECT 'ALTER DEFAULT PRIVILEGES FOR ROLE pipeline_rw IN SCHEMA transformed_data GRANT SELECT ON TABLES TO api_ro'
  WHERE EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'api_ro') \gexec
SELECT 'GRANT SELECT ON ALL TABLES IN SCHEMA transformed_data TO api_ro'
  WHERE EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'api_ro') \gexec
