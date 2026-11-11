{{ config(materialized='view') }}

-- Staging model for LAU population data

SELECT
    -- Eurostat delivers German GISCO IDs for states 01-09 without the leading
    -- zero (DE_3251041 instead of DE_03251041). Pad them so they join to the
    -- LAU geometry; otherwise the population weighting only works for states 10-16.
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