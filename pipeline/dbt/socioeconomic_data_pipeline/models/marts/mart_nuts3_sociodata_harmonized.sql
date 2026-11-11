{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts3_harm_target on {{ this }} (target_version, target_nuts_id, dataset_slug, year)",
            "create index if not exists idx_mart_nuts3_harm_ds_year on {{ this }} (dataset_slug, year)"
        ]
    )
}}
/*
  Harmonized socioeconomic data for several NUTS-3 target versions at once
  (see target_versions below), distinguished by the column target_version.

  mart_nuts_version_crosswalk provides a direct (1-hop) mapping for EVERY
  pair of vintage years in stg_nuts_geometry (see
  int_nuts_spatial_crosswalk), so every version is reachable from every
  other regardless of the source data's geodata_year. target_versions
  therefore simply lists all available NUTS-3 vintage years.

  Strategy per target version:
  - Regions whose geodata_year already equals the target version -> weight = 1.0
  - All others -> crosswalk from mart_nuts_version_crosswalk
  - On merges: values of several source regions are summed proportionally
    (harmonized_value = sum(value_i * weight_i))

  is_crosswalked = true   -> value was converted across a version boundary
  is_crosswalked = false  -> direct value, no conversion needed

  measure_type per dataset (source_datasets below, plus
  beschaeftigungsquote-nuts3 separately) selects the aggregation formula when
  several source regions merge into one target region:
    - 'sum':          harmonized_value = sum(value_i * weight_i)
                       (correct for counts, e.g. population)
    - 'weighted_avg':  harmonized_value = sum(value_i * absolute_weight_i)
                                            / sum(absolute_weight_i)
                       (correct for ratios/averages - plain summing would
                       yield wrong, too small values, see the absolute_weight
                       comment in mart_nuts_version_crosswalk.sql)
*/

{% set target_versions = [2003, 2006, 2010, 2013, 2016, 2021, 2024] %}

-- Slugs deliberately match the RouteSlugs of the generated Kreis API
-- endpoints 1:1 so the backend can join both sides without a mapping table.
-- measure_type is derived from the column descriptions/meta.yaml per
-- dataset (count = 'sum', ratio/average = 'weighted_avg').
{% set source_datasets = [
    {'slug': 'arbeitslose',        'table': 'results_arbeitslose_kreise', 'measure_type': 'sum'},
    {'slug': 'arbeitsvolumen',     'table': 'results_arbeitsvolumen_kreise', 'measure_type': 'sum'},
    {'slug': 'auszubildende',      'table': 'results_auszubildende_kreise', 'measure_type': 'sum'},
    {'slug': 'bevoelkerung',       'table': 'results_bevoelkerung_kreise', 'measure_type': 'sum'},
    {'slug': 'bruttoverdienst',    'table': 'results_bruttoverdienst_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'durchschnittsalter', 'table': 'results_durchschnittsalter_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'table': 'results_erwerbstaetige_pro_1000_einwohner_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'kinderarmut',        'table': 'results_kinderarmut_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'medianeinkommen',    'table': 'results_medianeinkommen_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'schuldnerquote',     'table': 'results_schuldnerquote_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'sgb_ii_quote',       'table': 'results_sgb_ii_quote_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'studierende',        'table': 'results_studierende_kreise', 'measure_type': 'sum'},
    {'slug': 'wohngeldhaushalte',  'table': 'results_wohngeldhaushalte_kreise', 'measure_type': 'sum'},
] %}
{% set beschaeftigungsquote_nuts3_measure_type = 'weighted_avg' %}

with all_source_data as (

    -- Union of all Kreis raw datasets. Loaded with write_disposition=replace,
    -- so no duplicates per (geodata_id_nuts, year) are expected; DISTINCT is
    -- a defensive guard because sum() measure types would silently add up
    -- any accidentally reloaded duplicate into inflated values.
    {% for ds in source_datasets %}
    select distinct
        '{{ ds.slug }}'       as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer as geodata_year,
        year::integer         as year,
        value::numeric        as value
    from {{ source('loaded_data', ds.table) }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and value           is not null
    union all
    {% endfor %}

    -- beschaeftigungsquote-nuts3 is not a raw loaded_data source but already
    -- aggregated to NUTS-3 (2024) via the Gemeinde (LAU) population (see
    -- stg_beschaeftigungsquote_nuts3.sql / mart_beschaeftigungsquote_nuts3.sql),
    -- hence ref() instead of source().
    select
        'beschaeftigungsquote-nuts3' as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer        as geodata_year,
        year::integer                as year,
        beschaeftigungsquote_prozent::numeric as value
    from {{ ref('stg_beschaeftigungsquote_nuts3') }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and beschaeftigungsquote_prozent is not null

),

{% for tv in target_versions %}
crosswalk_{{ tv }} as (
    select *
    from {{ ref('mart_nuts_version_crosswalk') }}
    where target_version = {{ tv }}
),

harmonized_{{ tv }} as (

    -- Datasets already in the target version -> take over directly, no crosswalk
    select
        s.dataset_slug,
        s.geodata_id_nuts   as target_nuts_id,
        s.year,
        s.value,
        1.0::numeric        as weight,
        1.0::numeric        as absolute_weight,
        false               as is_crosswalked,
        s.geodata_id_nuts   as source_nuts_id,
        s.geodata_year      as source_version,
        0                   as n_hops
    from all_source_data s
    where s.geodata_year = {{ tv }}

    union all

    -- Datasets in other versions -> convert via crosswalk
    select
        s.dataset_slug,
        c.target_nuts_id,
        s.year,
        s.value,
        c.population_weight as weight,
        c.absolute_weight,
        true                 as is_crosswalked,
        s.geodata_id_nuts    as source_nuts_id,
        s.geodata_year       as source_version,
        c.n_hops
    from all_source_data s
    join crosswalk_{{ tv }} c
        on  c.source_nuts_id = s.geodata_id_nuts
        and c.source_version = s.geodata_year
    where s.geodata_year != {{ tv }}

),

-- Sum up (sum) or weighted-average (weighted_avg) merges, see measure_type
-- above; several source rows -> one target row
aggregated_{{ tv }} as (
    select
        dataset_slug,
        target_nuts_id,
        year,
        {{ tv }} as target_version,
        case dataset_slug
        {% for ds in source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        {% endfor %}
            when 'beschaeftigungsquote-nuts3' then
                {% if beschaeftigungsquote_nuts3_measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        end                      as harmonized_value,
        bool_or(is_crosswalked)  as is_crosswalked,
        min(source_version)      as source_version,
        max(n_hops)              as n_hops
    from harmonized_{{ tv }}
    group by dataset_slug, target_nuts_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_nuts_id, year
