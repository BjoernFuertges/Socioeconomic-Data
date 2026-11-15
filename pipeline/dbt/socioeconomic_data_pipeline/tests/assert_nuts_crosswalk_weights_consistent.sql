-- NUTS-3 version crosswalk: weights must be in one unit and complete.
--   * every row has weight_basis 'population' or 'area' and a non-negative
--     absolute_weight (inhabitants), never a bare area share next to
--     inhabitant counts: one weight_basis per source group, and no piece
--     carries more inhabitants than its whole source region
--     (source_population)
--   * the normalized population_weight of every (source_nuts_id,
--     source_version, target_version) group sums to 1
-- Returns the offending groups; the test fails if any row comes back.

select
    source_nuts_id,
    source_version,
    target_version,
    sum(population_weight)                                   as weight_sum,
    count(distinct weight_basis)                             as n_bases,
    count(*) filter (
        where weight_basis is null
           or weight_basis not in ('population', 'area')
           or absolute_weight is null
           or absolute_weight < 0
           or absolute_weight > coalesce(source_population, 0) * (1 + 1e-9) + 1e-6
    )                                                        as bad_rows
from {{ ref('mart_nuts_version_crosswalk') }}
group by source_nuts_id, source_version, target_version
having abs(coalesce(sum(population_weight), 0) - 1.0) > 1e-6
    or count(distinct weight_basis) != 1
    or count(*) filter (
        where weight_basis is null
           or weight_basis not in ('population', 'area')
           or absolute_weight is null
           or absolute_weight < 0
           or absolute_weight > coalesce(source_population, 0) * (1 + 1e-9) + 1e-6
    ) > 0
