{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_agenturbezirk_latest_agentur on {{ this }} (ba_schl)"
        ]
    )
}}

/*
  One row per Agenturbezirk with its latest reporting month
  (arbeitslose/unterbeschaeftigung) - for the map view, which (unlike
  NUTS/LAU) has no time-series/interpolation mart yet (see
  mart_agenturbezirk_sociodata.sql: gap filling is a deliberate later
  extension). Districts without any value (year is null after the left join
  in mart_agenturbezirk_sociodata) are dropped here instead of appearing on
  the map with empty values.
*/

with ranked as (
    select
        *,
        row_number() over (
            partition by ba_schl
            order by observation_date desc
        ) as rn
    from {{ ref('mart_agenturbezirk_sociodata') }}
    where year is not null
)

select
    ba_schl,
    name,
    geometry_wgs84,
    has_valid_geometry,
    valid_from,
    valid_to,
    year,
    month,
    observation_date,
    arbeitslose,
    unterbeschaeftigung
from ranked
where rn = 1
