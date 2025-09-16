{{ config(materialized='view') }}

-- Staging model for LAU population data
-- Standardizes column names and data types for population statistics from DLT

SELECT 
    gisco_id,
    nuts_3_code,
    lau_nat_code,
    lau_name_national,
    lau_name_latin,
    -- Convert population to numeric, handling potential string values
    CASE 
        WHEN population ~ '^[0-9]*\.?[0-9]+$' THEN population::numeric
        ELSE NULL 
    END as population_count,
    country,
    year,
    -- Add data quality flags
    CASE 
        WHEN population IS NULL THEN false
        WHEN population = '' THEN false
        WHEN NOT (population ~ '^[0-9]*\.?[0-9]+$') THEN false
        ELSE true 
    END as has_valid_population,
    -- Extract additional metadata
    current_timestamp as loaded_at

FROM {{ source('loaded_data', 'lau_population') }}
WHERE gisco_id IS NOT NULL