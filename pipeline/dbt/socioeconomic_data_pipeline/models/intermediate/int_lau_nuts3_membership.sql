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
  Spatial split of each Gemeinde (LAU) across the NUTS-3 regions of each NUTS
  version (2003, 2006, 2010, 2013, 2016, 2021, 2024). One row per
  (gisco_id, year, version_year, nuts_id) piece.

  Why pieces instead of "whole Gemeinde -> region with the largest share":
  a LAU vintage (2019-2024) does not fit older NUTS boundaries - e.g. the
  2007 Kreisreform in Sachsen-Anhalt or the 2003 Potsdam incorporations put
  large parts of today's Gemeinden into a different NUTS-3 region of 2003.
  Assigning such a Gemeinde wholesale to one region gave a (source, target)
  pair of int_lau_nuts3_population_weight no population at all unless some
  Gemeinde had its largest share exactly in source ∩ target, so the
  crosswalk silently fell back to area shares and lost up to ~75 % of a
  source region's population.

  Columns:
    area_share   = ST_Area(gemeinde ∩ region) / ST_Area(gemeinde)
    weight_share = share of the Gemeinde's population attributed to this
                   region. The area shares are first renormalized over the
                   Gemeinde's covered area (parts outside every German NUTS-3
                   geometry, e.g. coast lines generalized differently, would
                   otherwise lose population). If one region then holds at
                   least var('lau_nuts3_snap_share') (default 0.95) of the
                   Gemeinde, the Gemeinde is "snapped" wholly to that region
                   (weight_share = 1, single row): LAU and NUTS geometries
                   are generalized independently, so small splits are
                   boundary noise, not real territory.
    is_split     = true if the Gemeinde is genuinely split across regions of
                   this version (several rows, weight_share sums to 1); the
                   exact gemeinde ∩ source ∩ target area is then computed in
                   int_lau_nuts3_population_weight.
    is_largest_share = the region with the largest area share (the former
                   single assignment).

  Only computed for the LAU population years present in
  int_lau_population_geo, to keep the spatial join small.

  NUTS-3 geometries come from mart_nuts_geometry_historized, not
  stg_nuts_geometry: the staging view re-parses geometry_3035 via
  ST_GeomFromText on every query and cannot be indexed, which made this
  join hang. The mart stores a GiST-indexed geometry per validity period
  (valid_from/valid_to), so version_year is filtered against that range.
*/

{% set snap_share = var('lau_nuts3_snap_share', 0.95) %}

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

normalized as (
    select
        *,
        area_share / sum(area_share) over w      as share_norm,
        max(area_share) over w / sum(area_share) over w as max_share_norm,
        row_number() over (
            partition by gisco_id, year, version_year
            order by area_share desc, nuts_id
        ) as rn
    from pieces
    where area_share > 0
    window w as (partition by gisco_id, year, version_year)
)

select
    gisco_id,
    year,
    version_year,
    nuts_id,
    area_share,
    case
        when max_share_norm >= {{ snap_share }} then 1.0
        else share_norm
    end                                    as weight_share,
    max_share_norm < {{ snap_share }}      as is_split,
    rn = 1                                 as is_largest_share
from normalized
where max_share_norm < {{ snap_share }}
   or rn = 1
