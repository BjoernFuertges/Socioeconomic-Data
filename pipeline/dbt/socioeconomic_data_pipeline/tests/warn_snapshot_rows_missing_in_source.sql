{{ config(severity='warn') }}

-- Warns about Kreis rows that are still current in a snap_results_*_kreise
-- snapshot (dbt_valid_to is null) but no longer exist in the corresponding
-- loaded_data.results_*_kreise table. hard_deletes is 'ignore' on
-- purpose (dbt_project.yml), so such rows stay in staging and in every mart
-- built on it (mart_nuts3_sociodata_harmonized) - typically after a dataset
-- was re-mapped to other geodata_id_nuts. Returns one row per dataset with
-- the number of stale keys; decide manually whether to close them.
--
-- The snapshot list is read from the dbt graph (every snap_results_*_kreise
-- with a matching source), so a new Kreis dataset is covered automatically.
-- Relations are taken from the graph instead of ref()/source(), which cannot
-- be called conditionally; hence the test has no selectable parents.

{% set pairs = [] %}
{% if execute %}
    {% for node in graph.nodes.values()
         if node.resource_type == 'snapshot'
         and node.name.startswith('snap_results_')
         and node.name.endswith('_kreise') %}
        {% set src = graph.sources.get('source.' ~ project_name ~ '.loaded_data.' ~ node.name[5:]) %}
        {% if src %}
            {% do pairs.append((node.name[5:], node.relation_name, src.relation_name)) %}
        {% endif %}
    {% endfor %}
{% endif %}

{% if pairs %}
{% for table, snap_relation, src_relation in pairs %}
select
    '{{ table }}' as source_table,
    count(*)      as stale_current_rows
from {{ snap_relation }} s
where s.dbt_valid_to is null
  and not exists (
      select 1
      from {{ src_relation }} r
      where r.geodata_id_nuts = s.geodata_id_nuts
        and r.year::text      = s.year::text
  )
having count(*) > 0
{% if not loop.last %}union all{% endif %}
{% endfor %}
{% else %}
select null::text as source_table, null::bigint as stale_current_rows
where false
{% endif %}
