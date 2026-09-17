{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_agentur_nuts3_spatial_link on {{ this }} (ba_schl)"
        ]
    )
}}

{% set reference_nuts_version = 2024 %}

/*
  Räumliche Zuordnung jedes Agenturbezirks (Bundesagentur für Arbeit) zu
  seiner NUTS-3-Region, gegen eine feste Referenz-NUTS-Version (aktuell/
  neueste, {{ reference_nuts_version }}) - gleiche Logik wie
  int_lau_nuts3_spatial_link.sql: Zuordnung über die größte Schnittfläche
  (statt reinem ST_Contains), um numerische Ungenauigkeiten und
  Randberührungen an Bezirksgrenzen abzufangen. Agenturbezirke sind eine
  eigenständige, nicht mit NUTS-3/LAU/Kreisen deckungsgleiche
  Gebietsgliederung - diese Zuordnung ist daher eine räumliche Schätzung,
  keine amtliche Zuordnung, analog zu nuts3_link_source =
  'spatial_estimate' in lau_integrated.sql.

  stg_agenturbezirk_geometry führt (anders als stg_nuts_geometry/
  stg_lau_geometry) nur eine WGS84-Geometrie (der dlt-Loader reprojiziert
  bereits, siehe dortiger Kommentar) - deshalb hier ST_Transform nach
  3035 statt einer bereits vorhandenen 3035-Spalte. Bei nur ~156
  Agenturbezirken gegen ~400 NUTS-3-Regionen ist das performant genug,
  eine eigene materialisierte 3035-Spalte lohnt sich nicht.
*/

with nuts_de as (
    select
        nuts_id,
        geometry_3035
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
      and valid_from <= {{ reference_nuts_version }}
      and (valid_to is null or valid_to >= {{ reference_nuts_version }})
),

agentur as (
    select
        ba_schl,
        ST_Transform(geometry_wgs84, 3035) as geometry_3035
    from {{ ref('stg_agenturbezirk_geometry') }}
    where has_valid_geometry
),

ranked as (
    select
        a.ba_schl,
        n.nuts_id,
        row_number() over (
            partition by a.ba_schl
            order by ST_Area(ST_Intersection(a.geometry_3035, n.geometry_3035)) desc
        ) as rn
    from agentur a
    join nuts_de n
        on ST_Intersects(a.geometry_3035, n.geometry_3035)
)

select
    ba_schl,
    nuts_id
from ranked
where rn = 1
