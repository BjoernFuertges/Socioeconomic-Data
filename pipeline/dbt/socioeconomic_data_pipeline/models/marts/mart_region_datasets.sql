-- depends_on: {{ ref('stg_arbeitslose_kreise') }}
-- depends_on: {{ ref('stg_arbeitsvolumen_kreise') }}
-- depends_on: {{ ref('stg_auszubildende_kreise') }}
-- depends_on: {{ ref('stg_beschaeftigungsquote_gemeinden') }}
-- depends_on: {{ ref('stg_beschaeftigungsquote_nuts3') }}
-- depends_on: {{ ref('stg_bevoelkerung_kreise') }}
-- depends_on: {{ ref('stg_bruttoverdienst_kreise') }}
-- depends_on: {{ ref('stg_durchschnittsalter_kreise') }}
-- depends_on: {{ ref('stg_erwerbstaetige_pro_1000_einwohner_kreise') }}
-- depends_on: {{ ref('stg_kinderarmut_kreise') }}
-- depends_on: {{ ref('stg_medianeinkommen_kreise') }}
-- depends_on: {{ ref('stg_schuldnerquote_kreise') }}
-- depends_on: {{ ref('stg_sgb_ii_quote_kreise') }}
-- depends_on: {{ ref('stg_studierende_kreise') }}
-- depends_on: {{ ref('stg_wohngeldhaushalte_kreise') }}
{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_region_datasets_nuts_id on {{ this }} (geodata_id_nuts)",
            "create index if not exists idx_mart_region_datasets_nuts_slug on {{ this }} (geodata_id_nuts, dataset_slug)"
        ]
    )
}}

{% set datasets = [] %}
{% for node in graph.nodes.values() | sort(attribute='name') %}
    {% if node.resource_type == 'model'
            and node.package_name == model.package_name
          and node.name.startswith('stg_')
          and (node.name.endswith('_kreise')
               or node.name.endswith('_gemeinden')
               or node.name.endswith('_nuts3')
               or node.name.endswith('_lau')) %}
        {% set slug = node.name[4:] %}
        {% set slug = slug
            | replace('_kreise', '')
            | replace('_gemeinden', '-gemeinden')
            | replace('_nuts3', '-nuts3')
            | replace('_lau', '-lau') %}

        {% do datasets.append({"slug": slug, "model": node.name}) %}
    {% endif %}
{% endfor %}

select distinct geodata_id_nuts, dataset_slug, year
from (
    {% for ds in datasets %}
    select
        geodata_id_nuts,
        '{{ ds.slug }}'::text as dataset_slug,
        year
    from {{ ref(ds.model) }}
    where geodata_id_nuts is not null and geodata_id_nuts != ''
    {% if not loop.last %}
    union all
    {% endif %}
    {% endfor %}
) combined
order by geodata_id_nuts, dataset_slug, year
