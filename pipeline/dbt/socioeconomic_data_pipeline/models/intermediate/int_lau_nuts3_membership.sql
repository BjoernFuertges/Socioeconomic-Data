{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_nuts3_memb on {{ this }} (gisco_id, year, version_year)"
        ]
    )
}}

/*
  Majority assignment of each Gemeinde (LAU) to one NUTS-3 region per NUTS
  version (2003, 2006, 2010, 2013, 2016, 2021, 2024): the whole Gemeinde
  goes to the region holding the largest share of its area. One row per
  (gisco_id, year, version_year).

  Why whole Gemeinden and not area-proportional pieces: a Gemeinde belongs
  to exactly one Kreis; where LAU and NUTS geometries (generalized
  independently) show it in two regions, that is boundary noise. The
  proportional variant was evaluated against 648 Kreis versions with known
  truth (analysis/data-analysis/nuts3_crosswalk_evaluation.ipynb): MAPE
  0.19 % for majority assignment vs 0.35-2 % for proportional splitting
  (e.g. Suhl -24 %, Bremerhaven 2003 20 % shifted to Cuxhaven).

  Known limitation: today's Gemeinden formed by incorporations (Potsdam
  2003, Sachsen-Anhalt 2007) lay in several older Kreise; they are assigned
  wholly to one, so back-calculations to 2003/2006 are estimates there
  (no ground truth exists for them).

  Columns:
    area_share = ST_Area(gemeinde ∩ region) / ST_Area(gemeinde) of the
                 chosen region (informational).

  Only computed for the LAU population years present in
  int_lau_population_geo, to keep the spatial join small.

  NUTS-3 geometries come from mart_nuts_geometry_historized, not
  stg_nuts_geometry: the staging view re-parses geometry_3035 via
  ST_GeomFromText on every query and cannot be indexed, which made this
  join hang. The mart stores a GiST-indexed geometry per validity period
  (valid_from/valid_to), so version_year is filtered against that range.
*/

with version_years as (
    select distinct year as version_year
    from {{ ref('stg_nuts_geometry') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
),

nuts_de as (
    select
        vy.version_year,
        g.nuts_id,
        g.geometry_3035
    from version_years vy
    join {{ ref('mart_nuts_geometry_historized') }} g
        on  g.levl_code    = 3
        and g.country_code = 'DE'
        and g.has_valid_geometry
        and g.valid_from <= vy.version_year
        and (g.valid_to is null or g.valid_to >= vy.version_year)
),

lau_years as (
    select distinct gisco_id, year, geometry_3035
    from {{ ref('int_lau_population_geo') }}
),

pieces as (
    select
        l.gisco_id,
        l.year,
        n.version_year,
        n.nuts_id,
        ST_Area(ST_Intersection(l.geometry_3035, n.geometry_3035))
            / nullif(ST_Area(l.geometry_3035), 0) as area_share
    from lau_years l
    join nuts_de n
        on ST_Intersects(l.geometry_3035, n.geometry_3035)
),

ranked as (
    select
        *,
        row_number() over (
            partition by gisco_id, year, version_year
            order by area_share desc, nuts_id
        ) as rn
    from pieces
    where area_share > 0
)

select
    gisco_id,
    year,
    version_year,
    nuts_id,
    area_share
from ranked
where rn = 1
