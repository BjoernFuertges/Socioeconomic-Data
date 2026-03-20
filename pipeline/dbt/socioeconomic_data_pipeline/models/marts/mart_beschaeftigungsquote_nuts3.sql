{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

/*
  Aggregates Beschäftigungsquote from Gemeinden (LAU) to NUTS-3 level.
  
  Two aggregation methods:
  1. Population-weighted average: (sum(quote * pop) / sum(pop))
  2. Median: 50th percentile across Gemeinden in NUTS-3 region
  
  Only Gemeinden with valid geodata_id_nuts mapping are included.
    Uses LAU population data from stg_lau_population for weighting.
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

lau_population as (
    select
        regexp_replace(cast(lau_nat_code as text), '\\.0+$', '') as original_id_1,
        year,
        population_count
    from {{ ref('stg_lau_population') }}
    where has_valid_population = true
      and population_count > 0
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
        and l.year = p.year
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
        sum(population_count) as population_sum
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
        s.population_sum
    from unweighted_stats s
    left join weighted w
        on  w.geodata_id_nuts = s.geodata_id_nuts
        and w.year            = s.year
    left join median_values m
        on  m.geodata_id_nuts = s.geodata_id_nuts
        and m.year            = s.year
)

select * from final
