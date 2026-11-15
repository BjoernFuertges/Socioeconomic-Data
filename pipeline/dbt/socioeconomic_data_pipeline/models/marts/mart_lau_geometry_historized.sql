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

  has_gap     = true if the period's [valid_from, coalesce(valid_to,
                max_vintage_year)] range contains vintage years in which this
                gisco_id has no row in stg_lau_geometry
  gap_years   = those vintage years (sorted, empty array = no gap)

  Gaps are only FLAGGED, not split: the islands technique below compares
  each row with the previous EXISTING row of the gisco_id, so a vintage
  missing between two identical geometries is merged into one period, and a
  vintage missing right before a geometry change is covered by the earlier
  period's valid_to (= next valid_from - 1). Row set and valid_from/valid_to
  are deliberately unchanged; consumers that must not bridge missing
  vintages can filter on has_gap / gap_years. "Vintage year" means a year
  present for at least one LAU in stg_lau_geometry (not every calendar
  year), so a vintage that is missing globally is not reported as a gap.

  Source: stg_lau_geometry (one row per gisco_id and vintage year)
  Available vintage years: 2011-2024
*/

with vintages as (
    -- All vintage years present in the source (reference for gap_years).
    -- Selected separately (year only) so source_geoms stays a single-use,
    -- inlinable CTE instead of being materialized with all geometries.
    select distinct year
    from {{ ref('stg_lau_geometry') }}
),

source_geoms as (
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
    -- lau_name comes from the period's newest vintage (a rename without a
    -- boundary change keeps the period but shows the current name); lau_id
    -- and nuts_3 stay those of the first vintage.
    select distinct on (gisco_id, period_id)
        gisco_id,
        country_code,
        lau_id,
        last_value(lau_name) over (
            partition by gisco_id, period_id
            order by year
            rows between unbounded preceding and unbounded following
        ) as lau_name,
        nuts_3,
        year as valid_from,
        max(year) over (partition by gisco_id, period_id) as max_vintage_year,
        -- Vintage years actually present in this period (input for gap_years).
        array_agg(year) over (partition by gisco_id, period_id) as present_years,
        -- Last vintage year across all LAUs - reference to tell "still
        -- current" from "discontinued" (see valid_to below).
        max(year) over () as global_max_vintage_year,
        geometry_3035,
        geometry_wgs84,
        has_valid_geometry
    from periods
    order by gisco_id, period_id, year
),

historized as (
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
        has_valid_geometry,
        present_years
    from period_bounds
)

select
    h.gisco_id,
    h.country_code,
    h.lau_id,
    h.lau_name,
    h.nuts_3,
    h.valid_from,
    h.valid_to,
    h.max_vintage_year,
    h.geometry_3035,
    h.geometry_wgs84,
    h.has_valid_geometry,
    cardinality(g.gap_years) > 0 as has_gap,
    g.gap_years
from historized h
cross join lateral (
    -- Vintage years inside the period's validity range for which the source
    -- has no row of this gisco_id (see header comment).
    select array(
        select v.year::integer
        from vintages v
        where v.year between h.valid_from and coalesce(h.valid_to, h.max_vintage_year)
          and v.year <> all (h.present_years)
        order by v.year
    ) as gap_years
) g
order by h.gisco_id, h.valid_from
