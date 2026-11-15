{# "create extension if not exists pg_trgm": needed locally (superuser); in prod pre-created by
   backend/scripts/ensure_pipeline_role.sql, so it is a no-op for the non-superuser pipeline_rw. #}
{{
  config(
    materialized='table',
    post_hook=[
      "create index if not exists idx_lau_integrated_gisco_year on {{ this }} (gisco_id, year)",
      "create index if not exists idx_lau_integrated_country_year on {{ this }} (country_code, year)",
      "create index if not exists idx_lau_integrated_nuts3_tpo on {{ this }} (nuts_3_code text_pattern_ops) where nuts_3_code is not null",
      "create index if not exists idx_lau_integrated_nuts3_id_tpo on {{ this }} (nuts3_id text_pattern_ops) where nuts3_id is not null",
      "create index if not exists idx_lau_integrated_gist on {{ this }} using gist (geometry_3035) where has_valid_geometry",
      "create extension if not exists pg_trgm",
      "create index if not exists idx_lau_integrated_lau_name_trgm on {{ this }} using gin (lau_name gin_trgm_ops)"
    ]
  )
}}

-- Main integrated model combining LAU geometry and population data
-- Joins on GISCO_ID (LAU code) and includes all years
--
--
-- The geometry comes from the historized mart (mart_lau_geometry_historized),
-- which holds one row per geometry period. For the per-year API granularity
-- the periods are expanded back to single vintage years
-- (valid_from .. max_vintage_year) via generate_series. LAU vintages exist
-- for every year without gaps, so this equals the original geometry years.
WITH geometry_data AS (
  SELECT
    g.gisco_id,
    gs.year,
    g.lau_name,
    g.country_code,
    g.lau_id,
    g.nuts_3,
    g.geometry_3035,
    g.geometry_wgs84,
    g.has_valid_geometry,
    -- Spatially derived NUTS-3 parent of this Gemeinde (fallback if neither
    -- the population nor the geometry source declares a NUTS-3 code - see
    -- int_lau_nuts3_spatial_link.sql). Joined on gisco_id only (not
    -- valid_from): for performance, int_lau_nuts3_spatial_link has one row
    -- per Gemeinde (representative geometry), not per geometry period.
    link.nuts_id as spatial_nuts3_id
  FROM {{ ref('mart_lau_geometry_historized') }} g
  LEFT JOIN {{ ref('int_lau_nuts3_spatial_link') }} link
    ON g.gisco_id = link.gisco_id
  CROSS JOIN LATERAL generate_series(g.valid_from, g.max_vintage_year) AS gs(year)
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
    -- geometry_wgs84 is already GeoJSON in the historized mart
    g.geometry_wgs84,
    g.has_valid_geometry,

    -- Population data
    p.lau_name_national,
    p.lau_name_latin,
    p.nuts_3_code,
    p.lau_nat_code,
    p.population_count,
    p.country,
    p.has_valid_population,

    -- Canonical NUTS-3 parent: the declared source columns (population or
    -- geometry source) take precedence, otherwise the spatial fallback from
    -- int_lau_nuts3_spatial_link. nuts3_link_source shows where the value
    -- comes from (matters for the GUI - a 'spatial_estimate' assignment is
    -- not an official statement).
    coalesce(nullif(p.nuts_3_code, ''), nullif(g.nuts_3, ''), g.spatial_nuts3_id) as nuts3_id,
    case
      when nullif(p.nuts_3_code, '') is not null then 'declared'
      when nullif(g.nuts_3, '') is not null then 'declared'
      when g.spatial_nuts3_id is not null then 'spatial_estimate'
    end as nuts3_link_source,

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