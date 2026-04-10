{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_arbeitslosenzahlen_geodata_id on {{ this }} (geodata_id)",
            "create index if not exists idx_mart_arbeitslosenzahlen_geodata_obs on {{ this }} (geodata_id, observation_date)",
            "create index if not exists idx_mart_arbeitslosenzahlen_year on {{ this }} (year)",
            "create index if not exists idx_mart_arbeitslosenzahlen_obs_date on {{ this }} (observation_date)"
        ]
    )
}}

with staged_data as (
    select * from {{ ref('stg_arbeitslosenzahlen') }}
),

deduplicated as (
    select * from staged_data
    where has_valid_value
),

final as (
    select
        arbeitslosenzahlen_pk,
        geodata_id_nuts  as geodata_id,
        geodata_name,
        geodata_id_ars,
        geodata_level,
        geodata_year,
        year,
        observation_date,
        arbeitslose,

        -- Year-over-year change
        lag(arbeitslose, 1) over (
            partition by geodata_id_nuts
            order by year
        ) as arbeitslose_previous_year,

        case
            when lag(arbeitslose, 1) over (
                partition by geodata_id_nuts
                order by year
            ) > 0 then
                round(
                    (arbeitslose - lag(arbeitslose, 1) over (
                        partition by geodata_id_nuts
                        order by year
                    )) * 100.0 / lag(arbeitslose, 1) over (
                        partition by geodata_id_nuts
                        order by year
                    ),
                    2
                )
            else null
        end as yoy_change_percent

    from deduplicated
)

select * from final
