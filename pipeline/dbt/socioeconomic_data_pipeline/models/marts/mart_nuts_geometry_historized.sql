{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts_geom_hist_gist on {{ this }} using gist (geometry_3035)",
            "create index if not exists idx_mart_nuts_geom_hist_id_from on {{ this }} (nuts_id, valid_from)",
            "create index if not exists idx_mart_nuts_geom_hist_levl_valid on {{ this }} (levl_code, has_valid_geometry)"
        ]
    )
}}

/*
  Historized NUTS geometries.

  Each row is a period in which a geometry version was valid for a region.
  Vintages with identical geometry are merged into one row.

  valid_from  = first vintage year in which this geometry version appears
  valid_to    = last year this version was valid (NULL = still current)

  Source: stg_nuts_geometry (one row per nuts_id and vintage year)
  Available vintage years: 2003, 2006, 2010, 2013, 2016, 2021, 2024
*/

with source_geoms as (
    -- Encode the geometry as a compact hash so the cross-vintage comparison
    -- is cheap (window functions buffer only the hash, not the full WKT).
    select
        nuts_id,
        levl_code,
        country_code,
        region_name,
        year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry,
        md5(ST_AsText(geometry_3035)) as geom_hash
    from {{ ref('stg_nuts_geometry') }}
),

flagged as (
    -- Flag period changes: 1 if the geometry differs from the previous
    -- vintage (is distinct from also covers a region's first row, where
    -- lag() is NULL).
    select
        *,
        case
            when geom_hash is distinct from lag(geom_hash) over (
                partition by nuts_id
                order by year
            ) then 1
            else 0
        end as is_new_period
    from source_geoms
),

periods as (
    -- Islands technique: the running sum of the change flags yields a stable
    -- period ID per contiguous run of identical geometry.
    select
        *,
        sum(is_new_period) over (
            partition by nuts_id
            order by year
            rows between unbounded preceding and current row
        ) as period_id
    from flagged
),

period_bounds as (
    -- One row per region and period: the first vintage (valid_from). The
    -- geometry is identical within the period, so it is taken along directly.
    select distinct on (nuts_id, period_id)
        nuts_id,
        levl_code,
        country_code,
        region_name,
        year as valid_from,
        max(year) over (partition by nuts_id, period_id) as max_vintage_year,
        -- Last vintage year across all regions - reference to tell "still
        -- current" from "discontinued" (see valid_to below).
        max(year) over () as global_max_vintage_year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry
    from periods
    order by nuts_id, period_id, year
)

select
    nuts_id,
    levl_code,
    country_code,
    region_name,
    valid_from,
    -- valid_to = start of the next period - 1, if one exists. For a region's
    -- last known period, NULL ("still current") is only correct if the
    -- region was still present in the global last vintage year. Otherwise
    -- the code was simply discontinued (e.g. retired/renamed by a boundary
    -- reform, see max_vintage_year); without this distinction such a code
    -- would wrongly match EVERY later geometry_year, duplicating its
    -- successor code.
    coalesce(
        lead(valid_from) over (partition by nuts_id order by valid_from) - 1,
        case when max_vintage_year < global_max_vintage_year then max_vintage_year end
    ) as valid_to,
    max_vintage_year,
    geometry_3035,
    ST_AsGeoJSON(geometry_wgs84)::json as geometry_wgs84,
    has_valid_geometry
from period_bounds
order by nuts_id, valid_from
