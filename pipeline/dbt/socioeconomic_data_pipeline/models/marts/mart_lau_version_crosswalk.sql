{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_xwalk_src on {{ this }} (source_gisco_id, source_version, target_version)",
            "create index if not exists idx_mart_lau_xwalk_tgt on {{ this }} (target_gisco_id, target_version, source_version)"
        ]
    )
}}

/*
  Crosswalk between LAU versions (Germany), analogous to
  mart_nuts_version_crosswalk.sql. Two parts:

  1. identity_pairs: trivial self-mapping with weight = 1.0 for every
     Gemeinde without an administrative boundary change between source and
     target year (by far the most common case), no spatial computation.
     Deliberately covers the WHOLE lifecycle of a gisco_id (first to last
     vintage period, via min(valid_from)/max(coalesce(valid_to,
     max_vintage_year))), not just years within one geometry period:
     mart_lau_geometry_historized collapses far less than NUTS (~12 instead
     of 1-3 periods per Gemeinde) because GISCO re-generalizes boundaries
     every year. This fragmentation almost never means a real
     administrative change, so int_lau_spatial_crosswalk computes ONLY
     pairs with source_gisco_id != target_gisco_id (real mergers/splits);
     all year pairs of the same gisco_id are covered cheaply here.
  2. crosswalk_pairs: for Gemeinden with an actual administrative boundary
     change (merger, split), the spatial intersection from
     int_lau_spatial_crosswalk, normalized to sum ~ 1.0 per source region.

  Both sets are disjoint at (source_gisco_id, source_version,
  target_version) level (identity_pairs always has target_gisco_id =
  source_gisco_id, crosswalk_pairs never), so UNION ALL suffices.

  Unlike NUTS there is no sub-LAU population layer (LAU is the finest level
  available), so the weight is purely area-based, no population fallback.

  absolute_weight (the un-normalized weight before scaling to sum 1.0) is
  kept next to the normalized weight column - needed for the weighted mean
  of ratios/averages in mart_lau_sociodata_harmonized
  (measure_type = 'weighted_avg'), where values are averaged, not summed.
*/

with lau_de_lifespan as (
    -- Whole lifecycle per gisco_id across all its periods (see comment
    -- above) - only the year span is needed, no spatial information.
    select
        gisco_id,
        min(valid_from) as min_valid_from,
        max(coalesce(valid_to, max_vintage_year)) as max_valid_to
    from {{ ref('mart_lau_geometry_historized') }}
    where country_code = 'DE'
      and has_valid_geometry
    group by gisco_id
),

-- 1. Trivial self-mapping: all year pairs across a Gemeinde's whole
-- lifecycle.
identity_pairs as (
    select
        sy.year    as source_version,
        ty.year    as target_version,
        l.gisco_id as source_gisco_id,
        l.gisco_id as target_gisco_id,
        1.0::numeric as weight,
        1.0::numeric as absolute_weight
    from lau_de_lifespan l
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as sy(year)
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as ty(year)
    where sy.year != ty.year
),

-- 2. Real boundary changes, normalized to sum 1.0 per source region.
spatial as (
    select * from {{ ref('int_lau_spatial_crosswalk') }}
),

crosswalk_pairs as (
    select
        source_version,
        target_version,
        source_gisco_id,
        target_gisco_id,
        area_fraction / nullif(
            sum(area_fraction) over (
                partition by source_gisco_id, source_version, target_version
            ), 0
        )              as weight,
        area_fraction  as absolute_weight
    from spatial
)

select * from identity_pairs
union all
select * from crosswalk_pairs
order by source_version, target_version, source_gisco_id, target_gisco_id
