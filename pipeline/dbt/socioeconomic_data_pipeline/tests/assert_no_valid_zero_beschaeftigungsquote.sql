-- A Beschaeftigungsquote of exactly 0 is the source's placeholder for
-- "not defined / missing", never a measured rate. It must be flagged
-- has_valid_value = false in staging and must not survive into the LAU
-- harmonization. Returns every violating row.

select
    'stg' as layer,
    original_id_1 as region_key,
    year
from {{ ref('stg_beschaeftigungsquote_gemeinden') }}
where has_valid_value
  and beschaeftigungsquote_prozent = 0

union all

select
    'harmonized' as layer,
    target_gisco_id as region_key,
    year
from {{ ref('mart_lau_sociodata_harmonized') }}
where dataset_slug = 'beschaeftigungsquote-gemeinden'
  and harmonized_value = 0
