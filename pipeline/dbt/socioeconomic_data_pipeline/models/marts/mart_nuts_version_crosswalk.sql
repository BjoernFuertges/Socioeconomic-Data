{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts_xwalk_src on {{ this }} (source_nuts_id, source_version, target_version)",
            "create index if not exists idx_mart_nuts_xwalk_tgt on {{ this }} (target_nuts_id, target_version, source_version)"
        ]
    )
}}

/*
  Bevölkerungsgewichteter Crosswalk zwischen NUTS-3-Versionen (Deutschland).

  Direkte (1-hop) Abbildung für JEDES Versionspaar - int_nuts_spatial_crosswalk
  berechnet die räumliche Schnittmenge bereits für alle Kombinationen der
  vorhandenen Vintage-Jahre (aktuell 2003, 2006, 2010, 2013, 2016, 2021,
  2024), nicht nur für benachbarte Versionen. Eine Hop-Verkettung über
  Zwischenversionen (z. B. 2003 → 2013 → 2021) ist dadurch nicht mehr nötig
  und würde nur unnötig Rundungs-/Gewichtungsfehler pro Zwischenschritt
  aufsummieren - jedes Paar hat hier bereits eine einzige, direkte
  räumliche Schnittmenge zugrunde liegen. n_hops ist daher immer 1; die
  Spalte bleibt zur Abwärtskompatibilität der Konsumenten erhalten.

  population_weight gibt an, welcher Anteil des Indikatorwerts einer Quell-Region
  auf die jeweilige Ziel-Region entfällt. Die Summe aller Gewichte je
  (source_nuts_id, source_version, target_version) ergibt ≈ 1,0.

  Die eigentliche Populationsgewichtung erfolgt über die Gemeinde(LAU)-Ebene
  (int_lau_nuts3_population_weight): Nur dort kann sich die Bevölkerung
  innerhalb einer Quellregion tatsächlich unterschiedlich auf mehrere
  Zielregionen verteilen. Eine Gewichtung mit der Gesamt-Bevölkerung der
  Quellregion selbst würde sich bei der Normierung immer herauskürzen und
  wäre de facto nur eine Flächengewichtung.

  Fallback auf reinen Flächenanteil (int_nuts_spatial_crosswalk), falls für
  ein Regionenpaar keine LAU-Bevölkerung ermittelt werden konnte (z. B.
  unbewohnte Fläche oder fehlende LAU-Daten).

  absolute_weight (= raw_weight vor der Normierung auf Summe 1,0) wird
  zusätzlich zur normierten population_weight-Spalte geführt - gebraucht
  für die gewichtete Mittelwertbildung bei Quoten/Durchschnitten in
  mart_nuts3_sociodata_harmonized (measure_type = 'weighted_avg'): dort
  darf eine Zusammenlegung mehrerer Quellregionen den Wert nicht aufsummieren
  (das wäre nur für Zählgrößen richtig), sondern muss ihn gewichtet mitteln.

  Beispiel Gebietsreform:
    Kreis A (2016) überlappt räumlich mit Kreis X und Kreis Y (2021).
    Leben 70 % der Einwohner von A auf dem Gebietsteil, der zu X wird, und
    30 % auf dem Teil, der zu Y wird, ergibt sich weight(A→X) = 0,70 und
    weight(A→Y) = 0,30 — unabhängig vom reinen Flächenverhältnis.
*/

with spatial as (
    select * from {{ ref('int_nuts_spatial_crosswalk') }}
),

lau_pop_weight as (
    select * from {{ ref('int_lau_nuts3_population_weight') }}
),

-- Rohgewicht: echte Gemeinde(LAU)-Bevölkerung je (Quell-, Ziel-)-Regionenpaar.
-- Fallback auf Flächenanteil, falls keine LAU-Bevölkerung vorhanden ist.
raw_weights as (
    select
        s.source_version,
        s.target_version,
        s.source_nuts_id,
        s.target_nuts_id,
        coalesce(lpw.lau_population, s.area_fraction) as raw_weight
    from spatial s
    left join lau_pop_weight lpw
        on  lpw.source_version = s.source_version
        and lpw.target_version = s.target_version
        and lpw.source_nuts_id = s.source_nuts_id
        and lpw.target_nuts_id = s.target_nuts_id
)

-- Normierung: Gewichte je Quell-Region auf Summe = 1,0 bringen
select
    source_version,
    target_version,
    source_nuts_id,
    target_nuts_id,
    raw_weight / nullif(
        sum(raw_weight) over (
            partition by source_nuts_id, source_version, target_version
        ), 0
    )              as population_weight,
    raw_weight     as absolute_weight,
    1              as n_hops
from raw_weights
where source_version != target_version
order by source_version, target_version, source_nuts_id, target_nuts_id
