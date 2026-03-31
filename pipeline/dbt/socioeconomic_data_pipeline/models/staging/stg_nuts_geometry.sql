{{ config(materialized='view') }}

{% set source_relation = source('loaded_data', 'nuts_geometry') %}

SELECT
    nuts_id,
    levl_code,
    cntr_code as country_code,
    coalesce(nuts_name, name_latn, nuts_id) as region_name,
    year,
    ST_GeomFromText(geometry, 3035) as geometry_3035,
    ST_Transform(ST_GeomFromText(geometry, 3035), 4326) as geometry_wgs84,
    CASE
        WHEN geometry IS NULL THEN false
        WHEN length(trim(geometry)) = 0 THEN false
        ELSE true
    END as has_valid_geometry,
    current_timestamp as loaded_at
FROM {{ source_relation }}
WHERE geometry IS NOT NULL
  AND trim(geometry) != ''
  AND nuts_id IS NOT NULL