{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_ts_interp_id on {{ this }} (target_version, gisco_id, dataset_slug, year)",
            "create index if not exists idx_mart_lau_ts_interp_ds on {{ this }} (target_version, dataset_slug, year)"
        ]
    )
}}

/*
  Analogous to mart_nuts3_timeseries_interpolated.sql, but at Gemeinde (LAU)
  level. Reads from mart_lau_sociodata_harmonized instead of raw staging
  sources so gap filling runs independently per LAU target version
  (target_version) - see the comment there for the target_version strategy.

  Region key: gisco_id (consistent with the harmonized mart).
  Peer group for method 3 (carry-forward): the first 5 digits of the
  official Gemeindeschlüssel (Kreis prefix), taken from
  stg_lau_population.lau_nat_code - unlike NUTS codes, LAU IDs have no
  hierarchical prefix structure, hence the detour via the AGS.

  See mart_nuts3_timeseries_interpolated.sql for the detailed documentation
  of method 1 (linear) / method 3 (peer_trend); the logic is carried over
  1:1, only with gisco_id/kreis_prefix instead of nuts3_id/nuts2_prefix and
  per target_version instead of a single target version.

  Note: with the locally loaded test data mart_lau_sociodata_harmonized is
  empty (see the comment there), so this mart builds but stays empty too.
  With correctly mapped production data it yields regular rows.
*/

with source as (
    select
        dataset_slug,
        target_gisco_id as gisco_id,
        target_version,
        year,
        harmonized_value as value
    from {{ ref('mart_lau_sociodata_harmonized') }}
),

-- Kreis key per gisco_id (for peer grouping) via the official
-- Gemeindeschlüssel from the population source.
gisco_kreis as (
    select distinct on (gisco_id)
        gisco_id,
        left(regexp_replace(cast(lau_nat_code as text), '\.0+$', ''), 5) as kreis_prefix
    from {{ ref('stg_lau_population') }}
    where gisco_id is not null and lau_nat_code is not null
    order by gisco_id, year desc
),

-- Year span per region, dataset and target version
year_ranges as (
    select
        dataset_slug,
        gisco_id,
        target_version,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by dataset_slug, gisco_id, target_version
),

-- Complete year series without gaps
all_years as (
    select
        r.gisco_id,
        r.dataset_slug,
        r.target_version,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Join the original data (NULL for missing years)
with_values as (
    select
        a.gisco_id,
        a.dataset_slug,
        a.target_version,
        a.year,
        s.value as value
    from all_years a
    left join source s
        on  s.gisco_id       = a.gisco_id
        and s.dataset_slug    = a.dataset_slug
        and s.target_version  = a.target_version
        and s.year            = a.year
),

with_prev_group as (
    select
        *,
        count(value) over (
            partition by gisco_id, dataset_slug, target_version
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by gisco_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by gisco_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

with_next_group as (
    select
        *,
        count(value) over (
            partition by gisco_id, dataset_slug, target_version
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by gisco_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by gisco_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

-- Method 3: peer-based carry-forward via the Kreis
with_peer_base as (
    select
        v.gisco_id,
        v.dataset_slug,
        v.target_version,
        v.year,
        v.value,
        k.kreis_prefix
    from with_values v
    left join gisco_kreis k
        on k.gisco_id = v.gisco_id
),

peer_rel_change as (
    select
        cur.kreis_prefix,
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
        on  prev.gisco_id       = cur.gisco_id
        and prev.dataset_slug    = cur.dataset_slug
        and prev.target_version  = cur.target_version
        and prev.year            = cur.year - 1
    where cur.kreis_prefix is not null
),

peer_avg_change as (
    select
        kreis_prefix,
        dataset_slug,
        target_version,
        year,
        avg(rel_change)   as avg_rel_change,
        count(rel_change) as n_peer_regions
    from peer_rel_change
    where rel_change is not null
    group by kreis_prefix, dataset_slug, target_version, year
),

own_prev_actual as (
    select
        gisco_id,
        dataset_slug,
        target_version,
        year + 1 as target_year,
        value    as own_prev_value
    from with_values
    where value is not null
),

interpolated as (
    select
        n.gisco_id,
        n.dataset_slug,
        n.target_version,
        n.year,
        case
            when n.value is not null
                then n.value
            when n.prev_value is null or n.next_value is null
                then null
            else n.prev_value
                + (n.next_value - n.prev_value)
                * (n.year       - n.prev_year )::numeric
                / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value,
        n.value is null
            and n.prev_value is not null
            and n.next_value is not null                      as is_interpolated,
        case
            when n.value is null
                and n.prev_value is not null
                and n.next_value is not null
                then n.prev_value
                    + (n.next_value - n.prev_value)
                    * (n.year       - n.prev_year )::numeric
                    / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value_linear,
        case
            when n.value is null
                then opa.own_prev_value * (1 + pac.avg_rel_change)
        end                                                   as value_peer_trend,
        case when n.value is null then pac.n_peer_regions end as n_peer_regions
    from with_next n
    left join gisco_kreis lk
        on lk.gisco_id = n.gisco_id
    left join peer_avg_change pac
        on  pac.kreis_prefix   = lk.kreis_prefix
        and pac.dataset_slug   = n.dataset_slug
        and pac.target_version = n.target_version
        and pac.year           = n.year
    left join own_prev_actual opa
        on  opa.gisco_id       = n.gisco_id
        and opa.dataset_slug   = n.dataset_slug
        and opa.target_version = n.target_version
        and opa.target_year    = n.year
)

select *
from interpolated
where value is not null
order by target_version, gisco_id, dataset_slug, year
