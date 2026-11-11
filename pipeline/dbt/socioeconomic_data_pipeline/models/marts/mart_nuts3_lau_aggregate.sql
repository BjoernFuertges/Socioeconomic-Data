{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregates LAU values (Gemeinden) up to NUTS-3 level (Kreise) - as a
  complement/cross-check to the native Kreis values in mart_nuts3_summary
  (which report e.g. population directly at Kreis level, not summed from
  the Gemeinden).

  Population: sum of lau_integrated.population_count per (nuts3_id, year),
  together with the number of assigned Gemeinden (lau_region_count) and how
  many of them have an actual population value
  (lau_region_count_with_population).

  Beschäftigungsquote: population-weighted average from
  mart_lau_sociodata_harmonized (dataset_slug='beschaeftigungsquote-gemeinden'),
  fixed to target_version 2024 (analogous to the fixed reference_nuts_version
  in mart_agentur_nuts3_crosswalk.sql), weighted by the Gemeinde population
  of the same year from lau_integrated.

  Note: like mart_lau_sociodata_harmonized.sql this requires
  loaded_data.lau_population to be filled via fetch_lau_population.py -
  otherwise stg_lau_population carries no gisco_id values and this mart
  builds but mostly yields NULL/0 (see the comment there).
*/

with lau_population as (
    select
        nuts3_id,
        year,
        gisco_id,
        population_count
    from {{ ref('lau_integrated') }}
    where nuts3_id is not null
),

population_agg as (
    select
        nuts3_id,
        year,
        sum(population_count)                                 as lau_population_sum,
        count(*)                                               as lau_region_count,
        count(*) filter (where population_count is not null)   as lau_region_count_with_population
    from lau_population
    group by nuts3_id, year
),

beschaeftigungsquote as (
    select
        lp.nuts3_id,
        lp.year,
        sum(h.harmonized_value * lp.population_count)
            / nullif(sum(lp.population_count), 0)             as lau_beschaeftigungsquote_weighted_avg
    from {{ ref('mart_lau_sociodata_harmonized') }} h
    join lau_population lp
        on  lp.gisco_id = h.target_gisco_id
        and lp.year      = h.year
    where h.dataset_slug    = 'beschaeftigungsquote-gemeinden'
      and h.target_version  = 2024
      and lp.population_count is not null
    group by lp.nuts3_id, lp.year
)

select
    p.nuts3_id,
    p.year,
    p.lau_population_sum,
    p.lau_region_count,
    p.lau_region_count_with_population,
    b.lau_beschaeftigungsquote_weighted_avg
from population_agg p
left join beschaeftigungsquote b
    on  b.nuts3_id = p.nuts3_id
    and b.year      = p.year
order by p.nuts3_id, p.year
