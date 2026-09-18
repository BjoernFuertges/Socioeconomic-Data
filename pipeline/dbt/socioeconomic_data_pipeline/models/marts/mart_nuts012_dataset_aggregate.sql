{{
    config(
        materialized='view',
        schema='transformed_data'
    )
}}

/*
  Aggregiert alle Kreis-grain-Soziodata-Datensätze (siehe source_datasets
  unten) auf NUTS-0/1/2-Ebene hoch - analog zu mart_nuts0/1/2_summary.sql
  (die dasselbe bisher nur für Bevölkerung/Schuldnerquote/Arbeitslose tun),
  hier aber für den kompletten Datensatz-Katalog. Macht Auswahl/Chart/
  Tabelle (DatasetPicker.vue) auch auf Land-/Bundesland-/
  Regierungsbezirksebene nutzbar, nicht nur auf Kreisebene - siehe
  GetNutsDatasetAggregateTimeseries (backend/internal/handlers/nuts.go).

  Quelle: mart_nuts3_sociodata_harmonized, fixiert auf target_version 2024
  (aktuellste Kreis-Gebietsstruktur - analog zur festen target_version in
  mart_nuts3_lau_aggregate.sql), NICHT die interpolierte/lückengefüllte
  Version (mart_nuts3_timeseries_interpolated): Aggregate laufen also nur
  über tatsächlich vorhandene Kreis-Werte, keine übernommenen
  Imputationen. source_region_count zeigt, wie viele Kreise pro Jahr
  tatsächlich beigetragen haben.

  measure_type (source_datasets unten) muss inhaltlich mit der
  gleichnamigen Liste in mart_nuts3_sociodata_harmonized.sql
  übereinstimmen - dort die autoritative Quelle, hier dupliziert, weil dbt
  Jinja-Variablen nicht modellübergreifend geteilt werden (gleiches Muster
  wie mart_lau_sociodata_harmonized.sql vs. backend/cmd/gen-models/main.go).
  'sum' summiert unverändert; 'weighted_avg' mittelt bevölkerungsgewichtet
  (Gewicht: mart_nuts3_summary.bevoelkerung desselben Jahres, Kreise ohne
  Bevölkerungswert werden ausgeschlossen statt mit Gewicht 0 verzerrend
  einzufließen) - ein reiner Mittelwert würde große und kleine Kreise
  gleich gewichten.
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

population as (
    select nuts_id, year, bevoelkerung
    from {{ ref('mart_nuts3_summary') }}
    where bevoelkerung is not null
),

weighted as (
    select
        h.dataset_slug,
        h.nuts_id,
        h.year,
        h.value,
        p.bevoelkerung as weight
    from harmonized h
    left join population p
        on  p.nuts_id = h.nuts_id
        and p.year    = h.year
),

{% for lvl, prefix_len in [(0, 2), (1, 3), (2, 4)] %}
level_{{ lvl }} as (
    select
        {{ lvl }} as nuts_level,
        left(nuts_id, {{ prefix_len }}) as nuts_id,
        dataset_slug,
        year,
        count(*) as source_region_count,
        case dataset_slug
        {% for ds in source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                case
                    when sum(case when weight is not null and weight > 0 then weight else 0 end) > 0
                        then sum(case when weight is not null and weight > 0 then value * weight else 0 end)
                             / nullif(sum(case when weight is not null and weight > 0 then weight else 0 end), 0)
                    else avg(value)
                end
                {% else %}
                sum(value)
                {% endif %}
        {% endfor %}
        end as value
    from weighted
    group by left(nuts_id, {{ prefix_len }}), dataset_slug, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for lvl, prefix_len in [(0, 2), (1, 3), (2, 4)] %}
select * from level_{{ lvl }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
