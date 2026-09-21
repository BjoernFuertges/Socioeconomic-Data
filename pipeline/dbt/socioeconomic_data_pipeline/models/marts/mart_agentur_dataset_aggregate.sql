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
    - 'weighted_avg' (Quoten/Durchschnitte): mittelt die Kreis-Werte,
      gewichtet mit dem Anteil, den jeder Kreis am AGENTURBEZIRK ausmacht
      (area_share_of_agentur) - das ist der Anteil, mit dem jeder Kreis
      tatsächlich zum Gesamtbild des Agenturbezirks beiträgt.
  Keine Bevölkerungsgewichtung - siehe Kommentar in
  mart_agentur_nuts3_crosswalk.sql (an dieser Join-Granularität nicht
  ausreichend Bevölkerungsdaten vorhanden).
*/

{% set source_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'sum'},
    {'slug': 'auszubildende', 'measure_type': 'sum'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'weighted_avg'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg'},
    {'slug': 'studierende', 'measure_type': 'sum'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'sum'},
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg'},
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
        -- sum_weight: Anteil des KREISES, der in diesem Agenturbezirk liegt.
        -- avg_weight: Anteil, den dieser Kreis am AGENTURBEZIRK ausmacht.
        -- Siehe Kommentar oben, warum diese beiden nicht austauschbar sind.
        c.area_share_of_nuts3    as sum_weight,
        c.area_share_of_agentur  as avg_weight
    from harmonized h
    join crosswalk c
        on c.nuts_id = h.nuts_id
)

select
    ba_schl,
    dataset_slug,
    year,
    count(*) as source_region_count,
    case dataset_slug
    {% for ds in source_datasets %}
        when '{{ ds.slug }}' then
            {% if ds.measure_type == 'weighted_avg' %}
            case
                when sum(avg_weight) > 0
                    then sum(value * avg_weight) / nullif(sum(avg_weight), 0)
                else avg(value)
            end
            {% else %}
            sum(value * sum_weight)
            {% endif %}
    {% endfor %}
    end as value
from weighted
group by ba_schl, dataset_slug, year
