{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregates all Kreis-grain socioeconomic datasets (see source_datasets
  below) up to Agenturbezirk level - the Agentur counterpart of
  mart_nuts012_dataset_aggregate.sql. Unlike there (plain prefix grouping,
  since NUTS-0/1/2 are strict supersets of NUTS-3 codes), Agenturbezirke
  and Kreise have no prefix relation, so the n:m area-share crosswalk
  mart_agentur_nuts3_crosswalk (area_share_of_agentur) is used for
  weighting; it also covers the Berlin case (3 Agenturbezirke share one
  Kreis).

  Source: mart_nuts3_sociodata_harmonized, fixed to the reference version (var reference_nuts_version)
  (same reference NUTS version as mart_agentur_nuts3_crosswalk.sql), NOT
  the interpolated/gap-filled version - aggregates run only over Kreis
  values that actually exist, no carried-over imputations.
  source_region_count shows how many Kreise contributed per year.

  measure_type (source_datasets below) must match the list of the same name
  in mart_nuts3_sociodata_harmonized.sql, which is authoritative; it is
  duplicated here (same pattern as mart_nuts012_dataset_aggregate.sql).

  The two area shares from mart_agentur_nuts3_crosswalk are NOT
  interchangeable - 'sum' and 'weighted_avg' deliberately need different
  weights:
    - 'sum' (counts, e.g. population): distributes the Kreis value over
      each overlapping Agenturbezirk, weighted by the share of the KREIS
      lying in this Agenturbezirk (area_share_of_nuts3). Otherwise a Kreis
      entirely within one Agenturbezirk (area_share_of_nuts3 = 1.0) but
      only a small part of it (small area_share_of_agentur) would have its
      population counted only partially instead of fully.
    - 'weighted_avg' (ratios/averages): averages the Kreis values. The
      preferred weight is the POPULATION of the Kreis part lying in this
      Agenturbezirk (Kreis population from source 'bevoelkerung' in the
      same year * area_share_of_nuts3) -> aggregation_method
      'weighted_avg_population'. If any contributing Kreis lacks population
      for that year, the whole row falls back to the area weight
      area_share_of_agentur -> 'weighted_avg_area' (the two weights are
      never mixed within a row). For Berlin (one Kreis, three
      Agenturbezirke) the weight has no effect; the value is that of the
      single Kreis.
    - 'weighted_avg_employees' (values per employed person, e.g.
      arbeitsvolumen, bruttoverdienst): same as 'weighted_avg', but the
      preferred weight is the number of EMPLOYED PERSONS of the Kreis part
      (employee count of the same year * area_share_of_nuts3) ->
      aggregation_method 'weighted_avg_employees'. The employee count is
      dataset erwerbstaetige_pro_1000_einwohner (despite its slug the
      absolute count in thousands) unless the entry names another one via
      'employee_slug' (auszubildende: per 1,000 SV-Beschaeftigte ->
      'sv_beschaeftigte'). Only valid (has_valid_value, > 0) counts qualify.
      Fallback if any contributing Kreis lacks it - e.g. auszubildende
      before 2007, when sv_beschaeftigte starts: the POPULATION weight
      ('weighted_avg_population'), then the area weight as above. Weights
      are never mixed within one row.

  LIMITATIONS (approximations, not an exact recomputation):
    - A true Agenturbezirk ratio cannot be recovered from finished Kreis
      ratios (numerator/denominator are not available). Total population
      is only a proxy for the actual reference group (e.g. adults, working
      age, children).
    - Medians (value_kind 'median') cannot be composed from Kreis medians;
      the weighted average is only an estimate.
    - 'sum' values for Kreise spanning several Agenturbezirke (only Berlin)
      are split by area share.
  value_kind ('count' | 'rate' | 'mean' | 'median') and aggregation_method
  are passed unchanged to the API/frontend labelling.

  Coverage: value is computed over the Kreise that have a value that year
  and is NOT corrected for missing ones (a 'sum' over an incomplete set is
  simply too small). expected_region_count = number of Kreise overlapping the
  Agenturbezirk in mart_agentur_nuts3_crosswalk; is_complete = every one of
  them contributed a value.
*/

{% set source_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'weighted_avg_employees', 'kind': 'mean'},
    {'slug': 'auszubildende', 'measure_type': 'weighted_avg_employees', 'employee_slug': 'sv_beschaeftigte', 'kind': 'rate'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg_employees', 'kind': 'mean'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg', 'kind': 'median'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'studierende', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sv_beschaeftigte', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg', 'kind': 'rate'},
] %}
{% set default_employee_slug = 'erwerbstaetige_pro_1000_einwohner' %}

with harmonized as (
    select
        dataset_slug,
        target_nuts_id as nuts_id,
        year,
        harmonized_value as value
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = {{ var('reference_nuts_version') }}
),

population as (
    select nuts_id, year, value as population
    from harmonized
    where dataset_slug = 'bevoelkerung'
),

-- Employee-count base per weighted_avg_employees dataset (see header)
employee_base as (
    select * from (
        values
        {% for ds in source_datasets if ds.measure_type == 'weighted_avg_employees' %}
            ('{{ ds.slug }}', '{{ ds.employee_slug or default_employee_slug }}'){% if not loop.last %},{% endif %}
        {% endfor %}
    ) as t (dataset_slug, employee_slug)
),

employees as (
    select dataset_slug as employee_slug, target_nuts_id as nuts_id, year, harmonized_value as employees
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = {{ var('reference_nuts_version') }}
      and dataset_slug in (select employee_slug from employee_base)
      and has_valid_value
      and harmonized_value > 0
),

crosswalk as (
    select ba_schl, nuts_id, area_share_of_agentur, area_share_of_nuts3
    from {{ ref('mart_agentur_nuts3_crosswalk') }}
),

-- Kreise per Agenturbezirk (coverage flag, see header)
expected as (
    select ba_schl, count(distinct nuts_id) as expected_region_count
    from crosswalk
    group by ba_schl
),

weighted as (
    select
        c.ba_schl,
        h.nuts_id,
        h.dataset_slug,
        h.year,
        h.value,
        p.population,
        -- sum_weight: share of the KREIS lying in this Agenturbezirk.
        -- avg_weight: share this Kreis makes up of the AGENTURBEZIRK (area).
        -- pop_weight: population of the Kreis part lying in this Agenturbezirk.
        -- See the header comment for why they are not interchangeable.
        c.area_share_of_nuts3                          as sum_weight,
        c.area_share_of_agentur                        as avg_weight,
        p.population * c.area_share_of_nuts3           as pop_weight,
        -- emp_weight: employed persons of the Kreis part in this Agenturbezirk.
        e.employees * c.area_share_of_nuts3            as emp_weight
    from harmonized h
    join crosswalk c
        on c.nuts_id = h.nuts_id
    left join population p
        on  p.nuts_id = h.nuts_id
        and p.year    = h.year
    left join employee_base eb
        on  eb.dataset_slug = h.dataset_slug
    left join employees e
        on  e.employee_slug = eb.employee_slug
        and e.nuts_id       = h.nuts_id
        and e.year          = h.year
),

grouped as (
    select
        ba_schl,
        dataset_slug,
        year,
        count(*)                                        as source_region_count,
        count(distinct nuts_id) filter (where value is not null) as contributing_region_count,
        sum(value * sum_weight)                         as sum_value,
        sum(value * avg_weight)                         as area_num,
        sum(avg_weight)                                 as area_den,
        avg(value)                                      as plain_avg,
        sum(value * pop_weight)                         as pop_num,
        sum(pop_weight)                                 as pop_den,
        -- Population weight only if ALL contributing Kreise have it
        (count(pop_weight) = count(*) and coalesce(sum(pop_weight), 0) > 0) as pop_complete,
        sum(value * emp_weight)                         as emp_num,
        sum(emp_weight)                                 as emp_den,
        (count(emp_weight) = count(*) and coalesce(sum(emp_weight), 0) > 0) as emp_complete
    from weighted
    group by ba_schl, dataset_slug, year
)

select
    g.ba_schl,
    g.dataset_slug,
    g.year,
    g.source_region_count,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case
                when pop_complete then pop_num / pop_den
                when area_den > 0 then area_num / area_den
                else plain_avg
            end
            {% elif ds.measure_type == 'weighted_avg_employees' %}
            case
                when emp_complete then emp_num / emp_den
                when pop_complete then pop_num / pop_den
                when area_den > 0 then area_num / area_den
                else plain_avg
            end
            {% else %}
            sum_value
            {% endif %}
    {% endfor %}
    end as value,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case when pop_complete then 'weighted_avg_population' else 'weighted_avg_area' end
            {% elif ds.measure_type == 'weighted_avg_employees' %}
            case
                when emp_complete then 'weighted_avg_employees'
                when pop_complete then 'weighted_avg_population'
                else 'weighted_avg_area'
            end
            {% else %}
            'sum'
            {% endif %}
    {% endfor %}
    end as aggregation_method,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then '{{ ds.kind }}'
    {% endfor %}
    end as value_kind,
    x.expected_region_count,
    (g.contributing_region_count = x.expected_region_count) as is_complete
from grouped g
join expected x
    on x.ba_schl = g.ba_schl
