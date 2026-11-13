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

  Instead, each Gemeinde's population is distributed over the (source,
  target) region pairs it overlaps, proportional to the area of
  gemeinde ∩ source ∩ target:

    lau_population(S, T) = sum over Gemeinden g of
                           pop(g) * area(g ∩ S ∩ T) / area(g)

  (areal interpolation within a Gemeinde, i.e. population is assumed to be
  spread evenly over its territory). The pieces come from
  int_lau_nuts3_membership:
    - Gemeinde wholly in one region in both versions (the vast majority):
      full population to that pair, no geometry work.
    - split in only one of the two versions: weight_share of that side
      (g ∩ S ∩ T = g ∩ T if g lies in S).
    - split in both versions: the exact triple intersection is computed
      here (only for these few Gemeinden), renormalized per Gemeinde so its
      population is fully conserved.

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

-- Closest actually available LAU population year per reference year
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

-- Every (Gemeinde, source piece, target piece) combination per version pair
pairs as (
    select
        r.source_version,
        r.target_version,
        pop.gisco_id,
        pop.year,
        pop.population_count,
        src.nuts_id                          as source_nuts_id,
        tgt.nuts_id                          as target_nuts_id,
        src.is_split and tgt.is_split        as both_split,
        src.weight_share * tgt.weight_share  as share
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

-- NUTS-3 geometries per version, only needed for Gemeinden split in both
-- versions (same source as int_lau_nuts3_membership)
nuts_de as (
    select
        vy.version_year,
        g.nuts_id,
        g.geometry_3035
    from nuts_version_years vy
    join {{ ref('mart_nuts_geometry_historized') }} g
        on  g.levl_code    = 3
        and g.country_code = 'DE'
        and g.has_valid_geometry
        and g.valid_from <= vy.version_year
        and (g.valid_to is null or g.valid_to >= vy.version_year)
),

triple as (
    select
        p.source_version,
        p.target_version,
        p.gisco_id,
        p.source_nuts_id,
        p.target_nuts_id,
        ST_Area(
            ST_Intersection(
                ST_Intersection(l.geometry_3035, s.geometry_3035),
                t.geometry_3035
            )
        ) as triple_area
    from pairs p
    join {{ ref('int_lau_population_geo') }} l
        on  l.gisco_id = p.gisco_id
        and l.year     = p.year
    join nuts_de s
        on  s.version_year = p.source_version
        and s.nuts_id      = p.source_nuts_id
    join nuts_de t
        on  t.version_year = p.target_version
        and t.nuts_id      = p.target_nuts_id
    where p.both_split
),

triple_share as (
    select
        source_version,
        target_version,
        gisco_id,
        source_nuts_id,
        target_nuts_id,
        triple_area / nullif(
            sum(triple_area) over (partition by source_version, target_version, gisco_id), 0
        ) as share
    from triple
),

pieces as (
    select
        p.source_version,
        p.target_version,
        p.source_nuts_id,
        p.target_nuts_id,
        p.population_count * case
            when p.both_split then coalesce(ts.share, 0)
            else p.share
        end as population_piece
    from pairs p
    left join triple_share ts
        on  p.both_split
        and ts.source_version = p.source_version
        and ts.target_version = p.target_version
        and ts.gisco_id       = p.gisco_id
        and ts.source_nuts_id = p.source_nuts_id
        and ts.target_nuts_id = p.target_nuts_id
)

select
    source_version,
    target_version,
    source_nuts_id,
    target_nuts_id,
    sum(population_piece)  as lau_population
from pieces
group by source_version, target_version, source_nuts_id, target_nuts_id
having sum(population_piece) > 0
