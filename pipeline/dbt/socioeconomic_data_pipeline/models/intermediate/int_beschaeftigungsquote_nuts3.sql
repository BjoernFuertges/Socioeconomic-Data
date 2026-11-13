/*
  Aggregates Beschäftigungsquote from Gemeinden (LAU) to NUTS-3 level.
  
  Two aggregation methods:
  1. Population-weighted average: (sum(quote * pop) / sum(pop))
  2. Median: 50th percentile across Gemeinden in NUTS-3 region
  
  Only Gemeinden with valid geodata_id_nuts mapping are included.
  Uses LAU population data from stg_lau_population for weighting.

  Approximation (relevant for data users): the weighted value is a mean of
  the Gemeinde quotas weighted by each Gemeinde's TOTAL population from the
  newest loaded Eurostat LAU vintage containing it (2024 for all current
  Gemeinden in the current load), applied to every observation year. The quota itself refers to residents of working age
  (15 to under 65), so the weight is not the quota's real denominator, and
  it is constant over time. Eurostat publishes one correspondence-table
  snapshot per year, not a historical series (see
  pipeline/dlt/fetch_lau_population.py), so it is not used as a reliable
  per-year population. Gemeinden without a population weight drop out of
  the weighted mean; weight_coverage (share of a region's Gemeinden that
  have a weight, per year) shows how complete it is. If no Gemeinde has a
  weight, beschaeftigungsquote_gewichtet falls back to the unweighted mean.
*/

with lau_data as (
    select
        original_id_1,
        original_name,
        geodata_name,
        geodata_id_nuts,
        year,
        beschaeftigungsquote_prozent
    from {{ ref('stg_beschaeftigungsquote_gemeinden') }}
    where has_valid_value = true
            and nullif(geodata_id_nuts, '') is not null  -- Only mapped Gemeinden
),

-- Population from the newest LAU vintage, matching the staging mapping
-- (stg_beschaeftigungsquote_gemeinden): not joined per observation year.
lau_population as (
    select distinct on (regexp_replace(cast(lau_nat_code as text), '\.0+$', ''))
        regexp_replace(cast(lau_nat_code as text), '\.0+$', '') as original_id_1,
        population_count
    from {{ ref('stg_lau_population') }}
    where has_valid_population = true
      and population_count > 0
    order by regexp_replace(cast(lau_nat_code as text), '\.0+$', ''), year desc
),

--  Combine LAU data with population for weighting
lau_with_population as (
    select
        l.original_id_1,
        l.original_name,
        l.geodata_name,
        l.geodata_id_nuts,
        l.year,
        l.beschaeftigungsquote_prozent,
        p.population_count
    from lau_data l
    left join lau_population p
        on p.original_id_1 = l.original_id_1
),

unweighted_stats as (
    select
        geodata_id_nuts,
        min(geodata_name) as geodata_name,
        year,
        -- Simple average (unweighted)
        round(avg(beschaeftigungsquote_prozent), 2) as beschaeftigungsquote_ungewichtet,
        
        -- Count of Gemeinden contributing to this region/year
        count(distinct original_id_1) as gemeinden_count,
        
        -- Min/Max for reference
        min(beschaeftigungsquote_prozent) as beschaeftigungsquote_min,
        max(beschaeftigungsquote_prozent) as beschaeftigungsquote_max,

        -- Sum of available population weights
        sum(population_count) as population_sum,

        -- Share of Gemeinden with a population weight (0..1)
        round(
            count(distinct original_id_1) filter (where population_count is not null)::numeric
            / nullif(count(distinct original_id_1), 0),
            4
        ) as weight_coverage
    from lau_with_population
    group by geodata_id_nuts, year
),

weighted as (
    select
        geodata_id_nuts,
        year,
        round(
            sum(beschaeftigungsquote_prozent * population_count)::numeric
            / nullif(sum(population_count), 0),
            2
        ) as beschaeftigungsquote_gewichtet
    from lau_with_population
    where population_count is not null
    group by geodata_id_nuts, year
),

median_values as (
    select
        geodata_id_nuts,
        year,
        round(
            percentile_cont(0.5) within group (order by beschaeftigungsquote_prozent)::numeric,
            2
        ) as beschaeftigungsquote_median
    from lau_with_population
    group by geodata_id_nuts, year
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['s.geodata_id_nuts', 's.year']) }} as beschaeftigungsquote_nuts3_pk,
        s.geodata_id_nuts,
        s.geodata_name,
        s.year,
        make_date(s.year, 1, 1) as observation_date,
        coalesce(w.beschaeftigungsquote_gewichtet, s.beschaeftigungsquote_ungewichtet) as beschaeftigungsquote_gewichtet,
        m.beschaeftigungsquote_median,
        s.beschaeftigungsquote_ungewichtet,
        s.gemeinden_count,
        s.beschaeftigungsquote_min,
        s.beschaeftigungsquote_max,
        s.population_sum,
        s.weight_coverage
    from unweighted_stats s
    left join weighted w
        on  w.geodata_id_nuts = s.geodata_id_nuts
        and w.year            = s.year
    left join median_values m
        on  m.geodata_id_nuts = s.geodata_id_nuts
        and m.year            = s.year
)

select * from final
