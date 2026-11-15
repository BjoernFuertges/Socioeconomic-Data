{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_harm_target on {{ this }} (target_version, target_gisco_id, dataset_slug, year)",
            "create index if not exists idx_mart_lau_harm_ds_year on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Harmonized LAU datasets for several LAU target versions at once
  (target_version), analogous to mart_nuts3_sociodata_harmonized.sql - see
  the comment there for the general strategy (direct vs. crosswalked,
  merges).

  measure_type per dataset (lau_source_datasets below) selects the
  aggregation formula when several source regions merge into one target
  region:
    - 'sum':          harmonized_value = sum(value_i * weight_i)
                       (correct for counts, e.g. population)
    - 'weighted_avg':  harmonized_value = sum(value_i * absolute_weight_i)
                                            / sum(absolute_weight_i)
                       (correct for ratios/averages - plain summing would
                       yield wrong, too small values, see the absolute_weight
                       comment in mart_lau_version_crosswalk.sql)

  The region key is gisco_id (consistent with the geometry/crosswalk side).
  stg_beschaeftigungsquote_gemeinden carries no gisco_id, only official
  Gemeindeschlüssel (geodata_id_ars/geodata_id_lau); they are resolved to
  gisco_id here via stg_lau_population.lau_nat_code. This requires
  loaded_data.lau_population to actually carry gisco_id, i.e.
  fetch_lau_population.py must have run (part of run_all_pipelines.sh);
  otherwise load_geodata_to_postgis.py falls back to a variant without
  gisco_id/population and this mart stays empty although it builds (see
  pipeline/dlt/AGENTS.md).
*/

{% set target_versions = var('lau_versions') %}

-- Must match harmonizedLauRouteSlugs in backend/cmd/gen-models/main.go 1:1
-- (see the comment there).
{% set lau_source_datasets = [
    {'slug': 'beschaeftigungsquote-gemeinden', 'model': 'stg_beschaeftigungsquote_gemeinden', 'value_col': 'beschaeftigungsquote_prozent', 'measure_type': 'weighted_avg'},
] %}

with gisco_lookup as (
    select distinct on (lau_nat_code)
        lau_nat_code as ars_code,
        gisco_id
    from {{ ref('stg_lau_population') }}
    where lau_nat_code is not null and gisco_id is not null
    order by lau_nat_code, year desc
),

all_source_data as (
    {% for ds in lau_source_datasets %}
    select
        '{{ ds.slug }}'        as dataset_slug,
        gl.gisco_id            as gisco_id,
        s.geodata_year::integer as geodata_year,
        s.year::integer        as year,
        s.{{ ds.value_col }}::numeric as value
    from {{ ref(ds.model) }} s
    join gisco_lookup gl
        -- geodata_id_ars/lau may be the 7-digit BBSR key (states 01-09); ars_code is the 8-digit AGS
    on gl.ars_code = lpad(coalesce(s.geodata_id_ars, s.geodata_id_lau), 8, '0')
    -- Invalid source values (e.g. the placeholder 0 of the employment rate)
    -- must not enter crosswalk weights or averages: they would be blended in
    -- as real values. The gap is left open and filled by interpolation.
    where s.{{ ds.value_col }} is not null
      and s.has_valid_value
    {% if not loop.last %}union all{% endif %}
    {% endfor %}
),

{% for tv in target_versions %}
crosswalk_{{ tv }} as (
    select *
    from {{ ref('mart_lau_version_crosswalk') }}
    where target_version = {{ tv }}
),

harmonized_{{ tv }} as (

    -- Datasets already in the target version -> take over directly, no crosswalk
    select
        s.dataset_slug,
        s.gisco_id     as target_gisco_id,
        s.year,
        s.value,
        1.0::numeric   as weight,
        1.0::numeric   as absolute_weight,
        false          as is_crosswalked,
        s.gisco_id     as source_gisco_id,
        s.geodata_year as source_version,
        0              as n_hops
    from all_source_data s
    where s.geodata_year = {{ tv }}

    union all

    -- Datasets in other versions -> convert via crosswalk
    select
        s.dataset_slug,
        c.target_gisco_id,
        s.year,
        s.value,
        c.weight,
        c.absolute_weight,
        true            as is_crosswalked,
        s.gisco_id      as source_gisco_id,
        s.geodata_year  as source_version,
        1               as n_hops
    from all_source_data s
    join crosswalk_{{ tv }} c
        on  c.source_gisco_id = s.gisco_id
        and c.source_version  = s.geodata_year
    where s.geodata_year != {{ tv }}

),

-- Sum up (sum) or weighted-average (weighted_avg) merges, see measure_type
-- above; several source rows -> one target row
aggregated_{{ tv }} as (
    select
        dataset_slug,
        target_gisco_id,
        year,
        {{ tv }} as target_version,
        case dataset_slug
        {% for ds in lau_source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        {% endfor %}
        end                      as harmonized_value,
        bool_or(is_crosswalked)  as is_crosswalked,
        min(source_version)      as source_version,
        max(n_hops)              as n_hops
    from harmonized_{{ tv }}
    group by dataset_slug, target_gisco_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_gisco_id, year
