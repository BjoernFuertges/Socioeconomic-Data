with source_data as (
    select * from {{ source('loaded_data', 'dataset_metadata') }}
),

cleaned as (
    select
        dataset_slug,
        table_name,
        name,
        description,
        description_long,
        unit,
        source,
        -- 'unbekannt' (unknown) instead of NULL: most datasets have no
        -- researched license yet, and consumers must be able to tell
        -- "not researched" apart from "no license".
        coalesce(nullif(trim(license), ''), 'unbekannt') as license,
        nullif(trim(license_url), '') as license_url,
        algorithm,
        number_type,
        geodata_level,
        geodata_year::integer as geodata_year,
        updated_at::timestamp as updated_at
    from source_data
)

select * from cleaned
