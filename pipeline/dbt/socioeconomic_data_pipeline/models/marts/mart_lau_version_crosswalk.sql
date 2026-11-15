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

  1. identity_pairs: trivial self-mapping for every Gemeinde that exists in
     both years (by far the most common case), no spatial computation. Its
     absolute_weight is 1.0 minus the area share claimed by spatial partners
     (see 2.), so it is 1.0 for every Gemeinde without a boundary change.
     Deliberately covers the WHOLE lifecycle of a gisco_id (first to last
     vintage period, via min(valid_from)/max(coalesce(valid_to,
     max_vintage_year))), not just years within one geometry period:
     mart_lau_geometry_historized collapses far less than NUTS (~12 instead
     of 1-3 periods per Gemeinde) because GISCO re-generalizes boundaries
     every year. This fragmentation almost never means a real
     administrative change, so int_lau_spatial_crosswalk computes ONLY
     pairs with source_gisco_id != target_gisco_id (real mergers/splits);
     all year pairs of the same gisco_id are covered cheaply here.
  2. Spatial candidates: for Gemeinden with an actual administrative
     boundary change (merger, split), the spatial intersection from
     int_lau_spatial_crosswalk, with absolute_weight = area share.

  Identity and spatial candidates of one (source_gisco_id, source_version,
  target_version) group are normalized together, so the weights always sum
  to 1.0 (test: assert_lau_crosswalk_weights_sum_to_one).

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

-- 1. Identity candidates: all year pairs across a Gemeinde's whole lifecycle.
identity_raw as (
    select
        sy.year    as source_version,
        ty.year    as target_version,
        l.gisco_id as source_gisco_id,
        l.gisco_id as target_gisco_id
    from lau_de_lifespan l
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as sy(year)
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as ty(year)
    where sy.year != ty.year
),

-- 2. Real boundary changes (candidates with their area share of the source).
spatial as (
    select * from {{ ref('int_lau_spatial_crosswalk') }}
),

-- Area share already claimed by spatial partners, per source and year pair.
spatial_claimed as (
    select
        source_version,
        target_version,
        source_gisco_id,
        sum(area_fraction) as claimed_fraction
    from spatial
    group by source_version, target_version, source_gisco_id
),

-- The identity candidate only receives what the spatial partners do not
-- claim. Without this, a Gemeinde that still exists in the target year and
-- also overlaps a partner (the partner geometry is the latest one, so a
-- Gemeinde absorbing another one shows up here) would be counted twice.
identity_pairs as (
    select
        i.source_version,
        i.target_version,
        i.source_gisco_id,
        i.target_gisco_id,
        greatest(1.0 - coalesce(c.claimed_fraction, 0), 0)::numeric as absolute_weight
    from identity_raw i
    left join spatial_claimed c
        on  c.source_version = i.source_version
        and c.target_version = i.target_version
        and c.source_gisco_id = i.source_gisco_id
),

candidates as (
    select source_version, target_version, source_gisco_id, target_gisco_id, absolute_weight
    from identity_pairs
    union all
    select source_version, target_version, source_gisco_id, target_gisco_id, area_fraction
    from spatial
)

-- Normalized so that the weights of every (source_gisco_id, source_version,
-- target_version) group sum to 1.0 (identity and spatial candidates together).
select
    source_version,
    target_version,
    source_gisco_id,
    target_gisco_id,
    absolute_weight / nullif(
        sum(absolute_weight) over (
            partition by source_gisco_id, source_version, target_version
        ), 0
    )              as weight,
    absolute_weight
from candidates
where absolute_weight > 0
order by source_version, target_version, source_gisco_id, target_gisco_id
