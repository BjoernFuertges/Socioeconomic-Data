{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregates all Kreis-grain socioeconomic datasets (see source_datasets
  below) up to NUTS-0/1/2 level - analogous to mart_nuts0/1/2_summary.sql
  (which so far do the same only for population/Schuldnerquote/Arbeitslose),
  but for the full dataset catalog. Makes selection/chart/table
  (DatasetPicker.vue) usable at country/Bundesland/Regierungsbezirk level,
  not just Kreis level - see GetNutsDatasetAggregateTimeseries
  (backend/internal/handlers/nuts.go).

  Source: mart_nuts3_sociodata_harmonized, fixed to target_version 2024
  (latest Kreis territorial structure - analogous to the fixed
  target_version in mart_nuts3_lau_aggregate.sql), NOT the
  interpolated/gap-filled version (mart_nuts3_timeseries_interpolated):
  aggregates run only over Kreis values that actually exist, no carried-over
  imputations. source_region_count shows how many Kreise contributed per
  year.

  measure_type (source_datasets below) must match the list of the same name
  in mart_nuts3_sociodata_harmonized.sql, which is authoritative; it is
  duplicated here because dbt cannot share Jinja variables across models
  (same pattern as mart_lau_sociodata_harmonized.sql vs.
  backend/cmd/gen-models/main.go). 'sum' sums unchanged; 'weighted_avg'
  averages population-weighted (weight: mart_nuts3_summary.bevoelkerung of
  the same year; Kreise without a population value are excluded instead of
  distorting with weight 0) - a plain mean would weight large and small
  Kreise equally. 'weighted_avg_employees' (arbeitsvolumen, bruttoverdienst:
  values per employed person) averages weighted by the number of employed
  persons (dataset erwerbstaetige_pro_1000_einwohner - despite its slug the
  absolute count in thousands) of the same year; Kreise without it are
  excluded, and without any weight the plain mean is used.
*/

{% set source_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'weighted_avg_employees'},
    {'slug': 'auszubildende', 'measure_type': 'weighted_avg'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg_employees'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'sum'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg'},
    {'slug': 'studierende', 'measure_type': 'weighted_avg'},
    {'slug': 'sv_beschaeftigte', 'measure_type': 'sum'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'weighted_avg'},
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg'},
] %}

with harmonized as (
    select
        dataset_slug,
        target_nuts_id as nuts_id,
        year,
        harmonized_value as value
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = 2024
),

population as (
    select nuts_id, year, bevoelkerung
    from {{ ref('mart_nuts3_summary') }}
    where bevoelkerung is not null
),

employees as (
    select nuts_id, year, value as employees
    from harmonized
    where dataset_slug = 'erwerbstaetige_pro_1000_einwohner'
),

weighted as (
    select
        h.dataset_slug,
        h.nuts_id,
        h.year,
        h.value,
        p.bevoelkerung as weight,
        e.employees    as emp_weight
    from harmonized h
    left join population p
        on  p.nuts_id = h.nuts_id
        and p.year    = h.year
    left join employees e
        on  e.nuts_id = h.nuts_id
        and e.year    = h.year
),

{% for lvl, prefix_len in [(0, 2), (1, 3), (2, 4)] %}
level_{{ lvl }} as (
    select
        {{ lvl }} as nuts_level,
        left(nuts_id, {{ prefix_len }}) as nuts_id,
        dataset_slug,
        year,
        count(*) as source_region_count,
        case dataset_slug
        {% for ds in source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                case
                    when sum(case when weight is not null and weight > 0 then weight else 0 end) > 0
                        then sum(case when weight is not null and weight > 0 then value * weight else 0 end)
                             / nullif(sum(case when weight is not null and weight > 0 then weight else 0 end), 0)
                    else avg(value)
                end
                {% elif ds.measure_type == 'weighted_avg_employees' %}
                case
                    when sum(case when emp_weight is not null and emp_weight > 0 then emp_weight else 0 end) > 0
                        then sum(case when emp_weight is not null and emp_weight > 0 then value * emp_weight else 0 end)
                             / nullif(sum(case when emp_weight is not null and emp_weight > 0 then emp_weight else 0 end), 0)
                    else avg(value)
                end
                {% else %}
                sum(value)
                {% endif %}
        {% endfor %}
        end as value
    from weighted
    group by left(nuts_id, {{ prefix_len }}), dataset_slug, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for lvl, prefix_len in [(0, 2), (1, 3), (2, 4)] %}
select * from level_{{ lvl }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
