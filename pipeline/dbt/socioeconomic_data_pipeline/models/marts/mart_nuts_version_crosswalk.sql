{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts_xwalk_src on {{ this }} (source_nuts_id, source_version, target_version)",
            "create index if not exists idx_mart_nuts_xwalk_tgt on {{ this }} (target_nuts_id, target_version, source_version)"
        ]
    )
}}

/*
  Population-weighted crosswalk between NUTS-3 versions (Germany).

  Direct (1-hop) mapping for EVERY version pair - int_nuts_spatial_crosswalk
  already computes the spatial intersection for all combinations of the
  available vintage years (currently 2003, 2006, 2010, 2013, 2016, 2021,
  2024), so chaining over intermediate versions would only accumulate
  rounding/weighting errors. n_hops is therefore always 1; the column is
  kept for backward compatibility of consumers.

  population_weight is the share of a source region's indicator value that
  goes to the respective target region. The weights per (source_nuts_id,
  source_version, target_version) sum to ~ 1.0.

  The population weighting itself comes from the Gemeinde (LAU) level
  (int_lau_nuts3_population_weight): only there can the population within a
  source region be distributed unevenly across target regions. Weighting
  with the source region's total population would cancel out in the
  normalization and be a pure area weighting.

  absolute_weight is ALWAYS a number of inhabitants (never mixed with area
  shares) and weight_basis says where it comes from:
    - 'population': the source group (source_nuts_id, source_version,
      target_version) has LAU population -> absolute_weight = inhabitants
      of the source region living in the target region
      (int_lau_nuts3_population_weight; 0 if none of them lives there).
    - 'area': only if the WHOLE source group has no LAU population (e.g.
      missing LAU data): area_fraction * population of the source region
      (largest LAU total of that region over all version pairs, 0 if the
      region has no LAU population at all). population_weight is then the
      normalized area share.
  Before, a missing pair population fell back per ROW to the bare
  area_fraction (0..1) next to inhabitant counts in the same group, which
  made those rows practically weightless (see int_lau_nuts3_membership).

  absolute_weight (the inhabitants before normalization to sum 1.0) is kept
  next to the normalized population_weight - needed for the weighted mean of
  ratios/averages in mart_nuts3_sociodata_harmonized
  (measure_type = 'weighted_avg'): merging several source regions must not
  sum such values (only correct for counts) but average them weighted.
  source_population (inhabitants of the whole source region, incl. pieces
  below the 1 % area threshold of int_nuts_spatial_crosswalk) is the
  matching weight for rows that need no crosswalk (identity rows there).

  is_estimate marks pairs whose weight is an estimate (see raw_weights):
  consumers propagate it as is_estimated on harmonized values.

  Example boundary reform:
    Kreis A (2016) spatially overlaps Kreis X and Kreis Y (2021). If 70 % of
    A's inhabitants live in the part that becomes X and 30 % in the part
    that becomes Y, weight(A->X) = 0.70 and weight(A->Y) = 0.30,
    independent of the plain area ratio.
*/

with spatial as (
    select * from {{ ref('int_nuts_spatial_crosswalk') }}
),

lau_pop_weight as (
    select * from {{ ref('int_lau_nuts3_population_weight') }}
),

-- Inhabitants of the source region per version pair (all pieces)
source_population as (
    select
        source_version,
        target_version,
        source_nuts_id,
        sum(lau_population) as source_population
    from lau_pop_weight
    group by source_version, target_version, source_nuts_id
),

-- Fallback population of a region independent of the version pair
region_population as (
    select
        source_version,
        source_nuts_id,
        max(source_population) as region_population
    from source_population
    group by source_version, source_nuts_id
),

joined as (
    select
        s.source_version,
        s.target_version,
        s.source_nuts_id,
        s.target_nuts_id,
        s.area_fraction,
        lpw.lau_population,
        coalesce(lpw.uncertain_population, 0) as uncertain_population,
        coalesce(sp.source_population, rp.region_population, 0) as source_population,
        coalesce(rp.region_population, 0)                       as region_population,
        coalesce(sum(lpw.lau_population) over (
            partition by s.source_nuts_id, s.source_version, s.target_version
        ), 0) as group_population
    from spatial s
    left join lau_pop_weight lpw
        on  lpw.source_version = s.source_version
        and lpw.target_version = s.target_version
        and lpw.source_nuts_id = s.source_nuts_id
        and lpw.target_nuts_id = s.target_nuts_id
    left join source_population sp
        on  sp.source_version = s.source_version
        and sp.target_version = s.target_version
        and sp.source_nuts_id = s.source_nuts_id
    left join region_population rp
        on  rp.source_version = s.source_version
        and rp.source_nuts_id = s.source_nuts_id
    where s.source_version != s.target_version
),

raw_weights as (
    select
        source_version,
        target_version,
        source_nuts_id,
        target_nuts_id,
        case
            when group_population > 0 then source_population
            else region_population
        end as source_population,
        case when group_population > 0 then 'population' else 'area' end as weight_basis,
        case
            when group_population > 0 then coalesce(lau_population, 0)
            else area_fraction * region_population
        end as absolute_weight,
        -- basis for the normalized share: inhabitants, or the area share
        -- when the source group has no population at all
        case
            when group_population > 0 then coalesce(lau_population, 0)
            else area_fraction
        end as share_basis,
        -- Estimate flag: the pair weight rests on an unreliable allocation.
        --  - 'area' basis: the whole source group has no LAU population
        --  - 'population' basis: at least var crosswalk_estimate_min_share
        --    of the pair's inhabitants come from Gemeinden that straddle a
        --    region boundary and are assigned wholly by majority (e.g.
        --    incorporations such as Potsdam 2003, Sachsen-Anhalt 2007)
        case
            when group_population > 0 then
                coalesce(lau_population, 0) > 0
                and uncertain_population >= {{ var('crosswalk_estimate_min_share', 0.05) }} * lau_population
            else true
        end as is_estimate
    from joined
)

-- Normalization: scale weights to sum 1.0 per source region
select
    source_version,
    target_version,
    source_nuts_id,
    target_nuts_id,
    share_basis / nullif(
        sum(share_basis) over (
            partition by source_nuts_id, source_version, target_version
        ), 0
    )                  as population_weight,
    absolute_weight::double precision as absolute_weight,
    weight_basis,
    source_population::double precision as source_population,
    1                  as n_hops,
    is_estimate
from raw_weights
order by source_version, target_version, source_nuts_id, target_nuts_id
