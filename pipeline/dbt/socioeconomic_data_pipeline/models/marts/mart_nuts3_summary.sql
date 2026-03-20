{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert alle verfügbaren sozioökonomischen Indikatoren je NUTS-3-Region und Jahr.

  Basis: FULL OUTER JOIN zwischen Bevölkerung und Schuldnerquote (beide NUTS-nativ,
  vollständig und qualitätssicher). Weitere Indikatoren werden per LEFT JOIN ergänzt,
  da die zeitliche Abdeckung variiert.

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

arbeitslose_kreise as (
    select
        geodata_id_nuts,
        year,
        arbeitslose as arbeitslose_kreise
    from {{ ref('stg_arbeitslose_kreise') }}
    where has_valid_value = true
),

arbeitsvolumen as (
    select
        geodata_id_nuts,
        year,
        arbeitsvolumen
    from {{ ref('stg_arbeitsvolumen_kreise') }}
    where has_valid_value = true
),

bruttoverdienst as (
    select
        geodata_id_nuts,
        year,
        bruttoverdienst
    from {{ ref('stg_bruttoverdienst_kreise') }}
    where has_valid_value = true
),

durchschnittsalter as (
    select
        geodata_id_nuts,
        year,
        durchschnittsalter
    from {{ ref('stg_durchschnittsalter_kreise') }}
    where has_valid_value = true
),

sgb_ii_quote as (
    select
        geodata_id_nuts,
        year,
        sgb_ii_quote_prozent
    from {{ ref('stg_sgb_ii_quote_kreise') }}
    where has_valid_value = true
),

wohngeldhaushalte as (
    select
        geodata_id_nuts,
        year,
        wohngeldhaushalte_je_1000
    from {{ ref('stg_wohngeldhaushalte_kreise') }}
    where has_valid_value = true
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
    a.yoy_change_percent,
    ak.arbeitslose_kreise,
    av.arbeitsvolumen,
    bv.bruttoverdienst,
    da.durchschnittsalter,
    sq.sgb_ii_quote_prozent,
    wg.wohngeldhaushalte_je_1000
from nuts_base n
left join arbeitslos a
    on  a.geodata_id = n.nuts_id
    and a.year       = n.year
left join arbeitslose_kreise ak
    on  ak.geodata_id_nuts = n.nuts_id
    and ak.year            = n.year
left join arbeitsvolumen av
    on  av.geodata_id_nuts = n.nuts_id
    and av.year            = n.year
left join bruttoverdienst bv
    on  bv.geodata_id_nuts = n.nuts_id
    and bv.year            = n.year
left join durchschnittsalter da
    on  da.geodata_id_nuts = n.nuts_id
    and da.year            = n.year
left join sgb_ii_quote sq
    on  sq.geodata_id_nuts = n.nuts_id
    and sq.year            = n.year
left join wohngeldhaushalte wg
    on  wg.geodata_id_nuts = n.nuts_id
    and wg.year            = n.year
