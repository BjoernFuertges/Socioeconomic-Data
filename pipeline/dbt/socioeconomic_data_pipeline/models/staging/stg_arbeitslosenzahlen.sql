{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

with source_data as (
    select * from {{ source('loaded_data', 'arbeitslosenzahlen_agentur_arbeit') }}
),

cleaned as (
    select
        -- Remove _dlt* fields by explicitly selecting only needed columns
        dataset_id,
        geodata_id,
        geodata_name,
        mapper,
        parameter,
        time_column,
        is_date,
        arbeitslose,
        year,
        month,
        day,
        time_type,
        load_date,
        
        -- Create a proper date field from year, month, day
        case 
            when time_type = 'monthly' and year is not null and month is not null then
                make_date(year::integer, month::integer, coalesce(day::integer, 1))
            when time_type = 'yearly' and year is not null then
                make_date(year::integer, 1, 1)
            else null
        end as observation_date,
        
        -- Cast arbeitslose to numeric for calculations
        cast(arbeitslose as numeric) as arbeitslose_numeric,
        
        -- Extract quarter for aggregations
        case 
            when month is not null then
                case 
                    when month between 1 and 3 then 1
                    when month between 4 and 6 then 2
                    when month between 7 and 9 then 3
                    when month between 10 and 12 then 4
                end
            else null
        end as quarter

    from source_data
),

final as (
    select
        -- Generate surrogate key AFTER observation_date is calculated
        {{ dbt_utils.generate_surrogate_key(['dataset_id', 'geodata_id', 'observation_date']) }} as unemployment_pk,
        *
    from cleaned
)

select * from final
