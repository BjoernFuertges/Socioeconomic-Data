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

  Falls back to the plain area share (int_nuts_spatial_crosswalk) if no LAU
  population could be determined for a region pair (e.g. uninhabited area
  or missing LAU data).

  absolute_weight (= raw_weight before normalization to sum 1.0) is kept
  next to the normalized population_weight - needed for the weighted mean of
  ratios/averages in mart_nuts3_sociodata_harmonized
  (measure_type = 'weighted_avg'): merging several source regions must not
  sum such values (only correct for counts) but average them weighted.

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

-- Raw weight: actual Gemeinde (LAU) population per (source, target) region
-- pair. Falls back to area share if no LAU population is available.
raw_weights as (
    select
        s.source_version,
        s.target_version,
        s.source_nuts_id,
        s.target_nuts_id,
        coalesce(lpw.lau_population, s.area_fraction) as raw_weight
    from spatial s
    left join lau_pop_weight lpw
        on  lpw.source_version = s.source_version
        and lpw.target_version = s.target_version
        and lpw.source_nuts_id = s.source_nuts_id
        and lpw.target_nuts_id = s.target_nuts_id
)

-- Normalization: scale weights to sum 1.0 per source region
select
    source_version,
    target_version,
    source_nuts_id,
    target_nuts_id,
    raw_weight / nullif(
        sum(raw_weight) over (
            partition by source_nuts_id, source_version, target_version
        ), 0
    )              as population_weight,
    raw_weight     as absolute_weight,
    1              as n_hops
from raw_weights
where source_version != target_version
order by source_version, target_version, source_nuts_id, target_nuts_id
