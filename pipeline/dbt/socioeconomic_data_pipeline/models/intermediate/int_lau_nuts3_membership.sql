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
  NUTS-Version (2003, 2006, 2010, 2013, 2016, 2021, 2024).

  Zuordnung über die größte Schnittfläche (statt reinem ST_Contains), um
  numerische Ungenauigkeiten und Randberührungen an Gemeindegrenzen
  abzufangen.

  Nur für die LAU-Bevölkerungsjahre berechnet, die in
  int_lau_population_geo tatsächlich vorhanden sind (nicht für alle
  verfügbaren LAU-Geometriejahre), um die räumliche Verschneidung nicht
  unnötig aufzublähen.

  NUTS-3-Geometrien kommen bewusst aus mart_nuts_geometry_historized statt
  direkt aus stg_nuts_geometry - gleicher Grund wie in
  int_lau_nuts3_spatial_link.sql dokumentiert: stg_nuts_geometry ist eine
  View, die geometry_3035 bei jeder Abfrage per ST_GeomFromText aus der
  rohen loaded_data-Textspalte neu parst, ohne jede Möglichkeit eines
  Geometrie-Index. Der Join der LAU-Bevölkerungsjahre (mehrere zehntausend
  Zeilen) dagegen blieb dadurch zuletzt in der Praxis hängen (EXPLAIN-
  Kosten im Milliarden-Bereich, dbt run kam über diesen Knoten nicht
  hinaus). mart_nuts_geometry_historized speichert dieselbe Geometrie
  bereits als echte, GiST-indizierte PostGIS-Spalte je Gültigkeitsperiode
  (valid_from/valid_to) statt je Einzeljahr - version_year wird daher
  gegen diesen Zeitraum gefiltert statt gegen ein einzelnes year-Feld.
*/

with version_years as (
    select distinct year as version_year
    from {{ ref('stg_nuts_geometry') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
),

nuts_de as (
    select
        vy.version_year,
        g.nuts_id,
        g.geometry_3035
    from version_years vy
    join {{ ref('mart_nuts_geometry_historized') }} g
        on  g.levl_code    = 3
        and g.country_code = 'DE'
        and g.has_valid_geometry
        and g.valid_from <= vy.version_year
        and (g.valid_to is null or g.valid_to >= vy.version_year)
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
