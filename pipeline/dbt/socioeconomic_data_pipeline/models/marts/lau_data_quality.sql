{{
  config(
    materialized='table'
  )
}}

-- Data quality metrics for LAU integration
WITH integration_summary AS (
  SELECT 
    year,
    COUNT(*) as total_lau_records,
    COUNT(CASE WHEN missing_population_data = false THEN 1 END) as with_population,
    COUNT(CASE WHEN missing_population_data = true THEN 1 END) as missing_population,
    ROUND(
      100.0 * COUNT(CASE WHEN missing_population_data = false THEN 1 END) / COUNT(*), 
      2
    ) as population_coverage_percent
  FROM {{ ref('lau_integrated') }}
  GROUP BY year
),

missing_data_details AS (
  SELECT 
    year,
    gisco_id,
    lau_name_national,
    country_code,
    'Missing population data' as issue_type
  FROM {{ ref('lau_integrated') }}
  WHERE missing_population_data = true
),

year_mismatch_details AS (
  SELECT 
    year,
    gisco_id,
    lau_name_national,
    country_code,
    'Year mismatch between geo and population' as issue_type
  FROM {{ ref('lau_integrated') }}
  WHERE year_match = false AND missing_population_data = false
)

SELECT 
  'summary' as report_type,
  year,
  NULL as gisco_id,
  NULL as lau_name_national,
  NULL as country_code,
  NULL as issue_type,
  total_lau_records,
  with_population,
  missing_population,
  population_coverage_percent
FROM integration_summary

UNION ALL

SELECT 
  'missing_data' as report_type,
  year,
  gisco_id,
  lau_name_national,
  country_code,
  issue_type,
  NULL as total_lau_records,
  NULL as with_population,
  NULL as missing_population,
  NULL as population_coverage_percent
FROM missing_data_details

UNION ALL

SELECT 
  'year_mismatch' as report_type,
  year,
  gisco_id,
  lau_name_national,
  country_code,
  issue_type,
  NULL as total_lau_records,
  NULL as with_population,
  NULL as missing_population,
  NULL as population_coverage_percent
FROM year_mismatch_details

ORDER BY report_type, year, gisco_id