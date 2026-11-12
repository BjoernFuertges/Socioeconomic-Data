{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_dataset_source_slug on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Source data of all meta.yaml-based datasets (Kreis and Gemeinde level) in
  one uniform long format, for the download via GET /datasets/{slug}/source.

  These are the values exactly as they come from the geo-mapper subproject
  (mapped_pairs.csv -> loaded_data): no NUTS harmonization, no
  interpolation, no aggregation. The mapping is limited to assigning region
  names to geodata IDs (geodata_id_*, mapper, parameter). This lets every
  value of the harmonized marts be traced back to its input.

  Deliberately reads from loaded_data (the state dlt last loaded), not from
  the snapshots - history is the snapshots' job. Not included:
  arbeitslosenzahlen-afa-agenturbezirke (own structure with monthly values,
  available via /agenturbezirke) and the derived slugs
  (beschaeftigungsquote-nuts3, lau_*, ba_*).

  New Kreis/Gemeinde dataset -> add an entry to source_datasets here
  (analogous to mart_nuts3_sociodata_harmonized.sql).
*/

{% set source_datasets = [
    {'slug': 'arbeitslose',                        'table': 'results_arbeitslose_kreise'},
    {'slug': 'arbeitsvolumen',                     'table': 'results_arbeitsvolumen_kreise'},
    {'slug': 'auszubildende',                      'table': 'results_auszubildende_kreise'},
    {'slug': 'beschaeftigungsquote-gemeinden',     'table': 'results_beschaeftigungsquote_gemeinden'},
    {'slug': 'bevoelkerung',                       'table': 'results_bevoelkerung_kreise'},
    {'slug': 'bruttoverdienst',                    'table': 'results_bruttoverdienst_kreise'},
    {'slug': 'durchschnittsalter',                 'table': 'results_durchschnittsalter_kreise'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner',  'table': 'results_erwerbstaetige_pro_1000_einwohner_kreise'},
    {'slug': 'kinderarmut',                        'table': 'results_kinderarmut_kreise'},
    {'slug': 'medianeinkommen',                    'table': 'results_medianeinkommen_kreise'},
    {'slug': 'schuldnerquote',                     'table': 'results_schuldnerquote_kreise'},
    {'slug': 'sgb_ii_quote',                       'table': 'results_sgb_ii_quote_kreise'},
    {'slug': 'studierende',                        'table': 'results_studierende_kreise'},
    {'slug': 'wohngeldhaushalte',                  'table': 'results_wohngeldhaushalte_kreise'},
] %}

{% for ds in source_datasets %}
{% set rel = source('loaded_data', ds.table) %}
{% set cols = adapter.get_columns_in_relation(rel) | map(attribute='name') | map('lower') | list %}
select
    '{{ ds.slug }}'::text as dataset_slug,
    regexp_replace(cast(original_id_1 as text), '\.0+$', '') as original_id_1,
    original_name,
    geodata_name,
    {% if 'geodata_id_nuts' in cols %}nullif(cast(geodata_id_nuts as text), ''){% else %}null::text{% endif %} as geodata_id_nuts,
    {% if 'geodata_id_lau' in cols %}nullif(cast(geodata_id_lau as text), ''){% else %}null::text{% endif %} as geodata_id_lau,
    {% if 'geodata_id_ars' in cols %}nullif(cast(geodata_id_ars as text), ''){% else %}null::text{% endif %} as geodata_id_ars,
    mapper,
    parameter,
    geodata_level,
    geodata_year::integer as geodata_year,
    year::integer as year,
    value::numeric as value
from {{ rel }}
where value is not null
{% if not loop.last %}union all{% endif %}
{% endfor %}
