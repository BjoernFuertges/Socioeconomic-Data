{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_geom_canonical_gist on {{ this }} using gist (geometry_3035)"
        ]
    )
}}

/*
  Exactly one representative geometry per German gisco_id (the latest known
  period from mart_lau_geometry_historized).

  A standalone model rather than a CTE so int_lau_spatial_crosswalk.sql can
  reference this table twice while keeping its GiST index (Postgres >= 12
  materializes a CTE referenced twice as an unindexed intermediate result).
  See int_lau_spatial_crosswalk.sql for why only one geometry per gisco_id
  is used.
*/

select distinct on (gisco_id)
    gisco_id,
    geometry_3035
from {{ ref('mart_lau_geometry_historized') }}
where country_code = 'DE'
  and has_valid_geometry
order by gisco_id, valid_from desc
