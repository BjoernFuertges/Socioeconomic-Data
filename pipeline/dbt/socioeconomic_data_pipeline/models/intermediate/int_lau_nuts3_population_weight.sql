{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_nuts3_popw_src on {{ this }} (source_nuts_id, source_version, target_version)",
            "create index if not exists idx_int_lau_nuts3_popw_tgt on {{ this }} (target_nuts_id, target_version, source_version)"
        ]
    )
}}

/*
  Population-based raw weight per NUTS-3 version pair, derived from the
  actual Gemeinde (LAU) population. lau_population is always a number of
  inhabitants.

  Why not the total population of the NUTS-3 source region? That value is
  identical for all target regions of a source region and cancels out
  completely in the normalization (division by the per-source-region sum),
  so the result would be purely area-based despite being called
  "population-weighted".

  Instead, each Gemeinde's whole population goes to the (source, target)
  region pair it lies in (majority assignment per version, see
  int_lau_nuts3_membership):

    lau_population(S, T) = sum of pop(g) over Gemeinden g in S (version A)
                           and T (version B)

  Reference year per version pair: the younger of the two NUTS version
  years minus 1 (population just before the newer version took effect),
  i.e. ref_year = GREATEST(source_version, target_version) - 1. This holds
  regardless of direction and generalizes to any version pair, e.g.:
    2013 -> 2016 : 2015        2016 -> 2013 : 2015
    2016 -> 2021 : 2020        2021 -> 2016 : 2020
    2021 -> 2024 : 2023        2024 -> 2021 : 2023
    2003 -> 2024 : 2023        2024 -> 2003 : 2023

  The closest *available* LAU population year is used (see resolved below).
  The Eurostat population snapshots for 2018-2025 are loaded, so pairs with
  an earlier reference year fall back to the 2018 snapshot.
*/

with available_years as (
    select distinct year from {{ ref('int_lau_population_geo') }}
),

nuts_version_years as (
    select distinct version_year from {{ ref('int_lau_nuts3_membership') }}
),

-- All directed version pairs (every year -> every other year), not just
-- adjacent vintages - analogous to int_nuts_spatial_crosswalk.
version_pairs as (
    select
        a.version_year                                  as source_version,
        b.version_year                                  as target_version,
        greatest(a.version_year, b.version_year) - 1     as ref_year
    from nuts_version_years a
    cross join nuts_version_years b
    where a.version_year != b.version_year
),

-- Closest actually available LAU population year per reference year.
-- int_lau_nuts3_membership only computes these years (pop_years CTE there):
-- change both together.
resolved as (
    select
        vp.source_version,
        vp.target_version,
        (
            select ay.year
            from available_years ay
            order by abs(ay.year - vp.ref_year), ay.year
            limit 1
        ) as pop_year
    from version_pairs vp
),

membership as (
    select * from {{ ref('int_lau_nuts3_membership') }}
),

population as (
    select gisco_id, year, population_count
    from {{ ref('int_lau_population_geo') }}
),

-- Every Gemeinde with its source and target region per version pair
pairs as (
    select
        r.source_version,
        r.target_version,
        pop.gisco_id,
        pop.year,
        pop.population_count,
        src.nuts_id                          as source_nuts_id,
        tgt.nuts_id                          as target_nuts_id,
        -- the Gemeinde lies (by area) not clearly inside its assigned region
        -- in at least one of the two versions: its whole population is
        -- allocated by majority assignment, so the pair weight is an estimate
        least(src.area_share, tgt.area_share) < {{ var('crosswalk_uncertain_area_share', 0.9) }}
                                             as is_uncertain
    from resolved r
    join population pop
        on pop.year = r.pop_year
    join membership src
        on  src.gisco_id     = pop.gisco_id
        and src.year         = pop.year
        and src.version_year = r.source_version
    join membership tgt
        on  tgt.gisco_id     = pop.gisco_id
        and tgt.year         = pop.year
        and tgt.version_year = r.target_version
),

pieces as (
    select source_version, target_version, source_nuts_id, target_nuts_id,
           population_count as population_piece, is_uncertain
    from pairs
)

select
    source_version,
    target_version,
    source_nuts_id,
    target_nuts_id,
    sum(population_piece)  as lau_population,
    -- inhabitants of Gemeinden assigned wholly although they straddle a
    -- region boundary (area share of the assigned region below
    -- var crosswalk_uncertain_area_share) in the source or target version
    coalesce(sum(population_piece) filter (where is_uncertain), 0) as uncertain_population
from pieces
group by source_version, target_version, source_nuts_id, target_nuts_id
having sum(population_piece) > 0
