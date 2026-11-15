{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_nuts3_de_geom_gist on {{ this }} using gist (geometry_3035)",
            "create index if not exists idx_int_nuts3_de_geom_version on {{ this }} (version_year, nuts_id)"
        ]
    )
}}

/*
  German NUTS-3 geometries per vintage year (one row per nuts_id and
  version_year), materialized with a GiST index.

  stg_nuts_geometry is a view that re-parses the WKT via ST_GeomFromText on
  every access and cannot be indexed, so spatial self-joins on it (all
  version pairs in int_nuts_spatial_crosswalk) run without an index. This
  table is the indexed input for such joins; content is identical to the
  staging rows (levl_code 3, country DE, valid geometry).
*/

select
    nuts_id,
    year as version_year,
    geometry_3035
from {{ ref('stg_nuts_geometry') }}
where levl_code    = 3
  and country_code = 'DE'
  and has_valid_geometry
