-- mart_nuts3_summary pivots mart_nuts3_sociodata_harmonized (one
-- target_version) into one row per (nuts_id, year). A duplicate here means
-- the pivot mixed boundary versions or source rows, and GetNutsRegionData
-- (single-year lookup without ORDER BY tie-breaker) would return an
-- arbitrary one.
select nuts_id, year, count(*) as n_rows
from {{ ref('mart_nuts3_summary') }}
group by nuts_id, year
having count(*) > 1
