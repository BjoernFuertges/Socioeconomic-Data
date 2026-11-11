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
  Unified values view per level (LAU / NUTS-3 / Agenturbezirk) for the
  custom-region query (POST /custom-regions/dataset-aggregate). The backend
  only intersects the user polygon with the geometries and weights these
  values - it does not transform any data itself.

  One row per (level, unit_id, dataset_slug, indicator, year, month):
    - nuts3: mart_nuts3_sociodata_harmonized, fixed to target_version 2024
             (like mart_agentur_dataset_aggregate). population = 'bevoelkerung'
             of the same year.
    - lau:   mart_lau_sociodata_harmonized, target_version 2024.
             population = lau_integrated.population_count of the same year.
    - agentur: monthly BA figures (mart_agenturbezirk_sociodata), without
             population - counts are distributed by area share.
  month is only set for 'agentur'. is_crosswalked: value was converted
  across a version boundary (otherwise a direct value) - basis for the
  "calculated" flag on exactly matching regions.
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
        p.population::numeric        as population,
        h.is_crosswalked
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
        p.population::numeric        as population,
        h.is_crosswalked
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
        null::numeric                                      as population,
        false                                              as is_crosswalked
    from {{ ref('mart_agenturbezirk_sociodata') }}
    cross join (values ('arbeitslose'), ('unterbeschaeftigung')) as i(indicator)
    where year is not null
)

-- Rows without a value (e.g. months without a report) do not contribute to the aggregation.
select *
from (
    select * from nuts3
    union all
    select * from lau
    union all
    select * from agentur
) u
where value is not null
