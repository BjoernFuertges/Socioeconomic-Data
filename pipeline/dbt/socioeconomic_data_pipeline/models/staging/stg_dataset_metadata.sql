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
        -- Explicit "unbekannt" status instead of a bare NULL: license/
        -- license_url are only populated once a dataset's actual usage
        -- terms have been researched - most
        -- datasets don't have that yet, and consumers should be able to
        -- tell "not researched" apart from "genuinely no license".
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
