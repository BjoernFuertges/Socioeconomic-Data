{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts_geometry_gist on {{ this }} using gist (geometry_3035)",
            "create index if not exists idx_mart_nuts_geometry_levl_valid on {{ this }} (levl_code, has_valid_geometry)"
        ]
    )
}}

with source_data as (
    select
        nuts_id,
        levl_code,
        country_code,
        region_name,
        year,
        geometry_3035,
        ST_AsGeoJSON(geometry_wgs84)::json as geometry_wgs84,
        has_valid_geometry
    from {{ ref('stg_nuts_geometry') }}
)

select *
from source_data
order by nuts_id, year desc