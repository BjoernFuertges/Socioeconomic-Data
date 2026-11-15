{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_agentur_nuts3_crosswalk_agentur on {{ this }} (ba_schl)",
            "create index if not exists idx_mart_agentur_nuts3_crosswalk_nuts3 on {{ this }} (nuts_id)"
        ]
    )
}}

{% set reference_nuts_version = var('reference_nuts_version') %}
{% set min_share = var('crosswalk_min_share', 0.10) %}

/*
  n:m mapping Agenturbezirk <-> NUTS-3 region with area shares in both
  directions. Agenturbezirke usually consist of SEVERAL whole Kreise (and
  vice versa; exception Berlin: 3 Agenturbezirke share the single NUTS-3
  Kreis Berlin), so keeping only the largest-overlap region would be wrong.

  area_share_of_agentur: share of this Agenturbezirk's area falling into
  this NUTS-3 region - rows of one Agenturbezirk sum to ~1.0 ("what does
  this Agenturbezirk consist of").

  area_share_of_nuts3: conversely, share of this NUTS-3 region falling into
  this Agenturbezirk - rows of one NUTS-3 region sum to ~1.0 (relevant for
  Berlin: well below 1.0 per Agenturbezirk).

  Population data is insufficient, so weighting is by area only.

  Small geometric overlap slivers caused by boundary inaccuracies are
  filtered out via a threshold (10% area share; at least one of the two
  shares must exceed it). Determined empirically (see
  analysis/data-analysis/agentur_nuts3_crosswalk_shares.ipynb): the larger
  of the two shares is at most ~6% for slivers (for small kreisfreie Städte
  a ~1-2 km2 sliver can already be several % of the city area, e.g. 0.7% for
  Stuttgart), while real components are >= ~28%. 10% lies safely in that
  gap without any Agenturbezirk or Kreis losing its assignment entirely
  (not even in Berlin). For notebook analysis the filter can be disabled
  with `--vars '{crosswalk_min_share: 0}'`. Earlier thresholds of 0.2% /
  0.5% still let visible slivers through (shown as "0 %" in the frontend).
*/

with agentur_geom as (
    select
        ba_schl,
        ST_Transform(geometry_wgs84, 3035) as geometry_3035
    from {{ ref('stg_agenturbezirk_geometry') }}
    where has_valid_geometry
),

nuts_de as (
    select
        nuts_id,
        region_name,
        geometry_3035
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code    = 3
      and country_code = 'DE'
      and has_valid_geometry
      and valid_from <= {{ reference_nuts_version }}
      and (valid_to is null or valid_to >= {{ reference_nuts_version }})
),

area_pairs as (
    select
        a.ba_schl,
        n.nuts_id,
        n.region_name,
        ST_Area(ST_Intersection(a.geometry_3035, n.geometry_3035)) as intersection_area_m2,
        ST_Area(a.geometry_3035) as agentur_area_m2,
        ST_Area(n.geometry_3035) as nuts3_area_m2
    from agentur_geom a
    join nuts_de n
        on ST_Intersects(a.geometry_3035, n.geometry_3035)
),

final as (
    select
        ap.ba_schl,
        ap.nuts_id,
        ap.region_name,
        ap.intersection_area_m2,
        ap.agentur_area_m2,
        ap.nuts3_area_m2,
        ap.intersection_area_m2 / nullif(ap.agentur_area_m2, 0) as area_share_of_agentur,
        ap.intersection_area_m2 / nullif(ap.nuts3_area_m2, 0) as area_share_of_nuts3
    from area_pairs ap
)

select *
from final
where area_share_of_agentur > {{ min_share }}
   or area_share_of_nuts3 > {{ min_share }}
