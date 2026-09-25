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
  Quelldaten aller meta.yaml-basierten Datensätze (Kreis- und Gemeinde-
  Ebene) in einem einheitlichen Long-Format, für den Download über
  GET /datasets/{slug}/source.

  Es sind die Werte, wie sie aus dem geo-mapper-Teilprojekt kommen
  (mapped_pairs.csv -> loaded_data), unverändert: keine NUTS-Harmonisierung,
  keine Interpolation, keine Aggregation. Das Mapping beschränkt sich auf
  die Zuordnung der Regionsnamen zu den Geodaten-IDs (geodata_id_*, mapper,
  parameter). Damit lässt sich jeder Wert der harmonisierten Marts auf
  seine Eingabe zurückführen.

  Liest bewusst aus loaded_data (aktueller Stand, wie ihn dlt zuletzt
  geladen hat), nicht aus den Snapshots - die Historie bleibt Sache der
  Snapshots. Nicht enthalten: arbeitslosenzahlen-afa-agenturbezirke (eigene
  Struktur mit Monatswerten, über /agenturbezirke abrufbar) sowie die
  abgeleiteten Slugs (beschaeftigungsquote-nuts3, lau_*, ba_*).

  Neuer Kreis-/Gemeinden-Datensatz -> hier einen Eintrag in source_datasets
  ergänzen (analog mart_nuts3_sociodata_harmonized.sql).
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
    regexp_replace(cast(original_id_1 as text), '\\.0+$', '') as original_id_1,
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
