{{
    config(
        materialized='table',
        schema='transformed_data'
    )
}}

/*
  Published table for the NUTS-3 Beschäftigungsquote (LAU -> NUTS-3
  aggregation). The logic, including the weighting approximation, lives in
  intermediate/int_beschaeftigungsquote_nuts3.sql; this mart only exposes it.
*/

select * from {{ ref('int_beschaeftigungsquote_nuts3') }}
