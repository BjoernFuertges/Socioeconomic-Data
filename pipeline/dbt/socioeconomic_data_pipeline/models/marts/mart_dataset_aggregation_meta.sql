{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

/*
  Aggregationsmetadaten je Indikator für die Custom-Region-Abfrage
  (POST /custom-regions/dataset-aggregate). Eine Zeile je (dataset_slug,
  indicator):

    level        - Ebene der Quelldaten: 'lau' | 'nuts3' | 'agentur'
    measure_type - 'sum' (Zählgröße) | 'weighted_avg' (Quote/Durchschnitt)
    kind         - 'count' | 'rate' | 'mean' | 'median' (Kennzeichnung der
                   Näherung, gleiche Bedeutung wie in
                   mart_agentur_dataset_aggregate)
    finer_slug   - Slug desselben Indikators auf feinerer Ebene, falls es
                   einen gibt (nuts3 -> lau). Das Backend nimmt die feinere
                   Variante, sofern sie für die Region Daten hat, sonst diese.

  Die NUTS-3-Liste muss inhaltlich mit den source_datasets in
  mart_nuts3_sociodata_harmonized.sql / mart_agentur_dataset_aggregate.sql
  übereinstimmen (dort autoritativ, hier dupliziert - gleiches Muster wie
  in den anderen Aggregat-Marts). Die LAU-Zeile mit
  mart_lau_sociodata_harmonized.sql.

  Agenturbezirke sind nur in Berlin feiner als NUTS-3. Die monatlichen
  BA-Zahlen existieren aber ausschließlich auf Agenturbezirks-Ebene (keine
  NUTS-3-Entsprechung), deshalb ist 'agentur' hier deren einzige Quelle -
  unabhängig vom Bundesland.
*/

{% set nuts3_datasets = [
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
    {'slug': 'beschaeftigungsquote-nuts3', 'measure_type': 'weighted_avg', 'kind': 'rate', 'finer_slug': 'beschaeftigungsquote-gemeinden'},
] %}

{% set lau_datasets = [
    {'slug': 'beschaeftigungsquote-gemeinden', 'measure_type': 'weighted_avg', 'kind': 'rate'},
] %}

{% set agentur_datasets = [
    {'slug': 'arbeitslosenzahlen-afa-agenturbezirke', 'indicator': 'arbeitslose', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'arbeitslosenzahlen-afa-agenturbezirke', 'indicator': 'unterbeschaeftigung', 'measure_type': 'sum', 'kind': 'count'},
] %}

select * from (
    values
    {% for ds in nuts3_datasets %}
        ('{{ ds.slug }}', '{{ ds.slug }}', 'nuts3', '{{ ds.measure_type }}', '{{ ds.kind }}',
         {% if ds.finer_slug %}'{{ ds.finer_slug }}'{% else %}null{% endif %}),
    {% endfor %}
    {% for ds in lau_datasets %}
        ('{{ ds.slug }}', '{{ ds.slug }}', 'lau', '{{ ds.measure_type }}', '{{ ds.kind }}', null),
    {% endfor %}
    {% for ds in agentur_datasets %}
        ('{{ ds.slug }}', '{{ ds.indicator }}', 'agentur', '{{ ds.measure_type }}', '{{ ds.kind }}', null){% if not loop.last %},{% endif %}
    {% endfor %}
) as t (dataset_slug, indicator, level, measure_type, kind, finer_slug)
