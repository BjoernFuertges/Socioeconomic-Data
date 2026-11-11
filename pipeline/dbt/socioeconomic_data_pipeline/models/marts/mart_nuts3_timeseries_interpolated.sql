{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts3_ts_interp_id on {{ this }} (target_version, nuts3_id, dataset_slug, year)",
            "create index if not exists idx_mart_nuts3_ts_interp_ds on {{ this }} (target_version, dataset_slug, year)"
        ]
    )
}}

/*
  Gap-free time series with two alternative imputation methods for missing
  years per (nuts3_id, dataset_slug, target_version):

    Method 1 (linear)      value_linear      - linear interpolation between
      the last known previous year and the next known year (possibly across
      several gap years).
    Method 3 (peer_trend)  value_peer_trend  - carries the last known own
      value (year - 1) forward with the average relative change of other
      regions in the same NUTS-2 group (first 4 characters of the NUTS-3 ID)
      from (year - 1) to year. Unlike method 1 it does NOT carry forward
      across several years - if the previous-year value itself is unknown,
      value_peer_trend stays NULL.

  target_version: mart_nuts3_sociodata_harmonized already carries the same
  values for several NUTS-3 target versions in parallel (see there).
  Gap filling and peer trend run independently per target_version, since a
  region can have different neighbours and value gaps in different versions.

  `value` remains the "official" output value for backward compatibility
  (original value, else the method-1 estimate). `value_linear` and
  `value_peer_trend` are filled only for actually interpolated rows
  (is_interpolated = true), so consumers (e.g. the API) can expose both
  estimates side by side instead of silently mixing them.

  For every (nuts3_id, dataset_slug, target_version) combination the known
  year span is filled completely (GENERATE_SERIES from min_year to max_year).

  No extrapolation: values outside the known span stay NULL (filtered out) -
  edge values are not carried forward.

  is_interpolated = false  -> original value from mart_nuts3_sociodata_harmonized
  is_interpolated = true   -> value linearly interpolated between neighbours

  Interpolation formula (method 1):
    value = v_prev + (v_next - v_prev) * (year - y_prev) / (y_next - y_prev)

  Carry-forward formula (method 3):
    value_peer_trend = v_own_prev * (1 + avg rel_change_peers)
    rel_change_peers  = (v_peer_year - v_peer_prevyear) / v_peer_prevyear
    (only peers with a known, non-zero previous-year value)

  Evaluation (analysis/data-analysis/impute_missing_values.py, leave-one-out
  over 520 samples from all 13 Kreis datasets, see
  docs/pipeline/dbt.md#interpolationsmethoden-vergleich): method 1 gives
  the more accurate estimates on average (MAPE 2.45 %, hit rate 51 %);
  method 3 is slightly better for some datasets (e.g. schuldnerquote,
  wohngeldhaushalte, medianeinkommen; hit rate 42 %).

  LOCF trick (Last Observation Carried Forward):
    count(value) cumulatively counts non-null values in the window frame.
    All rows up to the next non-null value share the same group ID, so
    first_value() within that group yields the last known value. Applied
    backward (ORDER BY year DESC) it yields the next value.
*/

with source as (
    select * from {{ ref('mart_nuts3_sociodata_harmonized') }}
),

-- Year span per region, dataset and target version
year_ranges as (
    select
        target_nuts_id,
        dataset_slug,
        target_version,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by target_nuts_id, dataset_slug, target_version
),

-- Complete year series without gaps
all_years as (
    select
        r.target_nuts_id   as nuts3_id,
        r.dataset_slug,
        r.target_version,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Join the original data (NULL for missing years)
with_values as (
    select
        a.nuts3_id,
        a.dataset_slug,
        a.target_version,
        a.year,
        s.harmonized_value as value
    from all_years a
    left join source s
        on  s.target_nuts_id  = a.nuts3_id
        and s.dataset_slug    = a.dataset_slug
        and s.target_version  = a.target_version
        and s.year            = a.year
),

/*
  Step 1: groups for the last known value (forward).
  count(value) OVER (ORDER BY year ROWS UNBOUNDED PRECEDING) grows only on
  non-NULL values -> forms groups in which the value is constant.
*/
with_prev_group as (
    select
        *,
        count(value) over (
            partition by nuts3_id, dataset_slug, target_version
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

-- Step 2: groups for the next known value (backward)
with_next_group as (
    select
        *,
        count(value) over (
            partition by nuts3_id, dataset_slug, target_version
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

/*
  Method 3 - peer-based carry-forward.

  Step A: NUTS-2 group per region (first 4 characters of the NUTS-3 ID).
  Step B: Relative change of each region from (year - 1) to year, based
          solely on original values (with_values), not on already
          interpolated/carried-forward values.
  Step C: Average relative change per (NUTS-2 group, dataset, target
          version, year) over all regions with known previous-year and
          current-year values (previous-year value 0 is excluded, division
          by 0). The target region itself contributes nothing, as its own
          value for the year is unknown by definition.
  Step D: The region's own previous-year value (exactly year - 1, from
          with_values, no LOCF over several years).
*/
with_peer_base as (
    select
        nuts3_id,
        dataset_slug,
        target_version,
        year,
        value,
        substr(nuts3_id, 1, 4) as nuts2_prefix
    from with_values
),

peer_rel_change as (
    select
        cur.nuts2_prefix,
        cur.dataset_slug,
        cur.target_version,
        cur.year,
        case
            when prev.value is not null
                and prev.value != 0
                and cur.value is not null
                then (cur.value - prev.value) / prev.value
        end as rel_change
    from with_peer_base cur
    join with_peer_base prev
        on  prev.nuts3_id       = cur.nuts3_id
        and prev.dataset_slug   = cur.dataset_slug
        and prev.target_version = cur.target_version
        and prev.year           = cur.year - 1
),

peer_avg_change as (
    select
        nuts2_prefix,
        dataset_slug,
        target_version,
        year,
        avg(rel_change)   as avg_rel_change,
        count(rel_change) as n_peer_regions
    from peer_rel_change
    where rel_change is not null
    group by nuts2_prefix, dataset_slug, target_version, year
),

own_prev_actual as (
    select
        nuts3_id,
        dataset_slug,
        target_version,
        year + 1 as target_year,
        value    as own_prev_value
    from with_values
    where value is not null
),

-- Step 3: apply interpolation
interpolated as (
    select
        n.nuts3_id,
        n.dataset_slug,
        n.target_version,
        n.year,
        case
            -- Original value present -> unchanged
            when n.value is not null
                then n.value
            -- No predecessor or successor -> no extrapolation
            when n.prev_value is null or n.next_value is null
                then null
            -- Linear interpolation (method 1)
            else n.prev_value
                + (n.next_value - n.prev_value)
                * (n.year       - n.prev_year )::numeric
                / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value,
        n.value is null
            and n.prev_value is not null
            and n.next_value is not null                      as is_interpolated,
        -- Method 1: only filled for actually interpolated rows, otherwise
        -- identical to "value" above (no duplicate needed).
        case
            when n.value is null
                and n.prev_value is not null
                and n.next_value is not null
                then n.prev_value
                    + (n.next_value - n.prev_value)
                    * (n.year       - n.prev_year )::numeric
                    / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value_linear,
        -- Method 3: only filled if both the region's own previous-year value
        -- and at least one peer value for this (year, dataset, target
        -- version) are known.
        case
            when n.value is null
                then opa.own_prev_value * (1 + pac.avg_rel_change)
        end                                                   as value_peer_trend,
        case when n.value is null then pac.n_peer_regions end as n_peer_regions
    from with_next n
    left join peer_avg_change pac
        on  pac.nuts2_prefix    = substr(n.nuts3_id, 1, 4)
        and pac.dataset_slug    = n.dataset_slug
        and pac.target_version  = n.target_version
        and pac.year            = n.year
    left join own_prev_actual opa
        on  opa.nuts3_id        = n.nuts3_id
        and opa.dataset_slug    = n.dataset_slug
        and opa.target_version  = n.target_version
        and opa.target_year     = n.year
)

-- Filter out rows without a value (outside the known range)
select *
from interpolated
where value is not null
order by target_version, nuts3_id, dataset_slug, year
