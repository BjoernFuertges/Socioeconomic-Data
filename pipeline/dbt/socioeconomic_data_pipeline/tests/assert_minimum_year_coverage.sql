-- Year coverage sanity check for lau_integrated.
--
-- total_years_available counts the geometry vintage years (2011-2024) of a
-- gisco_id. Short lifespans are legitimate when a Gemeinde was dissolved
-- (merged, e.g. only in the 2011 vintage) or newly formed (e.g. first
-- appears in 2024), so such units are NOT flagged.
--
-- What this test catches: a LAU that is still present in the latest vintage
-- and first appeared at least 3 vintages before it, yet has fewer than 3
-- years - i.e. vintages that were lost by a join/historization bug.
-- Note: total_years_available is constant per gisco_id, so min(year) is its
-- first vintage.

WITH bounds AS (
  SELECT max(year) AS max_year FROM {{ ref('lau_integrated') }}
),

per_lau AS (
  SELECT
    gisco_id,
    max(lau_name_national) AS lau_name_national,
    max(country_code) AS country_code,
    max(total_years_available) AS total_years_available,
    min(year) AS first_year,
    max(year) AS last_year
  FROM {{ ref('lau_integrated') }}
  GROUP BY gisco_id
)

SELECT
  p.gisco_id,
  p.lau_name_national,
  p.country_code,
  p.total_years_available,
  p.first_year,
  p.last_year
FROM per_lau p
CROSS JOIN bounds b
WHERE p.total_years_available < 3
  AND p.last_year = b.max_year
  AND p.first_year <= b.max_year - 2
