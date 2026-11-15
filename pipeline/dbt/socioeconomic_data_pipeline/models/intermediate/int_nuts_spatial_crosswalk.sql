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
  Spatial intersections between NUTS-3 versions (Germany only).

  For EVERY pair of different vintage years in stg_nuts_geometry (currently
  2003, 2006, 2010, 2013, 2016, 2021, 2024), the area share of a source
  region falling into each target region is computed directly - not just
  for adjacent versions. This makes mart_nuts_version_crosswalk independent
  of hop chaining over intermediate versions (e.g. 2003 -> 2024 is a direct
  intersection) and more accurate, since chaining would accumulate
  rounding/weighting errors per hop.

    area_fraction = ST_Area(source ∩ target) / ST_Area(source)

  Threshold > 1 %: excludes pure boundary touches and numerical noise at
  identical borders.

  Purely the geometric basis; population weighting happens in
  mart_nuts_version_crosswalk.

  Reads int_nuts3_de_geometry (materialized, GiST-indexed) instead of the
  stg_nuts_geometry view, so the spatial self-join can use the index.
*/

with nuts_de as (
    select
        nuts_id,
        version_year,
        geometry_3035
    from {{ ref('int_nuts3_de_geometry') }}
),

version_years as (
    select distinct version_year from nuts_de
),

-- All directed version pairs (every year -> every other year), not just
-- adjacent vintages.
version_pairs as (
    select
        a.version_year as source_version,
        b.version_year as target_version
    from version_years a
    cross join version_years b
    where a.version_year != b.version_year
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
