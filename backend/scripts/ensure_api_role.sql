-- Idempotent: legt die read-only Login-Rolle des Backends an bzw. zieht Rechte/Passwort nach.
-- Aufruf als Owner/Superuser; das Passwort kommt per Umgebung (docker exec -e NAME ohne Wert
-- erbt es aus der Shell -> nicht in argv/Prozessliste):
--   docker exec -i -e API_DB_PASSWORD postgis psql -q -U sozio_data_compose_user -d soziooekonomische-daten < ensure_api_role.sql
\set ON_ERROR_STOP on
\getenv api_pw API_DB_PASSWORD
\if :{?api_pw}
\else
  DO $$BEGIN RAISE EXCEPTION 'API_DB_PASSWORD fehlt (docker exec -e API_DB_PASSWORD ...)'; END$$;
\endif
SELECT current_user AS owner_role \gset

-- 1) Rolle (cluster-global)
SELECT 'CREATE ROLE api_ro' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'api_ro') \gexec
ALTER ROLE api_ro WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 40;
ALTER ROLE api_ro PASSWORD :'api_pw';
-- Defense in depth: Sessions sind read-only, Timeouts auch ohne DSN-Option
ALTER ROLE api_ro SET default_transaction_read_only = on;
ALTER ROLE api_ro SET statement_timeout = '60s';
ALTER ROLE api_ro SET idle_in_transaction_session_timeout = '60s';
ALTER ROLE api_ro SET search_path = public;

-- 2) Datenbank: PUBLIC verliert CONNECT/TEMP, nur explizite Rollen duerfen rein
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO api_ro', current_database()) \gexec

-- 3) Schemas: transformed_data lesen; public (PostGIS) bleibt USAGE fuer PUBLIC (PG15+-Default, kein CREATE)
CREATE SCHEMA IF NOT EXISTS transformed_data;
REVOKE ALL ON SCHEMA transformed_data FROM PUBLIC;
GRANT USAGE ON SCHEMA transformed_data TO api_ro;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;  -- No-op ab PG15, explizit fuer Klarheit

-- 4) Bestehende Relationen + kuenftige (dbt droppt/erzeugt bei jedem Lauf neu)
GRANT SELECT ON ALL TABLES IN SCHEMA transformed_data TO api_ro;   -- inkl. Views
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA transformed_data GRANT SELECT ON TABLES TO api_ro', :'owner_role') \gexec
