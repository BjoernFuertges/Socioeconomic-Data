{% snapshot snap_results_beschaeftigungsquote_gemeinden %}

{{
    config(
        unique_key="original_id_1 || '_' || year::text",
        strategy='check',
        check_cols=['value'],
        target_schema='snapshots',
        invalidate_hard_deletes=false
    )
}}

/*
  Beschäftigungsquote ist ein LAU/Gemeinden-Datensatz.
  Die Quelltabelle hat kein geodata_id_nuts, daher eigener unique_key
  auf Basis von original_id_1 (Gemeinde-ID).
*/
select distinct on (original_id_1, year)
    original_id_1,
    original_name,
    geodata_name,
    mapper,
    parameter,
    geodata_level,
    geodata_year,
    year,
    value,
    _dlt_load_id::numeric as loaded_at
from {{ source('loaded_data', 'results_beschaeftigungsquote_gemeinden') }}
order by original_id_1, year, _dlt_load_id::numeric desc

{% endsnapshot %}
