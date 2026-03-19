{{ config(materialized='view', schema='transformed_data') }}

with source_data as (
    select * from {{ source('loaded_data', 'results_arbeitslose_kreise') }}
),

cleaned as (
    select
        original_id_1,
        original_name,
        geodata_name,
        geodata_id_nuts,
        geodata_id_ars,
        mapper,
        parameter,
        geodata_level,
        geodata_year::integer as geodata_year,

        year::integer as year,
        make_date(year::integer, 1, 1) as observation_date,
        value as arbeitslose_raw,
        value::numeric as arbeitslose,

        case
            when value is null then false
            when value <= 0    then false
            else true
        end as has_valid_value

    from source_data
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['geodata_id_nuts', 'year']) }} as arbeitslosenzahlen_pk,
        *
    from cleaned
)

select * from final
