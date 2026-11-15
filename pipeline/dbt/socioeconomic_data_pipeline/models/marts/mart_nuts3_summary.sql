{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregates all available socioeconomic indicators per NUTS-3 region and year.

  All values come from mart_nuts3_sociodata_harmonized on ONE NUTS-3
  boundary version (target_version below), never from the staging models
  directly: the Kreis datasets may be delivered on different NUTS vintages
  (geodata_year), and NUTS-3 IDs are not stable across them (AGENTS.md).
  Reading staging per geodata_year would put e.g. a 2016-vintage dataset
  next to 2024-vintage ones in the same row, and mart_nuts0/1/2_summary
  (left(nuts_id, n)) would roll up mixed boundaries. Datasets already on
  the target version pass through the harmonized mart unchanged (weight 1.0).

  Base: rows with a valid population OR Schuldnerquote (formerly a FULL
  OUTER JOIN of the two NUTS-native datasets). Further indicators are
  attached where available (their temporal coverage varies).

  Only harmonized values with has_valid_value = true are included.
  Column names/types are a contract with GetNutsRegionData
  (backend/internal/handlers/nuts.go) and mart_nuts012_dataset_aggregate.
*/

{% set target_version = var('reference_nuts_version') %}

with harmonized as (
    select
        target_nuts_id as nuts_id,
        year,
        dataset_slug,
        harmonized_value::double precision as value
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = {{ target_version }}
      and has_valid_value
),

pivoted as (
    -- (target_version, target_nuts_id, dataset_slug, year) is unique in the
    -- harmonized mart, so max() only pivots, it never picks between values.
    select
        nuts_id,
        year,
        max(value) filter (where dataset_slug = 'bevoelkerung')       as bevoelkerung,
        max(value) filter (where dataset_slug = 'schuldnerquote')     as schuldnerquote_prozent,
        max(value) filter (where dataset_slug = 'arbeitslose')        as arbeitslose_kreise,
        max(value) filter (where dataset_slug = 'arbeitsvolumen')     as arbeitsvolumen,
        max(value) filter (where dataset_slug = 'bruttoverdienst')    as bruttoverdienst,
        max(value) filter (where dataset_slug = 'durchschnittsalter') as durchschnittsalter,
        max(value) filter (where dataset_slug = 'sgb_ii_quote')       as sgb_ii_quote_prozent,
        max(value) filter (where dataset_slug = 'wohngeldhaushalte')  as wohngeldhaushalte_je_1000,
        max(value) filter (where dataset_slug = 'sv_beschaeftigte')   as sv_beschaeftigte,
        max(value) filter (where dataset_slug = 'beschaeftigungsquote-nuts3') as beschaeftigungsquote_prozent
    from harmonized
    group by nuts_id, year
),

-- The median cannot be crosswalked (not additive), so it is only taken
-- where the series is already on the target version (true today: it is
-- aggregated from the Gemeinden onto NUTS 2024, see
-- int_beschaeftigungsquote_nuts3).
beschaeftigungsquote_median as (
    select
        geodata_id_nuts as nuts_id,
        year,
        beschaeftigungsquote_median
    from {{ ref('stg_beschaeftigungsquote_nuts3') }}
    where has_valid_value = true
      and geodata_year = {{ target_version }}
),

-- Name and ARS are region metadata, not time series: taken per nuts_id from
-- the NUTS-native staging rows that are on the target version (ARS is null
-- for ~20% of Kreise, hence max() over both sources and all years). Regions
-- that only appear via a crosswalk fall back to the Eurostat NUTS name.
region_meta as (
    select
        geodata_id_nuts as nuts_id,
        max(geodata_name) as region_name,
        max(nullif(cast(geodata_id_ars as text), '')) as geodata_id_ars
    from (
        select geodata_id_nuts, geodata_name, geodata_id_ars
        from {{ ref('stg_bevoelkerung_kreise') }}
        where has_valid_value = true and geodata_year = {{ target_version }}
        union all
        select geodata_id_nuts, geodata_name, geodata_id_ars
        from {{ ref('stg_schuldnerquote_kreise') }}
        where has_valid_value = true and geodata_year = {{ target_version }}
    ) s
    group by geodata_id_nuts
),

nuts3_names as (
    select distinct on (nuts_id) nuts_id, region_name
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code = 3
    order by nuts_id, valid_from desc
),

joined as (
    select
        p.nuts_id,
        coalesce(rm.region_name, n3.region_name, p.nuts_id)::varchar as region_name,
        p.year,
        rm.geodata_id_ars,
        p.bevoelkerung,
        p.schuldnerquote_prozent,
        p.arbeitslose_kreise,
        -- arbeitslose is an alias of arbeitslose_kreise (not a replacement,
        -- see mart_nuts0/1/2_summary.sql, which still reference
        -- arbeitslose_kreise): GetNutsRegionData (backend/internal/handlers/
        -- nuts.go) uses the same selectCols list for all four NUTS levels
        -- and expects the column name "arbeitslose", not "arbeitslose_kreise".
        p.arbeitslose_kreise::numeric as arbeitslose,
        p.arbeitsvolumen,
        p.bruttoverdienst,
        p.durchschnittsalter,
        p.sgb_ii_quote_prozent,
        p.wohngeldhaushalte_je_1000,
        p.sv_beschaeftigte,
        -- round(.., 2): same scale as the source (int_beschaeftigungsquote_nuts3
        -- rounds to 2 decimals), lost by the harmonized mart's double precision.
        round(p.beschaeftigungsquote_prozent::numeric, 2) as beschaeftigungsquote_prozent,
        bm.beschaeftigungsquote_median
    from pivoted p
    left join region_meta rm
        on rm.nuts_id = p.nuts_id
    left join nuts3_names n3
        on n3.nuts_id = p.nuts_id
    left join beschaeftigungsquote_median bm
        on  bm.nuts_id = p.nuts_id
        and bm.year    = p.year
    where p.bevoelkerung is not null
       or p.schuldnerquote_prozent is not null
)

-- yoy_change_percent is computed here (as mart_nuts0/1/2_summary do from
-- their own aggregation); GetNutsRegionData selects it for NUTS-3 as well
-- (single year and ?all=true series).
select
    *,
    round(
        case
            when lag(arbeitslose) over w is not null
                 and lag(arbeitslose) over w != 0
            then (arbeitslose - lag(arbeitslose) over w)
                 / lag(arbeitslose) over w * 100
            else null
        end,
        2
    ) as yoy_change_percent
from joined
window w as (partition by nuts_id order by year)
