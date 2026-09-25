{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert alle Kreis-grain-Soziodata-Datensätze (siehe source_datasets
  unten) auf Agenturbezirk-Ebene hoch - das Agentur-Pendant zu
  mart_nuts012_dataset_aggregate.sql. Anders als dort (reine
  Präfix-Gruppierung, da NUTS-0/1/2 strikte Obermengen von NUTS-3-Codes
  sind) gibt es zwischen Agenturbezirken und Kreisen keine
  Präfix-Beziehung - hier wird stattdessen über den n:m-Flächenanteils-
  Crosswalk mart_agentur_nuts3_crosswalk gewichtet (area_share_of_agentur),
  der auch den Berlin-Fall (3 Agenturbezirke teilen sich einen Kreis)
  bereits abdeckt.

  Quelle: mart_nuts3_sociodata_harmonized, fixiert auf target_version 2024
  (dieselbe Referenz-NUTS-Version wie mart_agentur_nuts3_crosswalk.sql),
  NICHT die interpolierte/lückengefüllte Version - Aggregate laufen also
  nur über tatsächlich vorhandene Kreis-Werte, keine übernommenen
  Imputationen. source_region_count zeigt, wie viele Kreise pro Jahr
  tatsächlich beigetragen haben.

  measure_type (source_datasets unten) muss inhaltlich mit der
  gleichnamigen Liste in mart_nuts3_sociodata_harmonized.sql
  übereinstimmen - dort die autoritative Quelle, hier dupliziert (gleiches
  Muster wie mart_nuts012_dataset_aggregate.sql).

  Die beiden Flächenanteile aus mart_agentur_nuts3_crosswalk sind NICHT
  austauschbar - 'sum' und 'weighted_avg' brauchen bewusst unterschiedliche
  Gewichte:
    - 'sum' (Zählgrößen, z.B. Bevölkerung): verteilt den Kreis-Wert anteilig
      auf jeden überlappenden Agenturbezirk, gewichtet mit dem Anteil des
      KREISES, der in diesem Agenturbezirk liegt (area_share_of_nuts3) -
      sonst würde z.B. bei einem Kreis, der vollständig in einem
      Agenturbezirk liegt (area_share_of_nuts3 = 1.0), aber selbst nur
      einen kleinen Teil dieses Agenturbezirks ausmacht
      (area_share_of_agentur klein), dessen Bevölkerung fälschlich nur
      anteilig statt vollständig gezählt.
    - 'weighted_avg' (Quoten/Durchschnitte): mittelt die Kreis-Werte.
      Gewicht ist bevorzugt die BEVÖLKERUNG des in diesem Agenturbezirk
      liegenden Kreisanteils (Kreis-Bevölkerung der Quelle 'bevoelkerung' im
      selben Jahr * area_share_of_nuts3) -> aggregation_method
      'weighted_avg_population'. Fehlt für mindestens einen beteiligten
      Kreis die Bevölkerung dieses Jahres, fällt die Zeile komplett auf das
      Flächengewicht area_share_of_agentur zurück -> 'weighted_avg_area'
      (kein Mischen beider Gewichte innerhalb einer Zeile). Bei Berlin (ein
      Kreis, drei Agenturbezirke) ist das Gewicht ohne Wirkung, der Wert
      ist der des einen Kreises.

  LIMITATIONEN (Näherungen, keine exakte Neuberechnung):
    - Aus fertigen Kreis-Quoten lässt sich keine echte Quote des
      Agenturbezirks zurückrechnen (Zähler/Nenner liegen hier nicht vor). Die
      Gesamtbevölkerung ist nur ein Ersatz für die eigentliche Bezugsgröße
      (z.B. Erwachsene, Erwerbsfähige, Kinder).
    - Mediane (value_kind 'median') lassen sich nicht aus Kreis-Medianen
      zusammensetzen - der gewichtete Durchschnitt ist nur eine Schätzung.
    - 'sum'-Werte für Kreise, die sich auf mehrere Agenturbezirke verteilen
      (nur Berlin), sind eine flächenanteilige Aufteilung.
  value_kind ('count' | 'rate' | 'mean' | 'median') und aggregation_method
  gehen unverändert bis in die API/Frontend-Kennzeichnung.
*/

{% set source_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'auszubildende', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg', 'kind': 'median'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'studierende', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg', 'kind': 'rate'},
] %}

with harmonized as (
    select
        dataset_slug,
        target_nuts_id as nuts_id,
        year,
        harmonized_value as value
    from {{ ref('mart_nuts3_sociodata_harmonized') }}
    where target_version = 2024
),

population as (
    select nuts_id, year, value as population
    from harmonized
    where dataset_slug = 'bevoelkerung'
),

crosswalk as (
    select ba_schl, nuts_id, area_share_of_agentur, area_share_of_nuts3
    from {{ ref('mart_agentur_nuts3_crosswalk') }}
),

weighted as (
    select
        c.ba_schl,
        h.dataset_slug,
        h.year,
        h.value,
        p.population,
        -- sum_weight: Anteil des KREISES, der in diesem Agenturbezirk liegt.
        -- avg_weight: Anteil, den dieser Kreis am AGENTURBEZIRK ausmacht (Fläche).
        -- pop_weight: Bevölkerung des in diesem Agenturbezirk liegenden Kreisanteils.
        -- Siehe Kommentar oben, warum diese nicht austauschbar sind.
        c.area_share_of_nuts3                          as sum_weight,
        c.area_share_of_agentur                        as avg_weight,
        p.population * c.area_share_of_nuts3           as pop_weight
    from harmonized h
    join crosswalk c
        on c.nuts_id = h.nuts_id
    left join population p
        on  p.nuts_id = h.nuts_id
        and p.year    = h.year
),

grouped as (
    select
        ba_schl,
        dataset_slug,
        year,
        count(*)                                        as source_region_count,
        sum(value * sum_weight)                         as sum_value,
        sum(value * avg_weight)                         as area_num,
        sum(avg_weight)                                 as area_den,
        avg(value)                                      as plain_avg,
        sum(value * pop_weight)                         as pop_num,
        sum(pop_weight)                                 as pop_den,
        -- Bevölkerungsgewicht nur, wenn ALLE beteiligten Kreise es haben
        (count(pop_weight) = count(*) and coalesce(sum(pop_weight), 0) > 0) as pop_complete
    from weighted
    group by ba_schl, dataset_slug, year
)

select
    ba_schl,
    dataset_slug,
    year,
    source_region_count,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case
                when pop_complete then pop_num / pop_den
                when area_den > 0 then area_num / area_den
                else plain_avg
            end
            {% else %}
            sum_value
            {% endif %}
    {% endfor %}
    end as value,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case when pop_complete then 'weighted_avg_population' else 'weighted_avg_area' end
            {% else %}
            'sum'
            {% endif %}
    {% endfor %}
    end as aggregation_method,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then '{{ ds.kind }}'
    {% endfor %}
    end as value_kind
from grouped
