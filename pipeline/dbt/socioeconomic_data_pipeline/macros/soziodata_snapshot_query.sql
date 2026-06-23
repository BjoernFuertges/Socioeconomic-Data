{% macro soziodata_snapshot_query(source_table) %}
    /*
      Liefert den aktuell gültigen Wert je (geodata_id_nuts, year)-Kombination.
      Mit DISTINCT ON wird bei mehreren Ladeläufen (append) immer nur die
      neueste Version berücksichtigt.

      Wird in allen snap_results_*.sql-Snapshots verwendet.
    */
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
        loaded_at
    from {{ source('loaded_data', source_table) }}
    order by geodata_id_nuts, year, loaded_at desc
{% endmacro %}
