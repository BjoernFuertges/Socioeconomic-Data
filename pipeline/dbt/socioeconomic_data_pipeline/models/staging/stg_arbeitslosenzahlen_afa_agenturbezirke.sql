{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_{{ this.name }}_agentur_period on {{ this }} (geodata_id_agentur, year, month)"
        ]
    )
}}

-- Kein dbt snapshot (anders als stg_arbeitslose_kreise): die Quelle liefert
-- bei jedem Lauf bereits die volle Monatsreihe neu (write_disposition=
-- replace), Historisierung der jüngsten "vorläufigen" Revisionen ist
-- bewusst spätere Erweiterung (siehe Planung, Phase 2).

with source_data as (
    select * from {{ source('loaded_data', 'results_arbeitslosenzahlen_afa_agenturbezirke') }}
),

cleaned as (
    select
        geodata_id_agentur,
        geodata_name,
        year::integer as year,
        month::integer as month,
        arbeitslose,
        unterbeschaeftigung,
        make_date(year::integer, month::integer, 1) as observation_date
    from source_data
),

final as (
    select
        {{ dbt_utils.generate_surrogate_key(['geodata_id_agentur', 'year', 'month']) }} as arbeitslosenzahlen_afa_pk,
        *
    from cleaned
)

select * from final
