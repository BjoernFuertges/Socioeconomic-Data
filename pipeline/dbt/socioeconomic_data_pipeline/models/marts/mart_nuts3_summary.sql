{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert alle verfügbaren sozioökonomischen Indikatoren je NUTS-3-Region und Jahr.

  Basis: FULL OUTER JOIN zwischen Bevölkerung und Schuldnerquote (beide NUTS-nativ,
  vollständig und qualitätssicher). Arbeitslosenzahlen werden per LEFT JOIN ergänzt,
  da die Arbeitsagenturbezirke nicht 1:1 den NUTS-3-Kreisen entsprechen und das
  Geo-Mapping variiert (exact_id bis fuzzy-matching).

  Nur Datensätze mit has_valid_value = true werden einbezogen.
  Das mart_arbeitslosenzahlen enthält bereits ausschließlich jahresbezogene Datensätze.
*/

with bevoelkerung as (
    select
        geodata_id_nuts,
        geodata_name,
        year,
        bevoelkerung
    from {{ ref('stg_bevoelkerung_kreise') }}
    where has_valid_value = true
),

schuldnerquote as (
    select
        geodata_id_nuts,
        geodata_name,
        year,
        schuldnerquote_prozent
    from {{ ref('stg_schuldnerquote_kreise') }}
    where has_valid_value = true
),

arbeitslos as (
    select
        geodata_id,
        geodata_name,
        year,
        arbeitslose,
        yoy_change_percent
    from {{ ref('mart_arbeitslosenzahlen') }}
),

-- Basis: FULL OUTER JOIN der beiden NUTS-nativen Datensätze
nuts_base as (
    select
        coalesce(b.geodata_id_nuts, s.geodata_id_nuts) as nuts_id,
        coalesce(b.geodata_name,    s.geodata_name)    as region_name,
        coalesce(b.year,            s.year)             as year,
        b.bevoelkerung,
        s.schuldnerquote_prozent
    from bevoelkerung b
    full outer join schuldnerquote s
        on  b.geodata_id_nuts = s.geodata_id_nuts
        and b.year            = s.year
)

select
    n.nuts_id,
    n.region_name,
    n.year,
    n.bevoelkerung,
    n.schuldnerquote_prozent,
    a.arbeitslose,
    a.yoy_change_percent
from nuts_base n
left join arbeitslos a
    on  a.geodata_id = n.nuts_id
    and a.year       = n.year
