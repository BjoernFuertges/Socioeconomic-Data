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

  Direkte (1-hop) und transitive (2-hop) Abbildungen.

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
),

-- Normierung: Gewichte je Quell-Region auf Summe = 1,0 bringen
direct as (
    select
        source_version,
        target_version,
        source_nuts_id,
        target_nuts_id,
        raw_weight / nullif(
            sum(raw_weight) over (
                partition by source_nuts_id, source_version, target_version
            ), 0
        )           as population_weight,
        1           as n_hops
    from raw_weights
),

-- 2-Hop-Kompositionen: z. B. 2013 → 2016 → 2021  ergibt  2013 → 2021
-- Gewicht = w(2013→2016) × w(2016→2021)
two_hop_raw as (
    select
        a.source_version,
        b.target_version,
        a.source_nuts_id,
        b.target_nuts_id,
        a.population_weight * b.population_weight as population_weight,
        2                                          as n_hops
    from direct a
    join direct b
        on  a.target_nuts_id = b.source_nuts_id
        and a.target_version = b.source_version
),

-- Mehrere Pfade zum gleichen Ziel summieren
two_hop as (
    select
        source_version,
        target_version,
        source_nuts_id,
        target_nuts_id,
        sum(population_weight) as population_weight,
        2                      as n_hops
    from two_hop_raw
    group by 1, 2, 3, 4
)

-- Selbst-Referenz-Paare ausfiltern (source = target, durch Hin-Rück-Hops entstanden)
select * from direct
where source_version != target_version
union all
select * from two_hop
where source_version != target_version
order by source_version, target_version, source_nuts_id, n_hops, target_nuts_id
