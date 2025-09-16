-- Macro to generate a summary report of missing data by country and year
{% macro missing_data_summary(country_codes=none, years=none) %}

WITH filtered_data AS (
  SELECT *
  FROM {{ ref('lau_integrated') }}
  WHERE 1=1
  {% if country_codes %}
    AND cntr_code IN ({{ country_codes | join("','") | replace("'","'") }})
  {% endif %}
  {% if years %}
    AND year IN ({{ years | join(",") }})
  {% endif %}
)

SELECT 
  cntr_code,
  year,
  COUNT(*) as total_laus,
  COUNT(CASE WHEN missing_population_data = false THEN 1 END) as with_population,
  COUNT(CASE WHEN missing_population_data = true THEN 1 END) as missing_population,
  ROUND(
    100.0 * COUNT(CASE WHEN missing_population_data = false THEN 1 END) / COUNT(*), 
    2
  ) as coverage_percent
FROM filtered_data
GROUP BY cntr_code, year
ORDER BY cntr_code, year

{% endmacro %}