{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_agenturbezirk_sociodata_agentur on {{ this }} (ba_schl)",
            "create index if not exists idx_mart_agenturbezirk_sociodata_period on {{ this }} (ba_schl, year, month)"
        ]
    )
}}

/*
  Agenturbezirke are an independent territorial division of the
  Bundesagentur für Arbeit - not congruent with NUTS-3/LAU/ARS, hence not
  another entry in mart_nuts3_sociodata_harmonized. Since only one current
  boundary vintage is used, no version crosswalk is needed (unlike NUTS/LAU)
  - a simple join of geometry and values on ba_schl. Serves both the
  Agenturbezirk list (distinct ba_schl/name/geometry) and the monthly time
  series per district.

  The link to NUTS-3 (Agenturbezirke usually consist of one or more whole
  Kreise, exception Berlin: 3 Agenturbezirke share one Kreis) is n:m with
  area/population shares - see mart_agentur_nuts3_crosswalk.sql, not
  modelled here as a single column.
*/

with geometry as (
    select * from {{ ref('stg_agenturbezirk_geometry') }}
),

sociodata as (
    select * from {{ ref('stg_arbeitslosenzahlen_afa_agenturbezirke') }}
),

final as (
    select
        g.ba_schl,
        g.name,
        ST_AsGeoJSON(g.geometry_wgs84)::json as geometry_wgs84,
        g.has_valid_geometry,
        g.valid_from,
        g.valid_to,
        s.year,
        s.month,
        s.observation_date,
        s.arbeitslose,
        s.unterbeschaeftigung
    from geometry g
    left join sociodata s on s.geodata_id_agentur = g.ba_schl
)

select * from final
