-- Every (source Gemeinde, source version, target version) group of the LAU
-- version crosswalk must distribute exactly 100 % of the source value.
-- Returns the offending groups; the test fails if any row comes back.

select
    source_gisco_id,
    source_version,
    target_version,
    sum(weight) as weight_sum
from {{ ref('mart_lau_version_crosswalk') }}
group by source_gisco_id, source_version, target_version
having abs(sum(weight) - 1.0) > 1e-6
