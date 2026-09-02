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
  Räumliche Schnittmengen zwischen LAU-Gemeinden VERSCHIEDENER gisco_id (nur
  Deutschland) - analog zu int_nuts_spatial_crosswalk.sql, deckt NUR echte
  administrative Grenzänderungen ab (Gemeindefusion/-teilung).

  WICHTIGE VEREINFACHUNG gegenüber dem NUTS-Vorbild, aus Performance-Gründen
  zwingend nötig: der Self-Join läuft nicht über alle Geometrie-Perioden aus
  mart_lau_geometry_historized, sondern über GENAU EINE repräsentative
  Geometrie je gisco_id (die jeweils letzte bekannte Periode). Grund:

    - mart_lau_geometry_historized kollabiert über die 14 LAU-Vintage-Jahre
      bei Weitem nicht so stark wie NUTS (im Schnitt ~12 statt 1-3 Perioden
      je Gemeinde), weil GISCO die Grenzlinien jedes Jahr neu generalisiert
      und dabei minimal andere Koordinaten erzeugt, auch ohne echte
      Gebietsänderung.
    - Ein Self-Join über alle ~150.000 deutschen LAU-Perioden (statt nur
      ~12.500 Gemeinden) wurde mehrfach getestet (siehe Git-Historie dieser
      Datei: direkter Tabellen-Join statt CTE, Ausschluss gleicher gisco_id,
      Deaktivieren von Bitmap-Scan-Plänen) und lief zuletzt 108 Minuten,
      ohne fertig zu werden (abgebrochen) - selbst mit GiST-Index-
      Unterstützung dominiert die schiere Zahl der zu verschneidenden
      Perioden-Paare (viele davon ohnehin nur Rausch-Duplikate derselben
      Gemeinde) die Laufzeit vollständig.
    - Genau eine Geometrie je gisco_id reduziert die Kandidatenmenge um
      Faktor ~12 UND vermeidet, dieselbe (Fast-)Antwort zig Mal für
      praktisch identische Perioden derselben Gemeinde neu zu berechnen.

  Trade-off: eine echte Gebietsreform wird anhand der jeweils LETZTEN
  bekannten Geometrie beider beteiligten Gemeinden erkannt, nicht anhand
  der historisch exakten Geometrie zum Zeitpunkt der Reform selbst. Für die
  Zwecke dieses Crosswalks (grobe Gewichtung, wie viel Fläche einer
  Quellregion in welche Zielregion(en) fällt) ist das eine akzeptable
  Näherung - die betroffenen Gemeinden ändern sich zwischen Reform und
  "aktuell" i. d. R. nicht noch einmal. Die zeitliche Zuordnung (WELCHE
  Vintage-Jahre als source_version/target_version gelten) bleibt exakt,
  da sie weiterhin aus dem tatsächlichen Lebenszyklus jeder gisco_id
  (min/max Vintage-Jahr) kommt, nicht aus der für den räumlichen Vergleich
  verwendeten Geometrie.

  area_fraction = ST_Area(source ∩ target) / ST_Area(source), Schwellwert
  > 1 % wie beim NUTS-Vorbild (schließt Randberührungen / numerisches
  Rauschen an gemeinsamen Grenzen aus).

  Performance-Hinweis: int_lau_geometry_canonical ist ein eigenständiges
  Modell (nicht eine hier lokale CTE), damit beide Join-Seiten unten die
  echte, indizierte Tabelle referenzieren - eine CTE, die zweimal
  referenziert wird, materialisiert Postgres >= v12 als eigenständiges
  Zwischenergebnis OHNE den GiST-Index auf geometry_3035, was den
  Self-Join auf einen ungeindexten Nested-Loop zurückwirft.
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
