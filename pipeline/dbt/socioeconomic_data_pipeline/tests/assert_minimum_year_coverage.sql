-- Test to ensure we have reasonable year coverage
-- This test will fail if any LAU has data for less than 3 years

SELECT
  gisco_id,
  lau_name_national,
  country_code,
  total_years_available
FROM {{ ref('lau_integrated') }}
WHERE total_years_available < 3
GROUP BY gisco_id, lau_name_national, country_code, total_years_available