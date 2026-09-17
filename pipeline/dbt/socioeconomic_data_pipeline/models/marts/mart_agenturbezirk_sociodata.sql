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
  Agenturbezirke sind eine eigenständige Gebietsgliederung der
  Bundesagentur für Arbeit - nicht deckungsgleich mit NUTS-3/LAU/ARS und
  deshalb kein weiterer Eintrag in mart_nuts3_sociodata_harmonized. Da nur
  eine aktuelle Grenzen-Vintage verwendet wird, ist hier (anders als bei
  NUTS/LAU) kein Versions-Crosswalk nötig - einfacher Join von Geometrie
  und Werten über ba_schl. Dient sowohl der Agenturbezirk-Liste (distinct
  ba_schl/name/geometry) als auch der Monatszeitreihe pro Bezirk.

  nuts3_id kommt aus int_agentur_nuts3_spatial_link (größte Schnittfläche) -
  eine räumliche Schätzung, keine amtliche Zuordnung, dient als Verweis auf
  den ergänzenden Kreis-Level-Datensatz "Arbeitslose" (INKAR/BBSR,
  arbeitslose_kreise in mart_nuts3_summary).
*/

with geometry as (
    select * from {{ ref('stg_agenturbezirk_geometry') }}
),

sociodata as (
    select * from {{ ref('stg_arbeitslosenzahlen_afa_agenturbezirke') }}
),

nuts3_link as (
    select * from {{ ref('int_agentur_nuts3_spatial_link') }}
),

final as (
    select
        g.ba_schl,
        g.name,
        ST_AsGeoJSON(g.geometry_wgs84)::json as geometry_wgs84,
        g.has_valid_geometry,
        g.valid_from,
        g.valid_to,
        x.nuts_id as nuts3_id,
        s.year,
        s.month,
        s.observation_date,
        s.arbeitslose,
        s.unterbeschaeftigung
    from geometry g
    left join sociodata s on s.geodata_id_agentur = g.ba_schl
    left join nuts3_link x on x.ba_schl = g.ba_schl
)

select * from final
