{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

with staged_data as (
    select * from {{ ref('stg_arbeitslosenzahlen') }}
),

metadata as (
    select * from {{ source('loaded_data', 'arbeitslosenzahlen_metadata') }}
),

enriched as (
    select
        s.dataset_id,
        s.geodata_id,
        s.geodata_name,
        s.year,
        s.month,
        s.quarter,
        s.observation_date,
        s.time_type,
        s.arbeitslose_numeric as arbeitslose,
        s.load_date,
        
        -- Add metadata fields for context
        m.title as dataset_title,
        m.series as dataset_series,
        m.source_url,
        m.geodata_level,
        
        -- Add row number for detecting duplicates
        row_number() over (
            partition by s.geodata_id, s.observation_date 
            order by s.load_date desc
        ) as rn
        
    from staged_data s
    left join metadata m on s.dataset_id = m.dataset_id
),

deduplicated as (
    -- Keep only the most recent load if duplicates exist
    select * from enriched
    where rn = 1
),

final as (
    select
        dataset_id,
        geodata_id,
        geodata_name,
        year,
        month,
        quarter,
        observation_date,
        time_type,
        arbeitslose,
        load_date,
        dataset_title,
        dataset_series,
        source_url,
        geodata_level,
        
        -- Calculate year-over-year change (where applicable)
        lag(arbeitslose, 1) over (
            partition by geodata_id, month 
            order by observation_date
        ) as arbeitslose_previous_year,
        
        -- Calculate percentage change
        case 
            when lag(arbeitslose, 1) over (
                partition by geodata_id, month 
                order by observation_date
            ) > 0 then
                round(
                    (arbeitslose - lag(arbeitslose, 1) over (
                        partition by geodata_id, month 
                        order by observation_date
                    )) * 100.0 / lag(arbeitslose, 1) over (
                        partition by geodata_id, month 
                        order by observation_date
                    ), 
                    2
                )
            else null
        end as yoy_change_percent,
        
        -- Add data quality flag
        case
            when arbeitslose < 0 then 'NEGATIVE_VALUE'
            when arbeitslose is null then 'NULL_VALUE'
            when geodata_id is null then 'MISSING_GEODATA'
            when observation_date is null then 'MISSING_DATE'
            else 'VALID'
        end as data_quality_flag
        
    from deduplicated
)

select * from final
where data_quality_flag = 'VALID'  -- Only include valid records with LAU reference
