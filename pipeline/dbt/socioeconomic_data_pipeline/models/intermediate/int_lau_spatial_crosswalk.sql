{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_spatial_xwalk_src on {{ this }} (source_gisco_id, source_version)",
            "create index if not exists idx_int_lau_spatial_xwalk_tgt on {{ this }} (target_gisco_id, target_version)"
        ]
    )
}}

/*
  Spatial intersections between LAU Gemeinden with DIFFERENT gisco_id
  (Germany only) - analogous to int_nuts_spatial_crosswalk.sql, covers ONLY
  real administrative boundary changes (Gemeinde mergers/splits).

  Simplification vs. the NUTS model, required for performance: the
  self-join runs over EXACTLY ONE representative geometry per gisco_id (the
  latest known period, int_lau_geometry_canonical), not over all geometry
  periods of mart_lau_geometry_historized. Reasons:

    - Over the 14 LAU vintage years, mart_lau_geometry_historized collapses
      far less than NUTS (~12 instead of 1-3 periods per Gemeinde), because
      GISCO re-generalizes boundaries every year and produces slightly
      different coordinates even without a real change.
    - A self-join over all ~150,000 German LAU periods (instead of ~12,500
      Gemeinden) was tried repeatedly and still ran over 108 minutes
      without finishing, even with GiST index support.
    - One geometry per gisco_id cuts the candidate set by ~12x and avoids
      recomputing near-identical results for noise-duplicate periods.

  Trade-off: a boundary reform is detected from the LATEST known geometry
  of both Gemeinden, not the historically exact one at reform time. That is
  an acceptable approximation for a coarse area weighting. The temporal
  assignment (which vintage years count as source_version/target_version)
  stays exact, since it comes from each gisco_id's actual lifecycle
  (min/max vintage year).

  area_fraction = ST_Area(source ∩ target) / ST_Area(source); threshold
  > 1 % as in the NUTS model (excludes boundary touches / numerical noise).

  int_lau_geometry_canonical is a standalone model (not a local CTE) so
  both join sides reference the indexed table; a CTE referenced twice is
  materialized by Postgres >= 12 without the GiST index, degrading the
  self-join to an unindexed nested loop.
*/

with lifespans as (
    select
        gisco_id,
        min(valid_from) as min_valid_from,
        max(coalesce(valid_to, max_vintage_year)) as max_valid_to
    from {{ ref('mart_lau_geometry_historized') }}
    where country_code = 'DE'
      and has_valid_geometry
    group by gisco_id
),

period_pairs as (
    select
        s.gisco_id as source_gisco_id,
        t.gisco_id as target_gisco_id,
        ST_Area(ST_Intersection(s.geometry_3035, t.geometry_3035))
            / nullif(ST_Area(s.geometry_3035), 0) as area_fraction
    from {{ ref('int_lau_geometry_canonical') }} s
    join {{ ref('int_lau_geometry_canonical') }} t
        on  ST_Intersects(s.geometry_3035, t.geometry_3035)
        and s.gisco_id != t.gisco_id
    where ST_Area(ST_Intersection(s.geometry_3035, t.geometry_3035))
            / nullif(ST_Area(s.geometry_3035), 0) > 0.01
)

select
    sy.year as source_version,
    ty.year as target_version,
    pp.source_gisco_id,
    pp.target_gisco_id,
    pp.area_fraction
from period_pairs pp
join lifespans sl on sl.gisco_id = pp.source_gisco_id
join lifespans tl on tl.gisco_id = pp.target_gisco_id
cross join lateral generate_series(sl.min_valid_from, sl.max_valid_to) as sy(year)
cross join lateral generate_series(tl.min_valid_from, tl.max_valid_to) as ty(year)
where sy.year != ty.year
  -- Only year pairs that span the change: if the source Gemeinde still
  -- exists in the target year AND the target Gemeinde already existed in
  -- the source year, nothing happened between them (e.g. a merger survivor
  -- and its later-absorbed partner, both before the merger: the survivor's
  -- canonical geometry is the enlarged one and would otherwise hand part
  -- of its value to the partner).
  and not (
        ty.year between sl.min_valid_from and sl.max_valid_to
    and sy.year between tl.min_valid_from and tl.max_valid_to
  )
