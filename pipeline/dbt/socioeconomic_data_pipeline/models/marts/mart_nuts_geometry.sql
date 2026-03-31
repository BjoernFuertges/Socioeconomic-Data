{{
    config(
        materialized='table',
        schema='transformed_data'
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