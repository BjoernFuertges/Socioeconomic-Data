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
  Genau eine repräsentative Geometrie je deutscher gisco_id (die jeweils
  letzte bekannte Periode aus mart_lau_geometry_historized).

  Eigenständiges Modell (statt CTE), damit int_lau_spatial_crosswalk.sql
  diese Tabelle zweimal referenzieren kann, ohne dass Postgres sie
  materialisiert und dabei den GiST-Index verliert (eine CTE, die zweimal
  referenziert wird, materialisiert Postgres >= v12 als eigenständiges,
  unindiziertes Zwischenergebnis - eine echte Tabelle mit eigenem Index ist
  davon nicht betroffen). Siehe Kommentar in int_lau_spatial_crosswalk.sql
  für die Begründung, warum nur eine Geometrie je gisco_id (statt aller
  Perioden) verwendet wird.
*/

select distinct on (gisco_id)
    gisco_id,
    geometry_3035
from {{ ref('mart_lau_geometry_historized') }}
where country_code = 'DE'
  and has_valid_geometry
order by gisco_id, valid_from desc
