{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_geom_hist_gist on {{ this }} using gist (geometry_3035)",
            "create index if not exists idx_mart_lau_geom_hist_id_from on {{ this }} (gisco_id, valid_from)",
            "create index if not exists idx_mart_lau_geom_hist_country on {{ this }} (country_code, valid_from)"
        ]
    )
}}

/*
  Historisierte LAU-Geometrien (Local Administrative Units).

  Jede Zeile repräsentiert eine Periode, in der eine Geometrie-Version für eine
  LAU-Einheit gültig war. Vintages mit identischer Geometrie werden zu einer Zeile
  zusammengefasst.

  valid_from  = erstes Vintage-Jahr, in dem diese Geometrie-Version erscheint
  valid_to    = letztes Jahr, in dem diese Version gültig war (NULL = noch aktuell)

  Datenquelle: stg_lau_geometry (enthält eine Zeile je gisco_id und Vintage-Jahr)
  Verfügbare Vintage-Jahre: 2011–2024
*/

with source_geoms as (
    -- Geometrie als kompakten Hash kodieren, damit der Vergleich zwischen
    -- Vintages günstig ist (Window-Funktionen puffern nur den Hash statt der
    -- vollständigen WKT-Strings).
    select
        gisco_id,
        country_code,
        lau_id,
        lau_name,
        nuts_3,
        year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry,
        md5(ST_AsText(geometry_3035)) as geom_hash
    from {{ ref('stg_lau_geometry') }}
),

flagged as (
    -- Periodenwechsel markieren: 1, wenn sich die Geometrie gegenüber dem
    -- vorherigen Vintage ändert (is distinct from deckt auch den ersten
    -- Datensatz je Einheit ab, bei dem lag() NULL liefert).
    select
        *,
        case
            when geom_hash is distinct from lag(geom_hash) over (
                partition by gisco_id
                order by year
            ) then 1
            else 0
        end as is_new_period
    from source_geoms
),

periods as (
    -- Island-Technik: kumulierte Summe der Wechsel-Flags ergibt eine stabile
    -- Perioden-ID je zusammenhängender Strecke mit identischer Geometrie.
    select
        *,
        sum(is_new_period) over (
            partition by gisco_id
            order by year
            rows between unbounded preceding and current row
        ) as period_id
    from flagged
),

period_bounds as (
    -- Je LAU und Periode genau eine Zeile: die des ersten Vintages
    -- (valid_from). Die Geometrie ist innerhalb der Periode identisch und wird
    -- direkt mitgenommen – kein Rück-Join nötig.
    select distinct on (gisco_id, period_id)
        gisco_id,
        country_code,
        lau_id,
        lau_name,
        nuts_3,
        year as valid_from,
        max(year) over (partition by gisco_id, period_id) as max_vintage_year,
        -- Letztes insgesamt verfügbares Vintage-Jahr über alle LAUs -
        -- Referenz, um "noch aktuell" von "stillgelegt" zu unterscheiden
        -- (siehe valid_to unten).
        max(year) over () as global_max_vintage_year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry
    from periods
    order by gisco_id, period_id, year
)

select
    gisco_id,
    country_code,
    lau_id,
    lau_name,
    nuts_3,
    valid_from,
    -- valid_to = Start der nächsten Periode - 1, falls eine existiert.
    -- Gibt es keine nächste Periode (letzte bekannte Periode dieser LAU),
    -- ist NULL nur dann korrekt "noch aktuell", wenn die LAU tatsächlich im
    -- global letzten Vintage-Jahr noch erfasst wurde. Andernfalls wurde der
    -- Code unter dieser gisco_id schlicht nicht mehr fortgeschrieben (z. B.
    -- durch Gemeindefusion stillgelegt, siehe max_vintage_year) - ohne diese
    -- Fallunterscheidung würde ein solcher Code für JEDES spätere
    -- geometry_year (auch das aktuelle) fälschlich als gültig gematcht,
    -- doppelt neben seinem Nachfolgecode.
    coalesce(
        lead(valid_from) over (partition by gisco_id order by valid_from) - 1,
        case when max_vintage_year < global_max_vintage_year then max_vintage_year end
    ) as valid_to,
    max_vintage_year,
    geometry_3035,
    ST_AsGeoJSON(geometry_wgs84)::json as geometry_wgs84,
    has_valid_geometry
from period_bounds
order by gisco_id, valid_from
