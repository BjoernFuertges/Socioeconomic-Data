{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_geom_hist_gist on {{ this }} using gist (geometry_3035)",
            "create index if not exists idx_mart_lau_geom_hist_id_from on {{ this }} (gisco_id, valid_from)",
            "create index if not exists idx_mart_lau_geom_hist_country on {{ this }} (country_code, valid_from)"
        ]
    )
}}

/*
  Historized LAU (Local Administrative Unit) geometries.

  Each row is a period in which a geometry version was valid for a LAU.
  Vintages with identical geometry are merged into one row.

  valid_from  = first vintage year in which this geometry version appears
  valid_to    = last year this version was valid (NULL = still current)

  Source: stg_lau_geometry (one row per gisco_id and vintage year)
  Available vintage years: 2011-2024
*/

with source_geoms as (
    -- Encode the geometry as a compact hash so the cross-vintage comparison
    -- is cheap (window functions buffer only the hash, not the full WKT).
    select
        gisco_id,
        country_code,
        lau_id,
        lau_name,
        nuts_3,
        year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry,
        md5(ST_AsText(geometry_3035)) as geom_hash
    from {{ ref('stg_lau_geometry') }}
),

flagged as (
    -- Flag period changes: 1 if the geometry differs from the previous
    -- vintage (is distinct from also covers a unit's first row, where
    -- lag() is NULL).
    select
        *,
        case
            when geom_hash is distinct from lag(geom_hash) over (
                partition by gisco_id
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
            partition by gisco_id
            order by year
            rows between unbounded preceding and current row
        ) as period_id
    from flagged
),

period_bounds as (
    -- One row per LAU and period: the first vintage (valid_from). The
    -- geometry is identical within the period, so it is taken along directly.
    select distinct on (gisco_id, period_id)
        gisco_id,
        country_code,
        lau_id,
        lau_name,
        nuts_3,
        year as valid_from,
        max(year) over (partition by gisco_id, period_id) as max_vintage_year,
        -- Last vintage year across all LAUs - reference to tell "still
        -- current" from "discontinued" (see valid_to below).
        max(year) over () as global_max_vintage_year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry
    from periods
    order by gisco_id, period_id, year
)

select
    gisco_id,
    country_code,
    lau_id,
    lau_name,
    nuts_3,
    valid_from,
    -- valid_to = start of the next period - 1, if one exists. For a LAU's
    -- last known period, NULL ("still current") is only correct if the LAU
    -- was still present in the global last vintage year. Otherwise the code
    -- was simply discontinued (e.g. retired by a Gemeinde merger, see
    -- max_vintage_year); without this distinction such a code would
    -- wrongly match EVERY later geometry_year, duplicating its successor.
    coalesce(
        lead(valid_from) over (partition by gisco_id order by valid_from) - 1,
        case when max_vintage_year < global_max_vintage_year then max_vintage_year end
    ) as valid_to,
    max_vintage_year,
    geometry_3035,
    ST_AsGeoJSON(geometry_wgs84)::json as geometry_wgs84,
    has_valid_geometry
from period_bounds
order by gisco_id, valid_from
