{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_custom_region_unit_values_lookup on {{ this }} (level, dataset_slug, indicator, year)"
        ]
    )
}}

/*
  Einheitliche Werte-Sicht je Ebene (LAU / NUTS-3 / Agenturbezirk) für die
  Custom-Region-Abfrage (POST /custom-regions/dataset-aggregate). Das
  Backend verschneidet nur noch das Nutzer-Polygon mit den Geometrien und
  gewichtet diese Werte - es transformiert selbst keine Daten.

  Eine Zeile je (level, unit_id, dataset_slug, indicator, year, month):
    - nuts3: mart_nuts3_sociodata_harmonized, fixiert auf target_version 2024
             (wie mart_agentur_dataset_aggregate). population = 'bevoelkerung'
             desselben Jahres.
    - lau:   mart_lau_sociodata_harmonized, target_version 2024.
             population = lau_integrated.population_count desselben Jahres.
    - agentur: monatliche BA-Zahlen (mart_agenturbezirk_sociodata), ohne
             population - Zählgrößen werden flächenanteilig verteilt.
  month ist nur bei 'agentur' gesetzt.
*/

with nuts3_population as (
    select target_nuts_id as unit_id, year, harmonized_value as population
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = 2024
      and dataset_slug = 'bevoelkerung'
),

nuts3 as (
    select
        'nuts3'::text                as level,
        h.target_nuts_id             as unit_id,
        h.dataset_slug,
        h.dataset_slug               as indicator,
        h.year,
        null::int                    as month,
        h.harmonized_value::numeric  as value,
        p.population::numeric        as population
    from {{ ref('mart_nuts3_sociodata_harmonized') }} h
    left join nuts3_population p
        on  p.unit_id = h.target_nuts_id
        and p.year    = h.year
    where h.target_version = 2024
),

lau_population as (
    select gisco_id, year, max(population_count) as population
    from {{ ref('lau_integrated') }}
    where population_count is not null
    group by gisco_id, year
),

lau as (
    select
        'lau'::text                  as level,
        h.target_gisco_id            as unit_id,
        h.dataset_slug,
        h.dataset_slug               as indicator,
        h.year,
        null::int                    as month,
        h.harmonized_value::numeric  as value,
        p.population::numeric        as population
    from {{ ref('mart_lau_sociodata_harmonized') }} h
    left join lau_population p
        on  p.gisco_id = h.target_gisco_id
        and p.year     = h.year
    where h.target_version = 2024
),

agentur as (
    select
        'agentur'::text                                    as level,
        ba_schl                                            as unit_id,
        'arbeitslosenzahlen-afa-agenturbezirke'::text      as dataset_slug,
        i.indicator,
        year,
        month,
        case i.indicator
            when 'arbeitslose' then arbeitslose
            when 'unterbeschaeftigung' then unterbeschaeftigung
        end::numeric                                       as value,
        null::numeric                                      as population
    from {{ ref('mart_agenturbezirk_sociodata') }}
    cross join (values ('arbeitslose'), ('unterbeschaeftigung')) as i(indicator)
    where year is not null
)

-- Zeilen ohne Wert (z.B. Monate ohne Meldung) tragen nichts zur Aggregation bei.
select *
from (
    select * from nuts3
    union all
    select * from lau
    union all
    select * from agentur
) u
where value is not null
