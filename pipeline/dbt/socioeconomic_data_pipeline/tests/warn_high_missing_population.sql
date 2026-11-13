{{ config(severity='warn') }}

-- Warns (does not fail the build) if more than 10% of LAUs are missing
-- population data in any year; returns one row per affected year.

SELECT
  year,
  COUNT(*) as total_laus,
  COUNT(CASE WHEN missing_population_data = true THEN 1 END) as missing_pop,
  ROUND(
    100.0 * COUNT(CASE WHEN missing_population_data = true THEN 1 END) / COUNT(*),
    2
  ) as missing_percentage
FROM {{ ref('lau_integrated') }}
GROUP BY year
HAVING 100.0 * COUNT(CASE WHEN missing_population_data = true THEN 1 END) / COUNT(*) > 10
