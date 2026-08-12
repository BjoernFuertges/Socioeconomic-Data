{% macro render_nuts3_fill_gaps_view(stage_model, dataset_slug, value_column) %}
{% set target_versions = [2003, 2006, 2010, 2013, 2016, 2021, 2024] %}

with timeseries as (
    select
        nuts3_id,
        target_version,
        year,
        value,
        is_interpolated,
        value_linear,
        value_peer_trend,
        n_peer_regions
    from {{ ref('mart_nuts3_timeseries_interpolated') }}
    where dataset_slug = '{{ dataset_slug }}'
),

source_rows as (
    select
        src.geodata_id_nuts,
        src.year,
        src.original_id_1,
        src.original_name,
        src.geodata_name,
        nullif(to_jsonb(src) ->> 'geodata_id_lau', '') as geodata_id_lau,
        coalesce(cast(src.geodata_id_ars as text), '') as geodata_id_ars,
        src.mapper,
        src.parameter,
        src.geodata_level,
        src.geodata_year,
        src.observation_date,
        src.has_valid_value
    from {{ ref(stage_model) }} src
),

-- Regionsnamen je NUTS-Zielversion - eine Region kann in unterschiedlichen
-- Versionen unterschiedliche valid_from/valid_to-Perioden (und damit ggf.
-- einen anderen Namen) haben, daher Join über beides statt nur nuts_id.
target_regions as (
    select
        nuts_id,
        region_name,
        valid_from,
        valid_to
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code = 3
)

select
    -- original_id_1 ist in den Kreis-Staging-Modellen double precision
    -- (numerische Kennziffer, führende Nullen bereits beim Einlesen
    -- verloren - z.B. 1001 statt "01001"), ts.nuts3_id dagegen text.
    -- Cast nötig, damit COALESCE beide Typen vereinen kann.
    coalesce(src.original_id_1::text, ts.nuts3_id) as original_id_1,
    coalesce(src.original_name, geo.region_name, ts.nuts3_id) as original_name,
    coalesce(src.geodata_name, geo.region_name, ts.nuts3_id) as geodata_name,
    ts.nuts3_id as geodata_id_nuts,
    coalesce(src.geodata_id_lau, '') as geodata_id_lau,
    coalesce(src.geodata_id_ars, '') as geodata_id_ars,
    coalesce(src.mapper, 'harmonized_fill_gaps') as mapper,
    coalesce(src.parameter, 'geodata_id_nuts') as parameter,
    coalesce(src.geodata_level, 'NUTS_3') as geodata_level,
    coalesce(src.geodata_year, ts.target_version) as geodata_year,
    -- NUTS-Zielversion dieser Zeile - macht die View konsistent mit dem
    -- nuts_version-Parameter von /{slug}/timeseries (beide lesen jetzt
    -- dieselben, mehrere Zielversionen führenden mart_nuts3_*-Modelle,
    -- statt die fill_gaps-Seite auf eine dbt-Build-Zeit-Version zu fixieren).
    ts.target_version as nuts_version,
    ts.year,
    coalesce(src.observation_date, make_date(ts.year, 1, 1)) as observation_date,
    coalesce(src.has_valid_value, true) as has_valid_value,
    ts.value as {{ value_column }},
    ts.is_interpolated,
    ts.value_linear,
    ts.value_peer_trend,
    ts.n_peer_regions
from timeseries ts
left join source_rows src
    on  src.geodata_id_nuts = ts.nuts3_id
    and src.year = ts.year
left join target_regions geo
    on  geo.nuts_id = ts.nuts3_id
    and geo.valid_from <= ts.target_version
    and (geo.valid_to is null or geo.valid_to >= ts.target_version)
order by geodata_id_nuts, nuts_version, year
{% endmacro %}
