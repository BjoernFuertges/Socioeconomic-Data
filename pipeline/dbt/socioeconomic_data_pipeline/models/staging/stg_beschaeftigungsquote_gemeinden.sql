{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_{{ this.name }}_geodata_year on {{ this }} (geodata_id_nuts, year)",
            "create index if not exists idx_{{ this.name }}_nuts_tpo on {{ this }} (geodata_id_nuts text_pattern_ops)"
        ]
    )
}}

{% set source_relation = ref('snap_results_beschaeftigungsquote_gemeinden') %}
{% set source_columns = adapter.get_columns_in_relation(source_relation) | map(attribute='name') | map('lower') | list %}

with source_data as (
    select * from {{ source_relation }}
    where dbt_valid_to is null
),

cleaned as (
    select
        -- Remove _dlt* fields by explicitly selecting only needed columns
        regexp_replace(cast(original_id_1 as text), '\.0+$', '') as original_id_1,
        original_name,
        geodata_name,
        {% if 'geodata_id_nuts' in source_columns %}
        nullif(cast(geodata_id_nuts as text), '') as geodata_id_nuts,
        {% else %}
        null::text as geodata_id_nuts,
        {% endif %}
        coalesce(
            {% if 'geodata_id_lau' in source_columns %}
            nullif(cast(geodata_id_lau as text), ''),
            {% endif %}
            {% if 'geodata_id_ars' in source_columns %}
            nullif(cast(geodata_id_ars as text), '')
            {% else %}
            null::text
            {% endif %}
        ) as geodata_id_lau,
        {% if 'geodata_id_ars' in source_columns %}
        nullif(cast(geodata_id_ars as text), '') as geodata_id_ars,
        {% else %}
        null::text as geodata_id_ars,
        {% endif %}
        mapper,
        parameter,
        coalesce(nullif(cast(geodata_level as text), ''), 'LAU') as geodata_level,
        -- No default: a missing geodata_year (meta.yaml) must fail the not_null test
        -- instead of silently falling back to a vintage.
        geodata_year::integer as geodata_year,

        -- Observation fields
        year::integer as year,
        value::numeric as beschaeftigungsquote_prozent,

        -- Derived observation date: 1 January of the year
        make_date(year::integer, 1, 1) as observation_date,

        -- Data quality flag. Values above 100 are legitimate: the rate is
        -- workplace-based, so commuter hubs and tiny municipalities (few
        -- residents aged 15-64) can exceed 100 %. Only negative values and
        -- extreme outliers are excluded; see is_low_confidence for small bases.
        -- A rate of exactly 0 is invalid: the source uses 0 as a placeholder for
        -- "not defined / missing" (municipality-free areas are 0 in every year,
        -- many municipalities are 0 in single years only), not as a measured
        -- rate. Left valid, such zeros would drag the NUTS-3 means/medians
        -- and the LAU crosswalk values down. There is deliberately no
        -- exception for tiny municipalities.
        case
            when value is null     then false
            when value <= 0        then false
            when value > 1000      then false
            else true
        end as has_valid_value

    from source_data
),

-- One row per municipality key from the newest LAU vintage. The BBSR series
-- is on the 2024 boundary/code level (see meta.yaml geodata_year), so the
-- mapping must not follow the observation year: lau_population.year is the
-- Eurostat vintage, and a same-year join leaves early observation years
-- without a NUTS-3 code and maps later ones onto the wrong vintage.
latest_lau as (
    select distinct on (lau_nat_code)
        lau_nat_code as ars_key,
        nuts_3_code,
        case when has_valid_population and population_count > 0
             then population_count end as population_count
    from {{ ref('stg_lau_population') }}
    where lau_nat_code is not null
    order by lau_nat_code, year desc
),

mapped as (
    select
        c.original_id_1,
        coalesce(c.original_name, '') as original_name,
        coalesce(c.geodata_name, c.original_name, '') as geodata_name,
        coalesce(c.geodata_id_nuts, p.nuts_3_code, '') as geodata_id_nuts,
        coalesce(c.geodata_id_lau, c.original_id_1, '') as geodata_id_lau,
        coalesce(c.geodata_id_ars, c.geodata_id_lau, c.original_id_1, '') as geodata_id_ars,
        coalesce(c.mapper, '') as mapper,
        coalesce(c.parameter, '') as parameter,
        c.geodata_level,
        c.geodata_year,
        c.year,
        c.beschaeftigungsquote_prozent,
        c.observation_date,
        c.has_valid_value,
        -- Small denominator: below {{ var('low_confidence_population', 500) }} inhabitants a handful of people moves the
        -- rate by tens of percentage points. Total population is only a proxy
        -- for the residents aged 15-64 behind the rate. Unknown -> false.
        coalesce(p.population_count < {{ var('low_confidence_population', 500) }}, false) as is_low_confidence
    from cleaned c
    left join latest_lau p
        -- original_id_1 is the BBSR key (7 digits for states 01-09), ars_key the 8-digit AGS
    on p.ars_key = lpad(c.original_id_1, 8, '0')
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['original_id_1', 'year']) }} as beschaeftigungsquote_lau_pk,
        *
    from mapped
)

select * from final
