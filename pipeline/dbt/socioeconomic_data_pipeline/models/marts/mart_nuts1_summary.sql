{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert NUTS-3-Daten auf NUTS-1-Ebene (3-stelliger NUTS-Code, z. B. DE1).

  Bevölkerung und Arbeitslose werden summiert.
  Die Schuldnerquote wird bevölkerungsgewichtet gemittelt (nur Zeilen, bei denen
  beide Werte vorhanden sind; andernfalls einfaches Mittel).
  Die YoY-Veränderung wird aus den aggregierten Arbeitslosenzahlen neu berechnet.
  Als region_name wird mangels übergeordneter Namen der NUTS-1-Code selbst verwendet.

  Hinweis: Arbeitslosenzahlen können unvollständig sein, da die Arbeitsagenturbezirke
  nicht 1:1 den NUTS-Kreisen entsprechen (vgl. mart_nuts3_summary).
*/

with nuts3 as (
    select * from {{ ref('mart_nuts3_summary') }}
),

aggregated as (
    select
        left(nuts_id, 3)    as nuts_id,
        left(nuts_id, 3)    as region_name,
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
        sum(arbeitslose)    as arbeitslose_raw
    from nuts3
    group by left(nuts_id, 3), year
),

with_yoy as (
    select
        nuts_id,
        region_name,
        year,
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
