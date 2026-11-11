{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_{{ this.name }}_geodata_year on {{ this }} (geodata_id_nuts, year)",
            "create index if not exists idx_{{ this.name }}_nuts_tpo on {{ this }} (geodata_id_nuts text_pattern_ops)"
        ]
    )
}}

with source_data as (
    select * from {{ ref('snap_results_studierende_kreise') }}
    where dbt_valid_to is null
),

cleaned as (
    select
        -- Remove _dlt* fields by explicitly selecting only needed columns
        original_id_1,
        original_name,
        geodata_name,
        geodata_id_nuts,
        geodata_id_ars,
        mapper,
        parameter,
        geodata_level,
        geodata_year::integer as geodata_year,

        -- Observation fields
        year::integer as year,
        value as studierende,

        -- Derived observation date: 1 January of the year
        make_date(year::integer, 1, 1) as observation_date,

        -- Data quality flag
        case
            when value is null then false
            when value < 0     then false
            else true
        end as has_valid_value

    from source_data
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['geodata_id_nuts', 'year']) }} as studierende_pk,
        *
    from cleaned
)

select * from final
