{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

/*
  Aggregation metadata per indicator for the custom-region query
  (POST /custom-regions/dataset-aggregate). One row per (dataset_slug,
  indicator):

    level        - level of the source data: 'lau' | 'nuts3' | 'agentur'
    measure_type - 'sum' (count) | 'weighted_avg' (ratio/average) |
                   'weighted_avg_employees' (value per employed person,
                   weighted by employees; see custom_region.go)
    kind         - 'count' | 'rate' | 'mean' | 'median' (labels the
                   approximation, same meaning as in
                   mart_agentur_dataset_aggregate)
    finer_slug   - slug of the same indicator at a finer level, if there is
                   one (nuts3 -> lau). The backend uses the finer variant if
                   it has data for the region, otherwise this one.

  The NUTS-3 list must match the source_datasets in
  mart_nuts3_sociodata_harmonized.sql / mart_agentur_dataset_aggregate.sql
  (authoritative there, duplicated here - same pattern as in the other
  aggregate marts). The LAU row mirrors mart_lau_sociodata_harmonized.sql.

  Agenturbezirke are only finer than NUTS-3 in Berlin. The monthly BA
  figures, however, exist only at Agenturbezirk level (no NUTS-3
  counterpart), so 'agentur' is their only source here - regardless of
  Bundesland.
*/

{% set nuts3_datasets = [
    {'slug': 'arbeitslose', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'arbeitsvolumen', 'measure_type': 'weighted_avg_employees', 'kind': 'mean'},
    {'slug': 'auszubildende', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'bevoelkerung', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'bruttoverdienst', 'measure_type': 'weighted_avg_employees', 'kind': 'mean'},
    {'slug': 'durchschnittsalter', 'measure_type': 'weighted_avg', 'kind': 'mean'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'kinderarmut', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'medianeinkommen', 'measure_type': 'weighted_avg', 'kind': 'median'},
    {'slug': 'schuldnerquote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sgb_ii_quote', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'studierende', 'measure_type': 'weighted_avg', 'kind': 'rate'},
    {'slug': 'sv_beschaeftigte', 'measure_type': 'sum', 'kind': 'count'},
    {'slug': 'wohngeldhaushalte', 'measure_type': 'weighted_avg', 'kind': 'rate'},
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
