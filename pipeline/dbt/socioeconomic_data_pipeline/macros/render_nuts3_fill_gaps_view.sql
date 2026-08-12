{% macro render_nuts3_fill_gaps_view(stage_model, dataset_slug, value_column) %}
{% set target_version = var('target_nuts_version', 2024) %}

with timeseries as (
    select
        nuts3_id,
        year,
        value,
        is_interpolated,
        value_linear,
        value_peer_trend,
        n_peer_regions
    from {{ ref('mart_nuts3_timeseries_interpolated') }}
    where dataset_slug = '{{ dataset_slug }}'
      -- mart_nuts3_timeseries_interpolated führt mehrere NUTS-3-Zielversionen
      -- parallel (target_version-Spalte); ohne diesen Filter würde die View
      -- Zeilen aus allen Zielversionen mischen (mehrfache, unterschiedliche
      -- Werte je nuts3_id/year).
      and target_version = {{ target_version }}
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

target_regions as (
    select
        nuts_id,
        region_name
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code = 3
      and valid_from <= {{ target_version }}
      and (valid_to is null or valid_to >= {{ target_version }})
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
    coalesce(src.geodata_year, {{ target_version }}) as geodata_year,
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
    on geo.nuts_id = ts.nuts3_id
order by geodata_id_nuts, year
{% endmacro %}