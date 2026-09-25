{% macro render_lau_fill_gaps_view(stage_model, dataset_slug, value_column) %}

with timeseries as (
    select
        gisco_id,
        target_version,
        year,
        value,
        is_interpolated,
        value_linear,
        value_peer_trend,
        n_peer_regions
    from {{ ref('mart_lau_timeseries_interpolated') }}
    where dataset_slug = '{{ dataset_slug }}'
),

source_rows as (
    select
        src.geodata_id_nuts,
        src.geodata_id_lau,
        src.geodata_id_ars,
        src.year,
        src.original_id_1,
        src.original_name,
        src.geodata_name,
        src.mapper,
        src.parameter,
        src.geodata_level,
        src.geodata_year,
        src.observation_date,
        src.has_valid_value
    from {{ ref(stage_model) }} src
),

-- gisco_id -> amtlicher Gemeindeschlüssel, um Quellzeilen (die keine
-- gisco_id führen) auf die Zeitreihe zurückjoinen zu können.
gisco_lookup as (
    select distinct on (lau_nat_code)
        regexp_replace(cast(lau_nat_code as text), '\.0+$', '') as ars_code,
        gisco_id
    from {{ ref('stg_lau_population') }}
    where lau_nat_code is not null and gisco_id is not null
    order by lau_nat_code, year desc
),

-- Regionsnamen je LAU-Zielversion.
target_regions as (
    select
        gisco_id,
        lau_name,
        valid_from,
        valid_to
    from {{ ref('mart_lau_geometry_historized') }}
)

select
    coalesce(src.original_id_1::text, ts.gisco_id) as original_id_1,
    coalesce(src.original_name, geo.lau_name, ts.gisco_id) as original_name,
    coalesce(src.geodata_name, geo.lau_name, ts.gisco_id) as geodata_name,
    coalesce(src.geodata_id_nuts, '') as geodata_id_nuts,
    ts.gisco_id as geodata_id_lau,
    coalesce(src.geodata_id_ars, '') as geodata_id_ars,
    coalesce(src.mapper, 'harmonized_fill_gaps') as mapper,
    coalesce(src.parameter, 'gisco_id') as parameter,
    coalesce(src.geodata_level, 'LAU') as geodata_level,
    coalesce(src.geodata_year, ts.target_version) as geodata_year,
    -- LAU-Zielversion dieser Zeile - macht die View konsistent mit dem
    -- lau_version-Parameter von /{slug}/timeseries (analog nuts_version bei
    -- den Kreis-Datensätzen).
    ts.target_version as lau_version,
    ts.year,
    coalesce(src.observation_date, make_date(ts.year, 1, 1)) as observation_date,
    coalesce(src.has_valid_value, true) as has_valid_value,
    ts.value as {{ value_column }},
    ts.is_interpolated,
    ts.value_linear,
    ts.value_peer_trend,
    ts.n_peer_regions
from timeseries ts
left join gisco_lookup gl
    on gl.gisco_id = ts.gisco_id
left join source_rows src
    on  coalesce(src.geodata_id_ars, src.geodata_id_lau) = gl.ars_code
    and src.year = ts.year
left join target_regions geo
    on  geo.gisco_id = ts.gisco_id
    and geo.valid_from <= ts.target_version
    and (geo.valid_to is null or geo.valid_to >= ts.target_version)
order by geodata_id_lau, lau_version, year
{% endmacro %}
