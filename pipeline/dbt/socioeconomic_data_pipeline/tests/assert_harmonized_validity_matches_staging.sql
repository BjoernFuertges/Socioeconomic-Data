-- mart_nuts3_sociodata_harmonized.has_valid_value re-states the
-- has_valid_value rule of every stg_<slug>_kreise model (valid_when in its
-- source_datasets list). For rows taken over directly (no crosswalk, target
-- version = the source's geodata_year) both flags must agree; returns the
-- rows where they differ, so a rule changed on one side only fails here.

{% set staging_models = {
    'arbeitslose': 'stg_arbeitslose_kreise',
    'arbeitsvolumen': 'stg_arbeitsvolumen_kreise',
    'auszubildende': 'stg_auszubildende_kreise',
    'bevoelkerung': 'stg_bevoelkerung_kreise',
    'bruttoverdienst': 'stg_bruttoverdienst_kreise',
    'durchschnittsalter': 'stg_durchschnittsalter_kreise',
    'erwerbstaetige_pro_1000_einwohner': 'stg_erwerbstaetige_pro_1000_einwohner_kreise',
    'kinderarmut': 'stg_kinderarmut_kreise',
    'medianeinkommen': 'stg_medianeinkommen_kreise',
    'schuldnerquote': 'stg_schuldnerquote_kreise',
    'sgb_ii_quote': 'stg_sgb_ii_quote_kreise',
    'studierende': 'stg_studierende_kreise',
    'wohngeldhaushalte': 'stg_wohngeldhaushalte_kreise',
} %}

{% for slug, model in staging_models.items() %}
select
    h.dataset_slug,
    h.target_nuts_id,
    h.year,
    h.target_version,
    h.has_valid_value as mart_has_valid_value,
    s.has_valid_value as staging_has_valid_value
from {{ ref('mart_nuts3_sociodata_harmonized') }} h
join {{ ref(model) }} s
    on  s.geodata_id_nuts = h.target_nuts_id
    and s.year            = h.year
    and s.geodata_year    = h.target_version
where h.dataset_slug = '{{ slug }}'
  and not h.is_crosswalked
  and h.has_valid_value is distinct from s.has_valid_value
{% if not loop.last %}union all{% endif %}
{% endfor %}
