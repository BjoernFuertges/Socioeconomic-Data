{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_xwalk_src on {{ this }} (source_gisco_id, source_version, target_version)",
            "create index if not exists idx_mart_lau_xwalk_tgt on {{ this }} (target_gisco_id, target_version, source_version)"
        ]
    )
}}

/*
  Crosswalk zwischen LAU-Versionen (Deutschland), analog zu
  mart_nuts_version_crosswalk.sql. Zwei Bestandteile:

  1. identity_pairs: für jede Gemeinde ohne administrative Grenzänderung
     zwischen Quell- und Zieljahr - der weitaus häufigste Fall - eine
     triviale Selbstzuordnung mit weight = 1,0, ohne räumliche Berechnung.
     Deckt bewusst den GESAMTEN Lebenszyklus einer gisco_id ab (von der
     ersten bis zur letzten Vintage-Periode, über
     min(valid_from)/max(coalesce(valid_to, max_vintage_year)) je
     gisco_id), nicht nur Jahre innerhalb einer einzelnen Geometrie-
     Periode: mart_lau_geometry_historized kollabiert über die 14
     Vintage-Jahre bei Weitem nicht so stark wie bei NUTS (im Schnitt ~12
     statt 1-3 Perioden je Gemeinde, weil GISCO die Grenzlinien jedes Jahr
     neu generalisiert und dabei minimal andere Koordinaten erzeugt, auch
     ohne echte Gebietsänderung). Diese Perioden-Fragmentierung bedeutet
     aber praktisch nie eine echte administrative Änderung - die Gemeinde
     bleibt dieselbe gisco_id. int_lau_spatial_crosswalk berechnet daher
     bewusst NUR Paare mit source_gisco_id != target_gisco_id (echte
     Fusionen/Teilungen); alle Jahres-Paare derselben gisco_id werden hier
     günstig ohne jede räumliche Berechnung abgedeckt.
  2. crosswalk_pairs: für Gemeinden mit tatsächlicher administrativer
     Grenzänderung (Fusion, Teilung) die räumliche Verschneidung aus
     int_lau_spatial_crosswalk, je Quellregion auf Summe ≈ 1,0 normiert.

  Beide Mengen sind auf Ebene (source_gisco_id, source_version,
  target_version) disjunkt (identity_pairs hat immer target_gisco_id =
  source_gisco_id, crosswalk_pairs nie) - daher genügt UNION ALL ohne
  weitere Deduplizierung.

  Anders als bei NUTS gibt es keinen Sub-LAU-Populationslayer (LAU ist die
  feinste in diesem Projekt verfügbare Ebene) - das Gewicht ist daher eine
  reine Flächengewichtung, kein Populations-Fallback nötig.

  absolute_weight (= unnormiertes Gewicht vor der Normierung auf Summe 1,0)
  wird zusätzlich zur normierten weight-Spalte geführt - gebraucht für die
  gewichtete Mittelwertbildung bei Quoten/Durchschnitten in
  mart_lau_sociodata_harmonized (measure_type = 'weighted_avg'), wo Werte
  nicht aufsummiert, sondern gewichtet gemittelt werden müssen.
*/

with lau_de_lifespan as (
    -- Gesamter Lebenszyklus je gisco_id über alle ihre Perioden hinweg
    -- (siehe Kommentar oben) - keine räumliche Information nötig, nur
    -- die Jahresspanne.
    select
        gisco_id,
        min(valid_from) as min_valid_from,
        max(coalesce(valid_to, max_vintage_year)) as max_valid_to
    from {{ ref('mart_lau_geometry_historized') }}
    where country_code = 'DE'
      and has_valid_geometry
    group by gisco_id
),

-- 1. Triviale Selbstzuordnung: alle Jahres-Paare über den gesamten
-- Lebenszyklus einer Gemeinde hinweg.
identity_pairs as (
    select
        sy.year    as source_version,
        ty.year    as target_version,
        l.gisco_id as source_gisco_id,
        l.gisco_id as target_gisco_id,
        1.0::numeric as weight,
        1.0::numeric as absolute_weight
    from lau_de_lifespan l
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as sy(year)
    cross join lateral generate_series(l.min_valid_from, l.max_valid_to) as ty(year)
    where sy.year != ty.year
),

-- 2. Echte Grenzänderungen, normiert je Quellregion auf Summe 1,0.
spatial as (
    select * from {{ ref('int_lau_spatial_crosswalk') }}
),

crosswalk_pairs as (
    select
        source_version,
        target_version,
        source_gisco_id,
        target_gisco_id,
        area_fraction / nullif(
            sum(area_fraction) over (
                partition by source_gisco_id, source_version, target_version
            ), 0
        )              as weight,
        area_fraction  as absolute_weight
    from spatial
)

select * from identity_pairs
union all
select * from crosswalk_pairs
order by source_version, target_version, source_gisco_id, target_gisco_id
