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
  Eine Zeile je Agenturbezirk mit dem jeweils neuesten Berichtsmonat
  (arbeitslose/unterbeschaeftigung) - für die Kartendarstellung, die (anders
  als NUTS/LAU) noch keine Zeitreihen-/Interpolations-Mart hat (siehe
  mart_agenturbezirk_sociodata.sql: Gap-Filling ist bewusst spätere
  Erweiterung). Bezirke ohne jeden Wert (year is null nach dem Left Join in
  mart_agenturbezirk_sociodata) fallen hier raus, statt mit leeren Werten
  auf der Karte zu erscheinen.
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
    nuts3_id,
    year,
    month,
    observation_date,
    arbeitslose,
    unterbeschaeftigung
from ranked
where rn = 1
