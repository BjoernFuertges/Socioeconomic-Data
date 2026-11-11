{{ config(materialized='view') }}

-- Staging model for LAU population data
-- Standardizes column names and data types for population statistics from DLT

SELECT
    -- Deutsche GISCO-IDs haben 8 Stellen (DE_ + AGS, z.B. DE_03251041), die
    -- Eurostat-Korrespondenztabellen liefern fuer Bundeslaender 01-09 aber
    -- 7 Stellen ohne fuehrende Null (DE_3251041). Ohne Auffuellen greift der
    -- Join auf die LAU-Geometrie (gleiche gisco_id) nur fuer Bundeslaender
    -- 10-16 - die Bevoelkerungsgewichtung der NUTS-Crosswalks (siehe
    -- int_lau_nuts3_population_weight) lief sonst nur dort.
    CASE
        WHEN gisco_id ~ '^DE_[0-9]{7}$' THEN 'DE_0' || substr(gisco_id, 4)
        ELSE gisco_id
    END as gisco_id,
    nuts_3_code,
    lau_nat_code,
    lau_name_national,
    lau_name_latin,
    -- Convert population to bigint, handling potential float-string values like "13351.0"
    CASE 
        WHEN population ~ '^[0-9]*\.?[0-9]+$' THEN population::numeric::bigint
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