{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_nuts3_spatial_link on {{ this }} (gisco_id)"
        ]
    )
}}

{% set reference_nuts_version = 2024 %}

/*
  Spatial assignment of each German LAU Gemeinde to its NUTS-3 region
  against a fixed reference NUTS version ({{ reference_nuts_version }}).

  Fallback for the canonical nuts3_id in lau_integrated.sql when the
  declared source columns (nuts_3_code from the population source, nuts_3
  from the geometry source) are NULL - always the case with local test data,
  since the GISCO GeoJSONs carry no NUTS_3 property (see stg_lau_geometry.sql).

  Assignment by largest intersection area (instead of plain ST_Contains),
  same logic as int_lau_nuts3_membership.sql.

  NUTS-3 geometries come from mart_nuts_geometry_historized, not
  stg_nuts_geometry: the staging view re-parses the geometry on every query
  and cannot be indexed; joining ~12,500 LAU geometries against it ran for
  minutes without finishing. The mart has a GiST-indexed PostGIS column.

  Grain: one row per gisco_id (not per geometry period), based on
  int_lau_geometry_canonical, for the same performance reason as in
  int_lau_spatial_crosswalk.sql. A Gemeinde's NUTS-3 parent practically
  never changes through mere re-generalization of its boundary between
  vintages, so lau_integrated.sql joins on gisco_id only (not valid_from).
*/

with nuts_de as (
    select
        nuts_id,
        geometry_3035
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
      and valid_from <= {{ reference_nuts_version }}
      and (valid_to is null or valid_to >= {{ reference_nuts_version }})
),

ranked as (
    select
        l.gisco_id,
        n.nuts_id,
        row_number() over (
            partition by l.gisco_id
            order by ST_Area(ST_Intersection(l.geometry_3035, n.geometry_3035)) desc
        ) as rn
    from {{ ref('int_lau_geometry_canonical') }} l
    join nuts_de n
        on ST_Intersects(l.geometry_3035, n.geometry_3035)
)

select
    gisco_id,
    nuts_id
from ranked
where rn = 1
