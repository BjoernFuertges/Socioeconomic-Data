select
    dataset_slug,
    license,
    license_url,
    dbt_valid_from::date as valid_from,
    dbt_valid_to::date as valid_to
from {{ ref('snap_dataset_license') }}
