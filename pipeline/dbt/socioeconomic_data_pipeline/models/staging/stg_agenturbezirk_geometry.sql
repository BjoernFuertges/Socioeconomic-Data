{{ config(materialized='view') }}

{% set source_relation = source('loaded_data', 'agenturbezirk_geometry') %}

-- Der dlt-Loader (load_agenturbezirke_geodata.py) reprojiziert bereits nach
-- WGS84, anders als nuts_geometry/lau_geometry (dort liegt das Rohmaterial
-- in EPSG:3035 vor) - deshalb hier nur ein einfaches ST_GeomFromText ohne
-- ST_Transform.
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
