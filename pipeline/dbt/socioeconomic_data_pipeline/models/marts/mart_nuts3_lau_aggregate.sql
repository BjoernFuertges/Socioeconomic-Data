{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert LAU-Werte (Gemeinden) auf NUTS-3-Ebene (Kreise) hoch - als
  Ergänzung/Cross-Check zu den nativen Kreis-Werten in mart_nuts3_summary
  (die Bevölkerung z. B. direkt auf Kreisebene ausweisen, nicht aus den
  zugehörigen Gemeinden aufsummiert).

  Bevölkerung: Summe von lau_integrated.population_count je (nuts3_id, year),
  zusammen mit der Anzahl zugeordneter Gemeinden (lau_region_count) und der
  Anzahl davon mit einem tatsächlichen Bevölkerungswert
  (lau_region_count_with_population).

  Beschäftigungsquote: bevölkerungsgewichteter Durchschnitt aus
  mart_lau_sociodata_harmonized (dataset_slug='beschaeftigungsquote-gemeinden'),
  fixiert auf target_version 2024 (analog zur festen reference_nuts_version in
  mart_agentur_nuts3_crosswalk.sql), gewichtet mit der Gemeinde-Bevölkerung im
  selben Jahrgang aus lau_integrated.

  Hinweis: setzt wie mart_lau_sociodata_harmonized.sql voraus, dass
  loaded_data.lau_population über fetch_lau_population.py befüllt wurde -
  fehlt das, führt stg_lau_population keine gisco_id-Werte und dieser
  Mart baut, liefert aber überwiegend NULL/0 (siehe dortigen Kommentar).
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
