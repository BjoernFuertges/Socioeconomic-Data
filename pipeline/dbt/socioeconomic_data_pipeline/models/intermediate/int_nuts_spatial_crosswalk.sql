{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_nuts_spatial_xwalk_src on {{ this }} (source_nuts_id, source_version)",
            "create index if not exists idx_int_nuts_spatial_xwalk_tgt on {{ this }} (target_nuts_id, target_version)"
        ]
    )
}}

/*
  Räumliche Schnittmengen zwischen NUTS-3-Versionen (nur Deutschland).

  Für jedes Versionspaar (2013→2016, 2016→2021, 2021→2024) wird berechnet,
  welcher Flächenanteil einer Quell-Region in jede Ziel-Region fällt.

    area_fraction = ST_Area(source ∩ target) / ST_Area(source)

  Schwellwert > 1 %: schließt reine Randberührungen aus und
  filtert numerische Ungenauigkeiten bei identischen Grenzen heraus.

  Diese Tabelle ist die rein geometrische Grundlage; die Bevölkerungsgewichtung
  erfolgt im nächsten Schritt (mart_nuts_version_crosswalk).
*/

with nuts_de as (
    select
        nuts_id,
        year            as version_year,
        geometry_3035
    from {{ ref('stg_nuts_geometry') }}
    where levl_code        = 3
      and country_code     = 'DE'
      and has_valid_geometry
),

version_pairs (source_version, target_version) as (
    values
        -- Vorwärts (ältere → neuere Version)
        (2013, 2016),
        (2016, 2021),
        (2021, 2024),
        -- Rückwärts (neuere → ältere Version, für Backward-Konversion)
        (2016, 2013),
        (2021, 2016),
        (2024, 2021)
)

select
    vp.source_version,
    vp.target_version,
    s.nuts_id                                                          as source_nuts_id,
    t.nuts_id                                                          as target_nuts_id,
    ST_Area(ST_Intersection(s.geometry_3035, t.geometry_3035))
        / nullif(ST_Area(s.geometry_3035), 0)                          as area_fraction

from version_pairs vp
join nuts_de s on s.version_year = vp.source_version
join nuts_de t on t.version_year = vp.target_version

where ST_Intersects(s.geometry_3035, t.geometry_3035)
  and ST_Area(ST_Intersection(s.geometry_3035, t.geometry_3035))
        / nullif(ST_Area(s.geometry_3035), 0) > 0.01
