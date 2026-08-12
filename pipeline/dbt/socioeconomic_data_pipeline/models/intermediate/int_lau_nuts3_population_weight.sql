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

  Referenzjahr je Versionspaar: das jeweils jüngere der beiden beteiligten
  NUTS-Versionsjahre minus 1 (Bevölkerungsstand kurz vor Inkrafttreten der
  neueren Version) - ref_year = GREATEST(source_version, target_version) - 1.
  Gilt unabhängig von der Richtung (vorwärts/rückwärts) und generalisiert
  automatisch auf jedes Versionspaar, nicht nur benachbarte Vintages, z. B.:
    2013 → 2016 : 2015        2016 → 2013 : 2015
    2016 → 2021 : 2020        2021 → 2016 : 2020
    2021 → 2024 : 2023        2024 → 2021 : 2023
    2003 → 2024 : 2023        2024 → 2003 : 2023

  Tatsächlich verwendet wird davon ausgehend das nächstgelegene *verfügbare*
  LAU-Bevölkerungsjahr (siehe resolved unten) - aktuell ist nur 2024 geladen,
  sodass jedes Paar de facto mit der 2024er-Bevölkerungsverteilung gewichtet
  wird, bis weitere historische LAU-Bevölkerungsjahre geladen werden.
*/

with available_years as (
    select distinct year from {{ ref('int_lau_population_geo') }}
),

nuts_version_years as (
    select distinct version_year from {{ ref('int_lau_nuts3_membership') }}
),

-- Alle gerichteten Versionspaare (jedes Jahr → jedes andere Jahr), nicht nur
-- benachbarte Vintages - analog zu int_nuts_spatial_crosswalk.
version_pairs as (
    select
        a.version_year                                  as source_version,
        b.version_year                                  as target_version,
        greatest(a.version_year, b.version_year) - 1     as ref_year
    from nuts_version_years a
    cross join nuts_version_years b
    where a.version_year != b.version_year
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
