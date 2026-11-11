{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_agentur_nuts_membership_agentur on {{ this }} (ba_schl)",
            "create index if not exists idx_mart_agentur_nuts_membership_nuts on {{ this }} (nuts_id)"
        ]
    )
}}

{% set reference_nuts_version = 2024 %}
{% set containment_threshold = 0.99 %}

/*
  Placement of Agenturbezirke in the NUTS hierarchy (NUTS-1/2/3), so one
  can navigate between Agenturbezirk and Kreis/Regierungsbezirk/Bundesland.

  One row per (Agenturbezirk, NUTS region), for every NUTS level 1-3 that
  overlaps the Agenturbezirk. Derived from mart_agentur_nuts3_crosswalk via
  the prefix-hierarchical NUTS codes (DE300 -> DE30 -> DE3) - no additional
  geometry computation.

  area_share_of_agentur: share of the Agenturbezirk's area in this region.
  Normalized to the sum of the Agenturbezirk's crosswalk intersection
  areas, so filtered-out overlap slivers and areas outside Germany do not
  push the share below the threshold.

  is_fully_contained: area_share_of_agentur >= {{ containment_threshold }}
  (tolerance for boundary inaccuracies between BA and NUTS geometries).

  is_parent: the smallest level (highest levl_code) that fully contains the
  Agenturbezirk. At most one row per Agenturbezirk - missing if no level
  contains it fully (Agenturbezirk spanning Bundesländer borders).

  Berlin: all three Agenturbezirke lie fully within the NUTS-3 Kreis DE300,
  which is their parent.
*/

with crosswalk as (
    select ba_schl, nuts_id, intersection_area_m2
    from {{ ref('mart_agentur_nuts3_crosswalk') }}
),

agentur_total as (
    select ba_schl, sum(intersection_area_m2) as total_area_m2
    from crosswalk
    group by ba_schl
),

per_level as (
    select ba_schl, 3 as nuts_level, nuts_id as region_nuts_id,
           sum(intersection_area_m2) as area_m2
    from crosswalk
    group by ba_schl, nuts_id

    union all

    select ba_schl, 2, left(nuts_id, 4), sum(intersection_area_m2)
    from crosswalk
    group by ba_schl, left(nuts_id, 4)

    union all

    select ba_schl, 1, left(nuts_id, 3), sum(intersection_area_m2)
    from crosswalk
    group by ba_schl, left(nuts_id, 3)
),

nuts_names as (
    select distinct on (nuts_id)
        nuts_id,
        region_name
    from {{ ref('mart_nuts_geometry_historized') }}
    where levl_code in (1, 2, 3)
      and country_code = 'DE'
      and valid_from <= {{ reference_nuts_version }}
      and (valid_to is null or valid_to >= {{ reference_nuts_version }})
    order by nuts_id, valid_from desc
),

shares as (
    select
        pl.ba_schl,
        pl.region_nuts_id as nuts_id,
        pl.nuts_level,
        n.region_name,
        pl.area_m2 / nullif(t.total_area_m2, 0) as area_share_of_agentur
    from per_level pl
    join agentur_total t using (ba_schl)
    left join nuts_names n on n.nuts_id = pl.region_nuts_id
),

flagged as (
    select
        *,
        area_share_of_agentur >= {{ containment_threshold }} as is_fully_contained
    from shares
)

select
    ba_schl,
    nuts_id,
    nuts_level,
    region_name,
    area_share_of_agentur,
    is_fully_contained,
    is_fully_contained
        and nuts_level = max(nuts_level) filter (where is_fully_contained) over (partition by ba_schl)
        as is_parent
from flagged
