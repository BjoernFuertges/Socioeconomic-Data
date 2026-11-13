{#
  Safety net to the DB-side default privileges (backend/scripts/ensure_api_role.sql):
  re-grants read access for the API role on everything in transformed_data
  after every dbt invocation. No-op if the role does not exist (local dev).
#}
{% macro grant_api_read(role='api_ro', schema='transformed_data') %}
  {% if execute %}
    {% set exists = run_query("select 1 from pg_roles where rolname = '" ~ role ~ "'") %}
    {% if exists.rows | length > 0 %}
      {% do run_query("grant usage on schema " ~ schema ~ " to " ~ role ~ "; grant select on all tables in schema " ~ schema ~ " to " ~ role) %}
      {{ log("grant_api_read: select on " ~ schema ~ ".* granted to " ~ role, info=True) }}
    {% endif %}
  {% endif %}
{% endmacro %}
