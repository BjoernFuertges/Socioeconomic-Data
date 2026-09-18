{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert NUTS-3-Daten auf NUTS-2-Ebene (4-stelliger NUTS-Code, z. B. DE11).

  Bevölkerung und Arbeitslose werden summiert.
  Die Schuldnerquote wird bevölkerungsgewichtet gemittelt (nur Zeilen, bei denen
  beide Werte vorhanden sind; andernfalls einfaches Mittel).
  Die YoY-Veränderung wird aus den aggregierten Arbeitslosenzahlen neu berechnet.
  region_name kommt aus mart_nuts_geometry_historized (Eurostat NUTS_NAME/
  NAME_LATN, siehe stg_nuts_geometry) - Fallback auf den Code selbst, falls dort
  kein Name vorliegt.

  Hinweis: Arbeitslosenzahlen (arbeitslose_kreise) können unvollständig sein, da
  stg_arbeitslose_kreise nicht jedes Jahr für jeden Kreis abdeckt (vgl. mart_nuts3_summary).
*/

with nuts3 as (
    select * from {{ ref('mart_nuts3_summary') }}
),

nuts2_names as (
    select distinct on (nuts_id) nuts_id, region_name
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code = 2
    order by nuts_id, valid_from desc
),

aggregated as (
    select
        left(nuts3.nuts_id, 4) as nuts_id,
        -- min() statt group by: der Name ist pro nuts2-Code konstant (dank
        -- distinct on in nuts2_names), min() ist hier nur ein zulässiger
        -- Weg, ihn trotz des Joins innerhalb der group by-Aggregation
        -- durchzureichen.
        coalesce(min(nuts2_names.region_name), left(nuts3.nuts_id, 4)) as region_name,
        year,
        sum(bevoelkerung)   as bevoelkerung,
        case
            when sum(
                case
                    when schuldnerquote_prozent is not null
                         and bevoelkerung is not null
                         and bevoelkerung > 0
                    then bevoelkerung else 0
                end
            ) > 0
                then sum(
                         case
                             when schuldnerquote_prozent is not null
                                  and bevoelkerung is not null
                                  and bevoelkerung > 0
                             then schuldnerquote_prozent * bevoelkerung else 0
                         end
                     )
                     / nullif(
                         sum(
                             case
                                 when schuldnerquote_prozent is not null
                                      and bevoelkerung is not null
                                      and bevoelkerung > 0
                                 then bevoelkerung else 0
                             end
                         ),
                         0
                     )
            else avg(schuldnerquote_prozent)
        end                 as schuldnerquote_prozent,
        sum(arbeitslose_kreise)::numeric as arbeitslose_raw
    from nuts3
    left join nuts2_names
        on nuts2_names.nuts_id = left(nuts3.nuts_id, 4)
    group by left(nuts3.nuts_id, 4), year
),

with_yoy as (
    select
        nuts_id,
        region_name,
        year,
        -- geodata_id_ars gibt es nur auf Kreisebene (mart_nuts3_summary) -
        -- als NULL mitgeführt, damit GetNutsRegionData (backend/internal/
        -- handlers/nuts.go) dieselbe selectCols-Spaltenliste über alle vier
        -- NUTS-Ebenen hinweg verwenden kann.
        null::text as geodata_id_ars,
        bevoelkerung,
        schuldnerquote_prozent,
        arbeitslose_raw                                                  as arbeitslose,
        round(
            case
                when lag(arbeitslose_raw) over w is not null
                     and lag(arbeitslose_raw) over w != 0
                then (arbeitslose_raw - lag(arbeitslose_raw) over w)
                     / lag(arbeitslose_raw) over w * 100
                else null
            end,
            2
        )                                                                as yoy_change_percent
    from aggregated
    window w as (partition by nuts_id order by year)
)

select * from with_yoy
