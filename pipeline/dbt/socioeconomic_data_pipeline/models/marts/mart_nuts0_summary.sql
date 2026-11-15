{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregates NUTS-3 data to NUTS-0 level (2-digit country code, e.g. DE).

  Population and Arbeitslose are summed.
  The Schuldnerquote is population-weighted averaged (only rows where both
  values exist; otherwise a simple mean).
  The YoY change is recomputed from the aggregated Arbeitslose figures.
  region_name comes from mart_nuts_geometry_historized (Eurostat NUTS_NAME/
  NAME_LATN, see stg_nuts_geometry), falling back to the code itself if no
  name exists.

  Note: Arbeitslose figures (arbeitslose_kreise) may be incomplete, since
  stg_arbeitslose_kreise does not cover every Kreis in every year (cf.
  mart_nuts3_summary).

  mart_nuts3_summary is harmonized onto a single NUTS-3 boundary version
  (2024, via mart_nuts3_sociodata_harmonized), so the left(nuts_id, n)
  roll-up never mixes Kreise of different NUTS vintages.

  Coverage: the sums are NOT corrected for missing Kreise.
  expected_region_count (Kreise of the region in the reference version,
  int_nuts_expected_kreis_count) vs. the number of Kreise with a value gives
  bevoelkerung_is_complete / arbeitslose_is_complete.
*/

with nuts3 as (
    select * from {{ ref('mart_nuts3_summary') }}
),

nuts0_names as (
    select distinct on (nuts_id) nuts_id, region_name
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code = 0
    order by nuts_id, valid_from desc
),

aggregated as (
    select
        left(nuts3.nuts_id, 2) as nuts_id,
        -- min() instead of group by: the name is constant per nuts0 code
        -- (distinct on in nuts0_names); min() merely passes it through the
        -- group by aggregation despite the join.
        coalesce(min(nuts0_names.region_name), left(nuts3.nuts_id, 2)) as region_name,
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
        sum(arbeitslose_kreise)::numeric as arbeitslose_raw,
        count(bevoelkerung)       as bevoelkerung_region_count,
        count(arbeitslose_kreise) as arbeitslose_region_count
    from nuts3
    left join nuts0_names
        on nuts0_names.nuts_id = left(nuts3.nuts_id, 2)
    group by left(nuts3.nuts_id, 2), year
),

with_yoy as (
    select
        nuts_id,
        region_name,
        year,
        -- geodata_id_ars exists only at Kreis level (mart_nuts3_summary) -
        -- carried as NULL so GetNutsRegionData (backend/internal/handlers/
        -- nuts.go) can use the same selectCols list across all four NUTS
        -- levels.
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
        )                                                                as yoy_change_percent,
        coalesce(expected_region_count, 0)                               as expected_region_count,
        coalesce(expected_region_count > 0
                 and bevoelkerung_region_count = expected_region_count, false) as bevoelkerung_is_complete,
        coalesce(expected_region_count > 0
                 and arbeitslose_region_count = expected_region_count, false)  as arbeitslose_is_complete
    from (
        select a.*, x.expected_region_count
        from aggregated a
        left join {{ ref('int_nuts_expected_kreis_count') }} x
            on  x.nuts_level = 0
            and x.nuts_id    = a.nuts_id
    ) aggregated
    window w as (partition by nuts_id order by year)
)

select * from with_yoy
