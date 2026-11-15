/*
  Expected number of Kreise (NUTS-3 regions) per NUTS-0/1/2 region in the
  reference NUTS version (var reference_nuts_version) - the denominator of
  the coverage flags (expected_region_count / is_complete) of the Kreis
  roll-ups mart_nuts012_dataset_aggregate and mart_nuts0/1/2_summary.
  NUTS codes are hierarchical, so a NUTS-n region contains exactly the
  NUTS-3 codes starting with its own code (prefix length 2/3/4).
*/

with reference_kreise as (
    select distinct nuts_id
    from {{ ref('int_nuts3_de_geometry') }}
    where version_year = {{ var('reference_nuts_version') }}
)

{% for lvl, prefix_len in [(0, 2), (1, 3), (2, 4)] %}
select
    {{ lvl }}                       as nuts_level,
    left(nuts_id, {{ prefix_len }}) as nuts_id,
    count(*)                        as expected_region_count
from reference_kreise
group by left(nuts_id, {{ prefix_len }})
{% if not loop.last %}union all{% endif %}
{% endfor %}
