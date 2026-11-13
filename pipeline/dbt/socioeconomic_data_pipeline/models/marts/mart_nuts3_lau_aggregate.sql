{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

{% set target_version = var('reference_nuts_version') %}

/*
  Aggregates LAU values (Gemeinden) up to NUTS-3 level (Kreise) - as a
  complement/cross-check to the native Kreis values in mart_nuts3_summary
  (which report e.g. population directly at Kreis level, not summed from
  the Gemeinden).

  Region key: nuts3_id is ALWAYS the NUTS-3 code of the reference version
  {{ target_version }} (same fixed version as mart_agentur_nuts3_crosswalk.sql
  and the backend's NUTS/LAU reference), for every year. lau_integrated.year
  is the Eurostat LAU vintage, not an observation year, and its declared
  nuts3_id follows that vintage's NUTS version (e.g. the 2020 vintage still
  carries NUTS 2016 codes such as DE915/DE918 for what is DE91C today), so
  it must not be used as the region key across years. Per gisco_id the
  NUTS-3 code comes from the {{ target_version }} vintage row of
  lau_integrated, falling back to the spatial link against NUTS
  {{ target_version }} (int_lau_nuts3_spatial_link) for Gemeinden that no
  longer exist in that vintage (dissolved/merged ones).

  Population (per LAU vintage year): sum of lau_integrated.population_count
  per (nuts3_id, year), together with the number of assigned Gemeinden
  (lau_region_count) and how many of them have an actual population value
  (lau_region_count_with_population). These counts belong to the
  population part only (0 in years without a LAU vintage).

  Beschäftigungsquote (per observation year): population-weighted average
  from mart_lau_sociodata_harmonized (dataset_slug=
  'beschaeftigungsquote-gemeinden') at target_version {{ target_version }}.
  Its target_gisco_id is a Gemeinde of the {{ target_version }} vintage, so
  the weight is the population of that same vintage (constant over the
  observation years), NOT the vintage equal to the observation year: that
  one may describe a different territory under the same gisco_id (mergers)
  or not exist at all (years before/between the loaded vintages). Same
  approximation as int_beschaeftigungsquote_nuts3.sql.

  The number of Gemeinden with a valid quote in that year (and a
  population weight) is lau_beschaeftigungsquote_region_count (0 without
  a quote) - use it, not the population counts, as source count of the quote.

  Rows: full outer join of both parts on (nuts3_id, year), so observation
  years without a LAU vintage still get their quote (counts are then 0,
  lau_population_sum NULL).

  Note: like mart_lau_sociodata_harmonized.sql this requires
  loaded_data.lau_population to be filled via fetch_lau_population.py -
  otherwise stg_lau_population carries no gisco_id values and this mart
  builds but mostly yields NULL/0 (see the comment there).
*/

with lau_rows as (
    select
        gisco_id,
        year,
        nuts3_id,
        population_count
    from {{ ref('lau_integrated') }}
),

-- gisco_id -> NUTS-3 code of the reference version (see header).
nuts3_ref as (
    select
        coalesce(t.gisco_id, s.gisco_id) as gisco_id,
        coalesce(t.nuts3_id, s.nuts_id)  as nuts3_id
    from (
        select distinct on (gisco_id) gisco_id, nuts3_id
        from lau_rows
        where year = {{ target_version }}
          and nuts3_id is not null
        order by gisco_id, nuts3_id
    ) t
    full outer join {{ ref('int_lau_nuts3_spatial_link') }} s
        on s.gisco_id = t.gisco_id
),

lau_population as (
    select
        r.nuts3_id,
        l.gisco_id,
        l.year,
        l.population_count
    from lau_rows l
    join nuts3_ref r
        on r.gisco_id = l.gisco_id
    where r.nuts3_id is not null
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

-- Population weight of the target vintage, one row per gisco_id.
target_population as (
    select
        gisco_id,
        nuts3_id,
        max(population_count) as population_count
    from lau_population
    where year = {{ target_version }}
      and population_count is not null
    group by gisco_id, nuts3_id
),

beschaeftigungsquote as (
    select
        tp.nuts3_id,
        h.year,
        sum(h.harmonized_value * tp.population_count)
            / nullif(sum(tp.population_count), 0)             as lau_beschaeftigungsquote_weighted_avg,
        count(distinct h.target_gisco_id)
            filter (where h.harmonized_value is not null)     as lau_beschaeftigungsquote_region_count
    from {{ ref('mart_lau_sociodata_harmonized') }} h
    join target_population tp
        on tp.gisco_id = h.target_gisco_id
    where h.dataset_slug    = 'beschaeftigungsquote-gemeinden'
      and h.target_version  = {{ target_version }}
    group by tp.nuts3_id, h.year
)

select
    coalesce(p.nuts3_id, b.nuts3_id)                  as nuts3_id,
    coalesce(p.year, b.year)                          as year,
    p.lau_population_sum,
    coalesce(p.lau_region_count, 0)                   as lau_region_count,
    coalesce(p.lau_region_count_with_population, 0)   as lau_region_count_with_population,
    b.lau_beschaeftigungsquote_weighted_avg,
    coalesce(b.lau_beschaeftigungsquote_region_count, 0) as lau_beschaeftigungsquote_region_count
from population_agg p
full outer join beschaeftigungsquote b
    on  b.nuts3_id = p.nuts3_id
    and b.year      = p.year
order by 1, 2
