{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_{{ this.name }}_geodata_year on {{ this }} (geodata_id_nuts, year)",
            "create index if not exists idx_{{ this.name }}_nuts_tpo on {{ this }} (geodata_id_nuts text_pattern_ops)"
        ]
    )
}}

/*
  Wrapper view over mart_beschaeftigungsquote_nuts3 for API exposure.
  Selects the weighted average as the primary metric for REST API.
*/

select
    beschaeftigungsquote_nuts3_pk,
    geodata_id_nuts as original_id_1,
    coalesce(geodata_name, geodata_id_nuts) as original_name,
    coalesce(geodata_name, geodata_id_nuts) as geodata_name,
    geodata_id_nuts,
    ''::text as geodata_id_ars,
    'aggregation_lau_to_nuts3'::text as mapper,
    'population_weighted_and_median'::text as parameter,
    'NUTS_3'::text as geodata_level,
    2024::integer as geodata_year,
    year,
    observation_date,
    case
        when beschaeftigungsquote_gewichtet is null and beschaeftigungsquote_ungewichtet is null then false
        else true
    end as has_valid_value,
    beschaeftigungsquote_gewichtet as beschaeftigungsquote_prozent,
    beschaeftigungsquote_median,
    beschaeftigungsquote_ungewichtet,
    gemeinden_count
from {{ ref('mart_beschaeftigungsquote_nuts3') }}
