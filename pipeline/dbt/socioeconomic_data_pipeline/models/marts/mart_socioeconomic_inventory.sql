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
-- depends_on: {{ ref('stg_arbeitslosenzahlen_afa_agenturbezirke') }}
{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_socio_inventory_dataset_year_level on {{ this }} (dataset, year, geodata_level)",
            "create index if not exists idx_mart_socio_inventory_year_level on {{ this }} (year, geodata_level)"
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

        {% do datasets.append({"slug": slug, "model": node.name, "geodata_level": none}) %}
    {% endif %}
{% endfor %}

{#
  stg_arbeitslosenzahlen_afa_agenturbezirke is Agenturbezirk grain (own
  geodata_id_agentur column, no geodata_level, no geodata_id_nuts/lau/ars)
  and so matches none of the naming suffixes above. Without this entry the
  dataset would have no min_year/max_year/total_records in
  mart_dataset_catalog (see the comment there). geodata_level is set as a
  literal here because the source has no such column.
#}
{% do datasets.append({
    "slug": "arbeitslosenzahlen-afa-agenturbezirke",
    "model": "stg_arbeitslosenzahlen_afa_agenturbezirke",
    "geodata_level": "AGENTUR"
}) %}

with inventory as (
    {% for ds in datasets %}
    select
        '{{ ds.slug }}'::text as dataset,
        year,
        {% if ds.geodata_level %}'{{ ds.geodata_level }}'::text{% else %}geodata_level{% endif %} as geodata_level,
        count(*)::int as records,
        count(distinct nullif(to_jsonb(s)->>'geodata_id_nuts', ''))::int as distinct_geodata_nuts,
        count(distinct nullif(to_jsonb(s)->>'geodata_id_lau', ''))::int as distinct_geodata_lau,
        count(distinct nullif(to_jsonb(s)->>'geodata_id_ars', ''))::int as distinct_geodata_ars
    from {{ ref(ds.model) }} s
    group by year{% if not ds.geodata_level %}, geodata_level{% endif %}
    {% if not loop.last %}
    union all
    {% endif %}
    {% endfor %}
)

select * from inventory
