{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_pop_geo_year on {{ this }} (year)",
            "create index if not exists idx_int_lau_pop_geo_gix on {{ this }} using gist (geometry_3035)"
        ]
    )
}}

/*
  Gemeinde(LAU)-Bevölkerung mit zugehöriger Geometrie (nur Deutschland).

  Grundlage für den populationsgewichteten NUTS-3-Versions-Crosswalk
  (siehe int_lau_nuts3_membership / int_lau_nuts3_population_weight):

  Auf NUTS-3-Ebene selbst lässt sich keine echte Populationsgewichtung
  berechnen, da die Gesamt-Bevölkerung einer Quellregion bei der Normierung
  über alle Zielregionen immer herausgekürzt wird (konstanter Faktor).
  Erst über die feinere Gemeinde-Ebene (LAU) lässt sich ermitteln, wie sich
  die Bevölkerung *innerhalb* einer Quellregion auf mehrere Zielregionen
  verteilt.
*/

select
    p.gisco_id,
    p.year,
    p.population_count,
    g.geometry_3035
from {{ ref('stg_lau_population') }} p
join {{ ref('stg_lau_geometry') }} g
    on  g.gisco_id = p.gisco_id
    and g.year     = p.year
where p.has_valid_population
  and g.has_valid_geometry
  and g.country_code = 'DE'
