{% snapshot snap_dataset_license %}
{{ config(
    target_schema='snapshots',
    strategy='check',
    check_cols=['license', 'license_url'],
    unique_key='dataset_slug',
    invalidate_hard_deletes=false
) }}
select dataset_slug, license, license_url
from {{ source('loaded_data', 'dataset_metadata') }}
{% endsnapshot %}
