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

  is_estimated = true     -> the value was converted via at least one crosswalk
                             pair flagged is_estimate (weight_basis 'area', or
                             Gemeinden straddling a region boundary assigned
                             wholly by majority, e.g. Potsdam 2003 or the
                             Sachsen-Anhalt reform 2007): treat as an estimate.
                             Always false for direct (non-crosswalked) values.

  has_valid_value = false -> at least one source value fails the has_valid_value
                             rule of its staging model (e.g. a quota outside
                             0..100); consumers such as GET /metrics skip it

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
    - 'weighted_avg_employees' with an 'employee_slug' (auszubildende: per
                       1,000 SV-Beschaeftigte -> employee_slug
                       'sv_beschaeftigte'): like 'weighted_avg', but the
                       weight of each source piece is the employee count of
                       the source region (employee_slug, harmonized onto the
                       source's own boundary version, same year) times the
                       crosswalk share population_weight:
                       harmonized_value = sum(value_i * emp_i * pw_i)
                                          / sum(emp_i * pw_i)
                       This keeps rate x count consistent: the harmonized rate
                       times the harmonized (summed) employee count equals the
                       sum of the absolute source counts (derived metric
                       auszubildende-absolut).
                       FALLBACK: if any contributing source piece has no valid
                       (> 0, has_valid_value) employee count for that year
                       (sv_beschaeftigte only covers 2007-2025, auszubildende
                       starts 1995), the whole target row falls back to the
                       population weight of 'weighted_avg' above - the two
                       weights are never mixed within one row, and a missing
                       weight never yields 0 or NULL.
*/

{% set target_versions = var('nuts_versions') %}

-- Slugs deliberately match the RouteSlugs of the generated Kreis API
-- endpoints 1:1 so the backend can join both sides without a mapping table.
-- measure_type is derived from the column descriptions/meta.yaml per
-- dataset (count = 'sum', ratio/average = 'weighted_avg').
-- Note: erwerbstaetige_pro_1000_einwohner is, despite its slug (kept: public
-- API route), the absolute number of employed persons in thousands - a sum.
-- 'weighted_avg_employees' needs an 'employee_slug' here (the employee-count
-- dataset used as weight, see header); arbeitsvolumen/bruttoverdienst are
-- kept as 'weighted_avg' in this list (population-weighted crosswalk) and are
-- only employee-weighted in the aggregation marts.
-- Each entry reads its staging model (staging, value_col = the renamed value
-- column there) - NOT loaded_data directly: the has_valid_value rule lives
-- only in stg_<slug>_kreise.sql and is taken over as is, and the rows are the
-- current snapshot state (snap_results_<slug>_kreise, dbt_valid_to is null),
-- i.e. the same rows the generated Kreis API endpoints serve. Collisions
-- (two raw rows on one (geodata_id_nuts, year)) are caught before that by the
-- unique_combination_of_columns test on each loaded_data.results_*_kreise
-- source (models/sources.yml): the snapshot keeps only one of them.
{% set source_datasets = [
    {'slug': 'arbeitslose',        'staging': 'stg_arbeitslose_kreise', 'value_col': 'arbeitslose', 'measure_type': 'sum'},
    {'slug': 'arbeitsvolumen',     'staging': 'stg_arbeitsvolumen_kreise', 'value_col': 'arbeitsvolumen', 'measure_type': 'weighted_avg'},
    {'slug': 'auszubildende',      'staging': 'stg_auszubildende_kreise', 'value_col': 'auszubildende', 'measure_type': 'weighted_avg_employees', 'employee_slug': 'sv_beschaeftigte'},
    {'slug': 'bevoelkerung',       'staging': 'stg_bevoelkerung_kreise', 'value_col': 'bevoelkerung', 'measure_type': 'sum'},
    {'slug': 'bruttoverdienst',    'staging': 'stg_bruttoverdienst_kreise', 'value_col': 'bruttoverdienst', 'measure_type': 'weighted_avg'},
    {'slug': 'durchschnittsalter', 'staging': 'stg_durchschnittsalter_kreise', 'value_col': 'durchschnittsalter', 'measure_type': 'weighted_avg'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'staging': 'stg_erwerbstaetige_pro_1000_einwohner_kreise', 'value_col': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'sum'},
    {'slug': 'kinderarmut',        'staging': 'stg_kinderarmut_kreise', 'value_col': 'kinderarmut', 'measure_type': 'weighted_avg'},
    {'slug': 'medianeinkommen',    'staging': 'stg_medianeinkommen_kreise', 'value_col': 'medianeinkommen', 'measure_type': 'weighted_avg'},
    {'slug': 'schuldnerquote',     'staging': 'stg_schuldnerquote_kreise', 'value_col': 'schuldnerquote_prozent', 'measure_type': 'weighted_avg'},
    {'slug': 'sgb_ii_quote',       'staging': 'stg_sgb_ii_quote_kreise', 'value_col': 'sgb_ii_quote_prozent', 'measure_type': 'weighted_avg'},
    {'slug': 'studierende',        'staging': 'stg_studierende_kreise', 'value_col': 'studierende', 'measure_type': 'weighted_avg'},
    {'slug': 'sv_beschaeftigte',   'staging': 'stg_sv_beschaeftigte_kreise', 'value_col': 'sv_beschaeftigte', 'measure_type': 'sum'},
    {'slug': 'wohngeldhaushalte',  'staging': 'stg_wohngeldhaushalte_kreise', 'value_col': 'wohngeldhaushalte_je_1000', 'measure_type': 'weighted_avg'},
] %}
{% set beschaeftigungsquote_nuts3_measure_type = 'weighted_avg' %}
{% set employee_weighted = [] %}
{% for ds in source_datasets %}{% if ds.employee_slug %}{% do employee_weighted.append(ds) %}{% endif %}{% endfor %}
{% set employee_slugs = employee_weighted | map(attribute='employee_slug') | unique | list %}

with all_source_data as (

    -- Union of all Kreis staging models. One row per (geodata_id_nuts, year)
    -- each (snapshot unique_key, tested via the staging *_pk unique test), so
    -- no DISTINCT is needed; has_valid_value is the staging flag unchanged.
    {% for ds in source_datasets %}
    select
        '{{ ds.slug }}'              as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer        as geodata_year,
        year::integer                as year,
        {{ ds.value_col }}::numeric  as value,
        has_valid_value
    from {{ ref(ds.staging) }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and {{ ds.value_col }} is not null
    union all
    {% endfor %}

    -- beschaeftigungsquote-nuts3 has no Kreis snapshot: it is aggregated to
    -- NUTS-3 (2024) from the Gemeinde (LAU) values via population (see
    -- stg_beschaeftigungsquote_nuts3.sql / int_beschaeftigungsquote_nuts3.sql).
    select
        'beschaeftigungsquote-nuts3' as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer        as geodata_year,
        year::integer                as year,
        beschaeftigungsquote_prozent::numeric as value,
        has_valid_value
    from {{ ref('stg_beschaeftigungsquote_nuts3') }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and beschaeftigungsquote_prozent is not null

),

{% if employee_weighted %}
-- Employee counts (employee_slug datasets, e.g. sv_beschaeftigte) per
-- (region, boundary version, year) on EVERY version: direct where the
-- employee data is already in that version, otherwise crosswalked as a count
-- (value * population_weight, summed). Only valid (> 0, has_valid_value)
-- counts qualify: one invalid contributing piece makes the whole cell NULL,
-- which triggers the population fallback below instead of a silent 0.
employee_weight_by_version as (
    select
        weight_slug,
        nuts_id,
        version,
        year,
        case when bool_and(is_valid) then sum(w) end as employees
    from (
        select
            s.dataset_slug                       as weight_slug,
            s.geodata_id_nuts                    as nuts_id,
            s.geodata_year                       as version,
            s.year,
            s.value                              as w,
            (s.has_valid_value and s.value > 0)  as is_valid
        from all_source_data s
        where s.dataset_slug in ('{{ employee_slugs | join("', '") }}')

        union all

        select
            s.dataset_slug,
            c.target_nuts_id,
            c.target_version,
            s.year,
            s.value * c.population_weight,
            (s.has_valid_value and s.value > 0 and c.population_weight is not null)
        from all_source_data s
        join {{ ref('mart_nuts_version_crosswalk') }} c
            on  c.source_nuts_id = s.geodata_id_nuts
            and c.source_version = s.geodata_year
        where s.dataset_slug in ('{{ employee_slugs | join("', '") }}')
    ) pieces
    group by weight_slug, nuts_id, version, year
),
{% endif %}

-- Source rows plus, for employee-weighted datasets, the employee count of the
-- source region in its own boundary version and year (NULL if missing).
source_data as (
    select
        s.*,
        {% if employee_weighted %}e.employees{% else %}null::numeric{% endif %} as source_employees
    from all_source_data s
    {% if employee_weighted %}
    left join (
        values
        {% for ds in employee_weighted %}
            ('{{ ds.slug }}', '{{ ds.employee_slug }}'){% if not loop.last %},{% endif %}
        {% endfor %}
    ) as m (dataset_slug, weight_slug)
        on m.dataset_slug = s.dataset_slug
    left join employee_weight_by_version e
        on  e.weight_slug = m.weight_slug
        and e.nuts_id     = s.geodata_id_nuts
        and e.version     = s.geodata_year
        and e.year        = s.year
    {% endif %}
),

-- Inhabitants per (region, boundary version) from the crosswalk, so the
-- identity rows below carry absolute_weight in the same unit (inhabitants)
-- as the crosswalked rows - both may meet in one weighted_avg group.
region_population as (
    select
        source_nuts_id,
        source_version,
        max(source_population) as population
    from {{ ref('mart_nuts_version_crosswalk') }}
    group by source_nuts_id, source_version
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
        -- inhabitants of the region (unit of the crosswalk's absolute_weight);
        -- 1.0 only if the region has no LAU population at all, where every
        -- crosswalked piece into it weighs 0 anyway
        coalesce(nullif(rp.population, 0), 1.0)::numeric as absolute_weight,
        false               as is_crosswalked,
        false               as is_estimated,
        s.geodata_id_nuts   as source_nuts_id,
        s.geodata_year      as source_version,
        0                   as n_hops,
        s.has_valid_value,
        s.source_employees  as emp_weight
    from source_data s
    left join region_population rp
        on  rp.source_nuts_id = s.geodata_id_nuts
        and rp.source_version = s.geodata_year
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
        -- weight > 0: a piece that contributes nothing cannot make it an estimate
        (c.is_estimate and c.population_weight > 0) as is_estimated,
        s.geodata_id_nuts    as source_nuts_id,
        s.geodata_year       as source_version,
        c.n_hops,
        s.has_valid_value,
        -- employees of the source piece going to this target region
        s.source_employees * c.population_weight as emp_weight
    from source_data s
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
                {% if ds.measure_type == 'weighted_avg_employees' and ds.employee_slug %}
                case
                    -- every contributing piece has an employee weight
                    when count(emp_weight) = count(*) and sum(emp_weight) > 0
                        then sum(value * emp_weight) / sum(emp_weight)
                    -- fallback: population weight (see header)
                    else sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                end
                {% elif ds.measure_type in ('weighted_avg', 'weighted_avg_employees') %}
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
        bool_or(is_estimated)    as is_estimated,
        min(source_version)      as source_version,
        max(n_hops)              as n_hops,
        -- false as soon as one contributing source value fails the staging
        -- has_valid_value rule: the target value is then contaminated.
        bool_and(has_valid_value) as has_valid_value
    from harmonized_{{ tv }}
    group by dataset_slug, target_nuts_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_nuts_id, year
