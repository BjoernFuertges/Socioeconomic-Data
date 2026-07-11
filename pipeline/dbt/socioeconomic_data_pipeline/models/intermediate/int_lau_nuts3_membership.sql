{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_nuts3_memb on {{ this }} (gisco_id, year, version_year)"
        ]
    )
}}

/*
  Räumliche Zuordnung jeder Gemeinde (LAU) zu ihrer NUTS-3-Region je
  NUTS-Version (2013, 2016, 2021, 2024).

  Zuordnung über die größte Schnittfläche (statt reinem ST_Contains), um
  numerische Ungenauigkeiten und Randberührungen an Gemeindegrenzen
  abzufangen.

  Nur für die LAU-Bevölkerungsjahre berechnet, die in
  int_lau_population_geo tatsächlich vorhanden sind (nicht für alle
  verfügbaren LAU-Geometriejahre), um die räumliche Verschneidung nicht
  unnötig aufzublähen.
*/

with nuts_de as (
    select
        nuts_id,
        year as version_year,
        geometry_3035
    from {{ ref('stg_nuts_geometry') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
),

lau_years as (
    select distinct gisco_id, year, geometry_3035
    from {{ ref('int_lau_population_geo') }}
),

ranked as (
    select
        l.gisco_id,
        l.year,
        n.version_year,
        n.nuts_id,
        row_number() over (
            partition by l.gisco_id, l.year, n.version_year
            order by ST_Area(ST_Intersection(l.geometry_3035, n.geometry_3035)) desc
        ) as rn
    from lau_years l
    join nuts_de n
        on ST_Intersects(l.geometry_3035, n.geometry_3035)
)

select
    gisco_id,
    year,
    version_year,
    nuts_id
from ranked
where rn = 1
