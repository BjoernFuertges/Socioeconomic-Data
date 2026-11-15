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
  Gemeinde (LAU) population with its geometry (Germany only).

  Basis for the population-weighted NUTS-3 version crosswalk (see
  int_lau_nuts3_membership / int_lau_nuts3_population_weight).

  At NUTS-3 level a real population weighting is impossible: the total
  population of a source region always cancels out in the normalization
  across target regions (constant factor). Only the finer LAU level shows
  how the population *within* a source region is split across several
  target regions.
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
