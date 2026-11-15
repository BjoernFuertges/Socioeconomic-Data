{% macro soziodata_snapshot_query(source_table) %}
    /*
      Returns the currently valid value per (geodata_id_nuts, year).
      DISTINCT ON is defensive only: dlt loads these tables with
      write_disposition="replace", so a key should appear once per load; if
      two source regions map onto the same (geodata_id_nuts, year), one row
      is silently dropped here (ties on loaded_at are arbitrary).

      Used by all snap_results_*.sql snapshots.

      Two dlt schema variants are supported:
        - Old tables: have `loaded_at` as timestamptz (used directly)
        - New tables: only have `_dlt_load_id` (cast to numeric, aliased as loaded_at)
    */
    {% set source_relation = source('loaded_data', source_table) %}
    {% set cols = adapter.get_columns_in_relation(source_relation) | map(attribute='name') | map('lower') | list %}

    select distinct on (geodata_id_nuts, year)
        original_id_1,
        original_name,
        geodata_name,
        geodata_id_nuts,
        geodata_id_ars,
        mapper,
        parameter,
        geodata_level,
        geodata_year,
        year,
        value,
        {% if 'loaded_at' in cols %}
        loaded_at
        {% else %}
        _dlt_load_id::numeric as loaded_at
        {% endif %}
    from {{ source_relation }}
    order by geodata_id_nuts, year,
        {% if 'loaded_at' in cols %}
        loaded_at desc
        {% else %}
        _dlt_load_id::numeric desc
        {% endif %}
{% endmacro %}
