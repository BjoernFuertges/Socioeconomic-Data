-- Test to warn about LAUs with missing population data
-- This test will fail if more than 10% of LAUs are missing population data in any year

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
HAVING missing_percentage > 10