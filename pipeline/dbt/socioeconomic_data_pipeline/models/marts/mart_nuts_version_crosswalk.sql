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

  Bevölkerungsjahr für die Gewichtung (letztes bekanntes Jahr vor dem Wechsel):
    2013 → 2016 : 2015
    2016 → 2021 : 2020
    2021 → 2024 : 2023

  Beispiel Gebietsreform:
    Kreis A (2016) überlappt zu 60 % mit Kreis X und zu 40 % mit Kreis Y (2021).
    Bei Bevölkerung A = 100.000 EW: weight(A→X) = 0,60, weight(A→Y) = 0,40.
    Ein Wert v für A ergibt: v × 0,60 in X und v × 0,40 in Y.
*/

with spatial as (
    select * from {{ ref('int_nuts_spatial_crosswalk') }}
),

pop as (
    select * from {{ ref('int_nuts3_population_by_year') }}
),

-- Bevölkerungsjahr = letztes bekanntes Jahr in der jeweiligen Quell-Version
pop_year_map (source_version, pop_year) as (
    values
        (2013, 2015),
        (2016, 2020),
        (2021, 2023),
        (2024, 2023)
),

-- Rohgewicht: Flächenanteil × Quell-Bevölkerung
-- Fallback auf 1, wenn keine Bevölkerungsdaten vorhanden (rein flächenbasiert)
raw_weights as (
    select
        s.source_version,
        s.target_version,
        s.source_nuts_id,
        s.target_nuts_id,
        s.area_fraction * coalesce(p.total_population, 1) as raw_weight
    from spatial s
    left join pop_year_map pym on pym.source_version = s.source_version
    left join pop          p   on  p.nuts3_id = s.source_nuts_id
                                and p.year    = pym.pop_year
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
