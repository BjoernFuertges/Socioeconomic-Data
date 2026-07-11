{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_int_lau_nuts3_popw_src on {{ this }} (source_nuts_id, source_version, target_version)",
            "create index if not exists idx_int_lau_nuts3_popw_tgt on {{ this }} (target_nuts_id, target_version, source_version)"
        ]
    )
}}

/*
  Populationsbasiertes Rohgewicht je NUTS-3-Versionspaar, ermittelt über die
  tatsächliche Gemeinde(LAU)-Bevölkerung.

  Warum nicht die Gesamt-Bevölkerung der NUTS-3-Quellregion (wie zuvor in
  einer früheren Version dieses Crosswalks)? Weil dieser Wert für alle
  Zielregionen einer Quellregion identisch ist und sich daher bei der
  Normierung (Division durch die Summe je Quellregion) vollständig
  herauskürzt — das Ergebnis wäre rein flächenbasiert, obwohl es als
  "populationsgewichtet" bezeichnet würde.

  Stattdessen wird jede Gemeinde (LAU) einzeln sowohl der Quell- als auch
  der Zielregion zugeordnet (int_lau_nuts3_membership) und ihre Bevölkerung
  vollständig dem jeweiligen (Quell-, Ziel-)-Paar zugeschlagen. Dadurch
  variiert das Gewicht tatsächlich je Zielregion, abhängig davon, wo
  innerhalb der Quellregion die Bevölkerung wirklich lebt.

  Bevölkerungsjahr je Versionswechsel (nächstgelegenes tatsächlich
  verfügbares LAU-Bevölkerungsjahr, falls das exakte Jahr fehlt):
    2013 → 2016 : 2015
    2016 → 2021 : 2020
    2021 → 2024 : 2023
*/

with available_years as (
    select distinct year from {{ ref('int_lau_population_geo') }}
),

version_pairs (source_version, target_version, ref_year) as (
    values
        (2013, 2016, 2015),
        (2016, 2021, 2020),
        (2021, 2024, 2023),
        (2016, 2013, 2015),
        (2021, 2016, 2020),
        (2024, 2021, 2023)
),

-- Nächstgelegenes tatsächlich verfügbares LAU-Bevölkerungsjahr je Referenzjahr
resolved as (
    select
        vp.source_version,
        vp.target_version,
        (
            select ay.year
            from available_years ay
            order by abs(ay.year - vp.ref_year), ay.year
            limit 1
        ) as pop_year
    from version_pairs vp
),

membership as (
    select * from {{ ref('int_lau_nuts3_membership') }}
),

population as (
    select gisco_id, year, population_count
    from {{ ref('int_lau_population_geo') }}
)

select
    r.source_version,
    r.target_version,
    src.nuts_id                as source_nuts_id,
    tgt.nuts_id                as target_nuts_id,
    sum(pop.population_count)  as lau_population
from resolved r
join population pop
    on pop.year = r.pop_year
join membership src
    on  src.gisco_id     = pop.gisco_id
    and src.year         = pop.year
    and src.version_year = r.source_version
join membership tgt
    on  tgt.gisco_id     = pop.gisco_id
    and tgt.year         = pop.year
    and tgt.version_year = r.target_version
group by r.source_version, r.target_version, src.nuts_id, tgt.nuts_id
