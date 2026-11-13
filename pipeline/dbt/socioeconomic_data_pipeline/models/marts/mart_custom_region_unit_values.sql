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
    - nuts3: mart_nuts3_sociodata_harmonized, fixed to the reference NUTS version (var reference_nuts_version)
             (like mart_agentur_dataset_aggregate). population = 'bevoelkerung'
             of the same year; employees = employee count of the same year -
             weight for 'weighted_avg_employees' indicators, taken from the
             dataset named in mart_dataset_aggregation_meta.employee_slug
             (erwerbstaetige_pro_1000_einwohner, absolute count in thousands,
             or sv_beschaeftigte for auszubildende); only valid (> 0) counts,
             NULL otherwise (the backend then falls back to the area weight).
             NULL for all other indicators.
    - lau:   mart_lau_sociodata_harmonized, the reference LAU version (var reference_lau_version).
             population = lau_integrated.population_count of the reference LAU
             vintage (constant over the observation years): lau_integrated.year
             is the Eurostat vintage, and target_gisco_id is a reference-vintage
             Gemeinde (matching the reference geometry the backend intersects).
             The vintage equal to the observation year may describe another
             territory under the same gisco_id (mergers) or be missing
             (years before/between loaded vintages -> NULL weight);
             employees is NULL (no employee figures on LAU level).
    - agentur: monthly BA figures (mart_agenturbezirk_sociodata), without
             population - counts are distributed by area share.
  month is only set for 'agentur'. is_crosswalked: value was converted
  across a version boundary (otherwise a direct value) - basis for the
  "calculated" flag on exactly matching regions.
*/

with nuts3_population as (
    select target_nuts_id as unit_id, year, harmonized_value as population
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = {{ var('reference_nuts_version') }}
      and dataset_slug = 'bevoelkerung'
),

-- Employee-count dataset per weighted_avg_employees indicator
nuts3_employee_base as (
    select distinct dataset_slug, employee_slug
    from {{ ref('mart_dataset_aggregation_meta') }}
    where level = 'nuts3'
      and employee_slug is not null
),

nuts3_employees as (
    select dataset_slug as employee_slug, target_nuts_id as unit_id, year, harmonized_value as employees
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = {{ var('reference_nuts_version') }}
      and dataset_slug in (select employee_slug from nuts3_employee_base)
      and has_valid_value
      and harmonized_value > 0
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
        e.employees::numeric         as employees,
        h.is_crosswalked
    from {{ ref('mart_nuts3_sociodata_harmonized') }} h
    left join nuts3_population p
        on  p.unit_id = h.target_nuts_id
        and p.year    = h.year
    left join nuts3_employee_base eb
        on  eb.dataset_slug = h.dataset_slug
    left join nuts3_employees e
        on  e.employee_slug = eb.employee_slug
        and e.unit_id       = h.target_nuts_id
        and e.year          = h.year
    where h.target_version = {{ var('reference_nuts_version') }}
),

lau_population as (
    select gisco_id, max(population_count) as population
    from {{ ref('lau_integrated') }}
    where population_count is not null
      and year = {{ var('reference_lau_version') }}
    group by gisco_id
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
        null::numeric                as employees,
        h.is_crosswalked
    from {{ ref('mart_lau_sociodata_harmonized') }} h
    left join lau_population p
        on  p.gisco_id = h.target_gisco_id
    where h.target_version = {{ var('reference_lau_version') }}
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
        null::numeric                                      as employees,
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
