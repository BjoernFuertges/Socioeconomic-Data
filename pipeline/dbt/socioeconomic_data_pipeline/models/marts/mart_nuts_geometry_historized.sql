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

-- All loaded vintage years with their predecessor/successor vintage, to
-- tell a region missing from an intermediate vintage (gap) apart from one
-- present in consecutive vintages.
vintages as (
    select
        year,
        lag(year)  over (order by year) as prev_vintage_year,
        lead(year) over (order by year) as next_vintage_year
    from (select distinct year from source_geoms) y
),

flagged as (
    -- Flag period changes: 1 if the geometry differs from the previous
    -- vintage (is distinct from also covers a region's first row, where
    -- lag() is NULL), or if the region was missing from the vintage right
    -- before this one (gap): a code that reappears with an unchanged
    -- geometry starts a new period instead of being merged across the gap.
    select
        sg.*,
        case
            when sg.geom_hash is distinct from lag(sg.geom_hash) over w then 1
            when lag(sg.year) over w is distinct from v.prev_vintage_year then 1
            else 0
        end as is_new_period
    from source_geoms sg
    join vintages v on v.year = sg.year
    window w as (partition by sg.nuts_id order by sg.year)
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
    -- region_name comes from the period's newest vintage (a rename without
    -- a boundary change keeps the period but shows the current name).
    select distinct on (p.nuts_id, p.period_id)
        p.nuts_id,
        p.levl_code,
        p.country_code,
        last_value(p.region_name) over (
            partition by p.nuts_id, p.period_id
            order by p.year
            rows between unbounded preceding and unbounded following
        ) as region_name,
        p.year as valid_from,
        max(p.year) over (partition by p.nuts_id, p.period_id) as max_vintage_year,
        -- Last vintage year across all regions - reference to tell "still
        -- current" from "discontinued" (see valid_to below).
        max(p.year) over () as global_max_vintage_year,
        p.geometry_3035,
        p.geometry_wgs84,
        p.has_valid_geometry
    from periods p
    order by p.nuts_id, p.period_id, p.year
),

period_bounds_next as (
    select
        pb.*,
        -- vintage following the period's last vintage (null = none loaded)
        v.next_vintage_year as next_vintage_after_period
    from period_bounds pb
    join vintages v on v.year = pb.max_vintage_year
)

select
    nuts_id,
    levl_code,
    country_code,
    region_name,
    valid_from,
    -- valid_to = start of the next period - 1, if that period starts in the
    -- very next vintage. For a region's last known period, NULL ("still
    -- current") is only correct if the region was still present in the
    -- global last vintage year. Otherwise the code was discontinued (e.g.
    -- retired/renamed by a boundary reform) or is missing from the next
    -- vintage (gap) - valid_to is then max_vintage_year; without this such
    -- a code would wrongly match later geometry_years in which it does not
    -- exist, duplicating its successor code.
    case
        when lead(valid_from) over (partition by nuts_id order by valid_from)
             = next_vintage_after_period
            then next_vintage_after_period - 1
        when max_vintage_year < global_max_vintage_year
            then max_vintage_year
    end as valid_to,
    max_vintage_year,
    geometry_3035,
    ST_AsGeoJSON(geometry_wgs84)::json as geometry_wgs84,
    has_valid_geometry
from period_bounds_next
order by nuts_id, valid_from
