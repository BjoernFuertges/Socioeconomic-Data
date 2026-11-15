-- mart_lau_geometry_historized.has_gap / gap_years must be internally
-- consistent: has_gap <=> gap_years non-empty, every gap year lies strictly
-- inside the period's range [valid_from, coalesce(valid_to, max_vintage_year)]
-- (valid_from and max_vintage_year are present vintages by definition), and
-- stg_lau_geometry really has no row for that gisco_id and year.
-- Returns violating rows (test passes with zero rows).

with mart as (
    select gisco_id, valid_from, valid_to, max_vintage_year, has_gap, gap_years
    from {{ ref('mart_lau_geometry_historized') }}
)

select m.gisco_id, m.valid_from, m.has_gap, m.gap_years, 'has_gap mismatch' as problem
from mart m
where m.has_gap is distinct from (cardinality(m.gap_years) > 0)

union all

select m.gisco_id, m.valid_from, m.has_gap, m.gap_years, 'gap year outside period' as problem
from mart m
cross join lateral unnest(m.gap_years) as gy(year)
where gy.year <= m.valid_from
   or gy.year >= coalesce(m.valid_to + 1, m.max_vintage_year)
   or gy.year = m.max_vintage_year

union all

select m.gisco_id, m.valid_from, m.has_gap, m.gap_years, 'gap year present in source' as problem
from mart m
cross join lateral unnest(m.gap_years) as gy(year)
where exists (
    select 1
    from {{ ref('stg_lau_geometry') }} s
    where s.gisco_id = m.gisco_id
      and s.year = gy.year
)
