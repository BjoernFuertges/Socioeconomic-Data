{{
  config(
    materialized='table',
    post_hook=[
      "create index if not exists idx_lau_integrated_gisco_year on {{ this }} (gisco_id, year)",
      "create index if not exists idx_lau_integrated_country_year on {{ this }} (country_code, year)",
      "create index if not exists idx_lau_integrated_nuts3_tpo on {{ this }} (nuts_3_code text_pattern_ops) where nuts_3_code is not null",
      "create index if not exists idx_lau_integrated_gist on {{ this }} using gist (geometry_3035) where has_valid_geometry"
    ]
  )
}}

-- Main integrated model combining LAU geometry and population data
-- Joins on GISCO_ID (LAU code) and includes all years
WITH geometry_data AS (
  SELECT * FROM {{ ref('stg_lau_geometry') }}
),

population_data AS (
  SELECT * FROM {{ ref('stg_lau_population') }}
),

integrated_data AS (
  SELECT 
    g.gisco_id,
    g.year,
    g.lau_name,
    g.country_code,
    g.lau_id,
    g.geometry_3035,
    ST_AsGeoJSON(g.geometry_wgs84)::json as geometry_wgs84,
    g.has_valid_geometry,
    
    -- Population data
    p.lau_name_national,
    p.lau_name_latin,
    p.nuts_3_code,
    p.lau_nat_code,
    p.population_count,
    p.country,
    p.has_valid_population,
    
    -- Data quality indicators
    CASE 
      WHEN p.gisco_id IS NULL THEN true 
      ELSE false 
    END as missing_population_data,
    
    CASE 
      WHEN g.year = p.year THEN true 
      ELSE false 
    END as year_match

  FROM geometry_data g
  LEFT JOIN population_data p 
    ON g.gisco_id = p.gisco_id 
    AND g.year = p.year
),

-- Add summary statistics
final_data AS (
  SELECT 
    *,
    COUNT(*) OVER (PARTITION BY gisco_id) as total_years_available,
    COUNT(population_count) OVER (PARTITION BY gisco_id) as years_with_population
  FROM integrated_data
)

SELECT * FROM final_data
ORDER BY gisco_id, year