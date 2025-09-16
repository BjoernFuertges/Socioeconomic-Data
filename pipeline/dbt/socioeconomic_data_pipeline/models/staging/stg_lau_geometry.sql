{{ config(materialized='view') }}

-- Staging model for LAU geometry data
-- Standardizes column names and data types for geometric data from DLT

SELECT 
    gisco_id,
    cntr_code as country_code,
    lau_id,
    lau_name,
    year,
    -- Convert geometry WKT string to PostGIS geometry type
    ST_GeomFromText(geometry, 3035) as geometry_3035,
    -- Also create WGS84 version for compatibility
    ST_Transform(ST_GeomFromText(geometry, 3035), 4326) as geometry_wgs84,
    -- Add data quality flags
    CASE 
        WHEN geometry IS NULL THEN false
        WHEN length(trim(geometry)) = 0 THEN false
        ELSE true 
    END as has_valid_geometry,
    -- Extract additional metadata
    current_timestamp as loaded_at

FROM {{ source('loaded_data', 'lau_geometry') }}
WHERE geometry IS NOT NULL 
    AND trim(geometry) != ''
    AND gisco_id IS NOT NULL