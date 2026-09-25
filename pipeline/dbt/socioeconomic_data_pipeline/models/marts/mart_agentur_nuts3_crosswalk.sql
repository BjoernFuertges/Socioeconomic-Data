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

{% set reference_nuts_version = 2024 %}
{% set min_share = var('crosswalk_min_share', 0.10) %}

/*
  n:m-Zuordnung Agenturbezirk <-> NUTS-3-Region mit Flächenanteil in beide
  Richtungen. Ersetzt eine frühere Version dieses Crosswalks, die pro
  Agenturbezirk nur die eine NUTS-3-Region mit der größten Schnittfläche
  behielt - das unterschlug, dass Agenturbezirke in aller Regel aus
  MEHREREN ganzen Kreisen bestehen (und umgekehrt, Ausnahme Berlin: 3
  Agenturbezirke teilen sich den einzigen NUTS-3-Kreis Berlin).

  area_share_of_agentur: welcher Anteil dieses Agenturbezirks (Fläche) auf
  diese NUTS-3-Region entfällt - Zeilen eines Agenturbezirks summieren
  sich näherungsweise auf 1.0 ("woraus besteht dieser Agenturbezirk").

  area_share_of_nuts3: umgekehrt, welcher Anteil dieser NUTS-3-Region auf
  diesen Agenturbezirk entfällt - Zeilen einer NUTS-3-Region summieren
  sich näherungsweise auf 1.0 (relevant für Berlin: je Agenturbezirk hier
  deutlich unter 1.0).

  Da nicht ausreichend Bevölkerungsdaten vorhanden sind, geht es nur nach Fläche.

  Kleine geometrische Überlappungs-"Krümel" durch Grenzungenauigkeiten
  werden über einen Schwellwert (10% Flächenanteil, mindestens einer der
  beiden Anteile muss darüber liegen) herausgefiltert. Empirisch ermittelt
  (siehe analysis/data-analysis/agentur_nuts3_crosswalk_shares.ipynb): der jeweils
  größere der beiden Anteile liegt bei Krümeln höchstens bei ~6% (bei
  kleinen Kreisfreien Städten kann ein Krümel von ~1-2 km² schon mehrere %
  der Stadtfläche ausmachen, z.B. 0.7% bei Stuttgart), echte Bestandteile
  liegen bei >= ~28%. 10% liegt sicher in dieser Lücke, ohne dass ein
  Agenturbezirk oder Kreis seine Zuordnung komplett verliert (auch nicht
  in Berlin). Für die Analyse im Notebook lässt sich der Filter mit
  `--vars '{crosswalk_min_share: 0}'` abschalten. Frühere Schwellen von 0.2% / 0.5% ließen noch sichtbare
  Krümel durch (Anzeige "0 %" im Frontend).
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
