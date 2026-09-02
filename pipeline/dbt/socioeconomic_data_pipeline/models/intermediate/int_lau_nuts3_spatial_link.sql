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
  Räumliche Zuordnung jeder deutschen LAU-Gemeinde zu ihrer NUTS-3-Region,
  gegen eine feste Referenz-NUTS-Version (aktuell/neueste, {{ reference_nuts_version }}).

  Dient als Fallback für die kanonische nuts3_id in lau_integrated.sql, wenn
  die deklarierten Quellspalten (nuts_3_code aus der Populationsquelle,
  nuts_3 aus der Geometriequelle) NULL sind - was mit den lokalen Testdaten
  durchgehend der Fall ist, da die GISCO-GeoJSONs kein NUTS_3-Property
  führen (siehe stg_lau_geometry.sql) und die Populationsquelle je nach
  geladenem Datensatz ebenfalls leer sein kann.

  Zuordnung über die größte Schnittfläche (statt reinem ST_Contains), um
  numerische Ungenauigkeiten und Randberührungen an Gemeindegrenzen
  abzufangen - gleiche Logik wie int_lau_nuts3_membership.sql.

  NUTS-3-Geometrien kommen bewusst aus mart_nuts_geometry_historized statt
  direkt aus stg_nuts_geometry: stg_nuts_geometry ist eine View, die
  geometry_3035 bei jeder Abfrage per ST_GeomFromText aus der rohen
  loaded_data-Textspalte neu parst - ohne jede Möglichkeit eines
  Geometrie-Index. Ein Join von ~12.500 LAU-Geometrien gegen ~400 solcher
  Views-Zeilen lief mehrfach mehrere Minuten, ohne fertig zu werden
  (Nested Loop ohne Index, ST_GeomFromText pro Kandidatenpaar neu
  ausgeführt). mart_nuts_geometry_historized speichert dieselbe Geometrie
  bereits als echte PostGIS-Spalte mit GiST-Index.

  Grain: eine Zeile je gisco_id (nicht je Geometrie-Periode) - basiert auf
  int_lau_geometry_canonical (genau eine repräsentative Geometrie je
  Gemeinde), aus demselben Performance-Grund wie in
  int_lau_spatial_crosswalk.sql dokumentiert: ein Join über alle ~150.000
  deutschen LAU-Perioden (statt nur ~12.500 Gemeinden) gegen die NUTS-3-
  Geometrien lief mehrere Minuten, ohne fertig zu werden. Da sich der
  NUTS-3-Parent einer Gemeinde durch reine Neu-Generalisierung der
  Grenzlinie zwischen zwei Vintages ohnehin praktisch nie ändert, ist eine
  Zeile je gisco_id ausreichend; lau_integrated.sql joint entsprechend nur
  über gisco_id, nicht zusätzlich über valid_from.
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
