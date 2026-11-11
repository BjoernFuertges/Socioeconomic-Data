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
        regexp_replace(cast(original_id_1 as text), '\\.0+$', '') as original_id_1,
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
        coalesce(geodata_year::integer, 2024) as geodata_year,

        -- Observation fields
        year::integer as year,
        value::numeric as beschaeftigungsquote_prozent,

        -- Derived observation date: 1 January of the year
        make_date(year::integer, 1, 1) as observation_date,

        -- Data quality flag: percentage must be 0-100
        case
            when value is null     then false
            when value < 0         then false
            when value > 100       then false
            else true
        end as has_valid_value

    from source_data
),

mapped as (
    select
        c.original_id_1,
        coalesce(c.original_name, '') as original_name,
        coalesce(c.geodata_name, c.original_name, '') as geodata_name,
        coalesce(c.geodata_id_nuts, p.nuts_3_code, g.nuts_3, '') as geodata_id_nuts,
        coalesce(c.geodata_id_lau, regexp_replace(cast(p.lau_nat_code as text), '\\.0+$', ''), c.original_id_1, '') as geodata_id_lau,
        coalesce(c.geodata_id_ars, c.geodata_id_lau, regexp_replace(cast(p.lau_nat_code as text), '\\.0+$', ''), c.original_id_1, '') as geodata_id_ars,
        coalesce(c.mapper, '') as mapper,
        coalesce(c.parameter, '') as parameter,
        c.geodata_level,
        c.geodata_year,
        c.year,
        c.beschaeftigungsquote_prozent,
        c.observation_date,
        c.has_valid_value
    from cleaned c
    left join {{ ref('stg_lau_population') }} p
        on regexp_replace(cast(p.lau_nat_code as text), '\\.0+$', '') = c.original_id_1
       and p.year = c.year
    left join {{ ref('stg_lau_geometry') }} g
        on lower(trim(g.lau_name)) = lower(trim(c.geodata_name))
       and g.year = c.year
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['original_id_1', 'year']) }} as beschaeftigungsquote_lau_pk,
        *
    from mapped
)

select * from final
