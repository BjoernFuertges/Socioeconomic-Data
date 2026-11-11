{{ config(materialized='view') }}

{% set source_relation = source('loaded_data', 'agenturbezirk_geometry') %}

-- Source is already in WGS84 (unlike nuts/lau geometry, which arrive in
-- EPSG:3035), so no ST_Transform is needed here.
SELECT
    ba_schl,
    name,
    id_dwh,
    valid_from::date as valid_from,
    valid_to::date as valid_to,
    ST_GeomFromText(geometry, 4326) as geometry_wgs84,
    CASE
        WHEN geometry IS NULL THEN false
        WHEN length(trim(geometry)) = 0 THEN false
        ELSE true
    END as has_valid_geometry,
    current_timestamp as loaded_at
FROM {{ source_relation }}
WHERE geometry IS NOT NULL
  AND trim(geometry) != ''
  AND ba_schl IS NOT NULL
