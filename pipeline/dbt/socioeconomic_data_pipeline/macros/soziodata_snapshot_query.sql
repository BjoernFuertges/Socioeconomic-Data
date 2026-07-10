{% macro soziodata_snapshot_query(source_table) %}
    /*
      Liefert den aktuell gültigen Wert je (geodata_id_nuts, year)-Kombination.
      Mit DISTINCT ON wird bei mehreren Ladeläufen (append) immer nur die
      neueste Version berücksichtigt.

      Wird in allen snap_results_*.sql-Snapshots verwendet.

      Zwei DLT-Schema-Varianten werden unterstützt:
        - Alte Tabellen: haben `loaded_at` als timestamptz (direkt verwenden)
        - Neue Tabellen: haben nur `_dlt_load_id` (als numeric casten, als loaded_at aliassen)
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
