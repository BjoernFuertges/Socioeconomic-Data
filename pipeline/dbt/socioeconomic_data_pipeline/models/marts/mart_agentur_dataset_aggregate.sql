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

  Source: mart_nuts3_sociodata_harmonized, fixed to target_version 2024
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
*/

{% set source_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'auszubildende', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg', 'kind': 'median'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'studierende', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg', 'kind': 'rate'},
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
    select nuts_id, year, value as population
    from harmonized
    where dataset_slug = 'bevoelkerung'
),

crosswalk as (
    select ba_schl, nuts_id, area_share_of_agentur, area_share_of_nuts3
    from {{ ref('mart_agentur_nuts3_crosswalk') }}
),

weighted as (
    select
        c.ba_schl,
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
        p.population * c.area_share_of_nuts3           as pop_weight
    from harmonized h
    join crosswalk c
        on c.nuts_id = h.nuts_id
    left join population p
        on  p.nuts_id = h.nuts_id
        and p.year    = h.year
),

grouped as (
    select
        ba_schl,
        dataset_slug,
        year,
        count(*)                                        as source_region_count,
        sum(value * sum_weight)                         as sum_value,
        sum(value * avg_weight)                         as area_num,
        sum(avg_weight)                                 as area_den,
        avg(value)                                      as plain_avg,
        sum(value * pop_weight)                         as pop_num,
        sum(pop_weight)                                 as pop_den,
        -- Population weight only if ALL contributing Kreise have it
        (count(pop_weight) = count(*) and coalesce(sum(pop_weight), 0) > 0) as pop_complete
    from weighted
    group by ba_schl, dataset_slug, year
)

select
    ba_schl,
    dataset_slug,
    year,
    source_region_count,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case
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
            {% else %}
            'sum'
            {% endif %}
    {% endfor %}
    end as aggregation_method,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then '{{ ds.kind }}'
    {% endfor %}
    end as value_kind
from grouped
