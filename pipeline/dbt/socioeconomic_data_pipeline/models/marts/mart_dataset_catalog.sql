-- depends_on: {{ ref('stg_dataset_metadata') }}
-- depends_on: {{ ref('mart_socioeconomic_inventory') }}
-- depends_on: {{ ref('stg_dataset_license_history') }}
{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_dataset_catalog_slug on {{ this }} (dataset_slug)"
        ]
    )
}}

/*
  One catalog entry per dataset (provenance, license, metadata, persistent ID):

  - Provenance/license/description come from stg_dataset_metadata (filled
    from meta.yaml by the dlt loader, see load_soziodata_generic.py).
  - coverage_* is aggregated from mart_socioeconomic_inventory so consumers
    (API/frontend) need not join there as well.
  - dataset_uri is a stable identifier derived deterministically from
    dataset_slug as `urn:soziodata:dataset:<slug>` - deliberately NOT a new
    synthetic random/sequence ID. It only changes if the slug is renamed.

  Contains every dataset whose loader upserts a loaded_data.dataset_metadata
  row - besides the meta.yaml-based Kreis/Gemeinden datasets
  (load_soziodata_generic.py) this includes
  arbeitslosenzahlen-afa-agenturbezirke (own upsert in
  load_arbeitslosenzahlen_afa.py, same mechanism, see stg_dataset_metadata.sql).

  Plus synthetic rows for slugs without an own loader: beschaeftigungsquote-
  nuts3 and four slugs that only appear in the combined region export
  (ba_arbeitslose, ba_unterbeschaeftigung,
  lau_beschaeftigungsquote_weighted_avg, lau_population_sum).
*/

with metadata as (
    select * from {{ ref('stg_dataset_metadata') }}
),

-- Unlike all other datasets here, beschaeftigungsquote-nuts3 has no loader
-- or meta.yaml of its own - it is a pure dbt aggregation wrapper over
-- beschaeftigungsquote-gemeinden (see stg_beschaeftigungsquote_nuts3.sql ->
-- int_beschaeftigungsquote_nuts3.sql) and would never get its own
-- loaded_data.dataset_metadata row. Without this synthetic entry the slug
-- would stay invisible in GET /datasets (and the frontend data catalog)
-- although it exists as a dataset_slug via dbt graph discovery in
-- mart_region_datasets.sql/mart_socioeconomic_inventory.sql. Provenance and
-- license are taken 1:1 from the Gemeinden source dataset (same source,
-- just aggregated to NUTS-3).
synthetic_nuts3_metadata as (
    select
        'beschaeftigungsquote-nuts3'::text as dataset_slug,
        'mart_beschaeftigungsquote_nuts3'::text as table_name,
        name || ' (NUTS-3)' as name,
        description || ' Auf NUTS-3-Ebene populationsgewichtet aus den Gemeindewerten aggregiert.' as description,
        description_long,
        unit,
        source,
        license,
        license_url,
        algorithm,
        number_type,
        'NUTS_3'::text as geodata_level,
        geodata_year,
        updated_at
    from metadata
    where dataset_slug = 'beschaeftigungsquote-gemeinden'
),

-- BA slugs from the combined region export; provenance/license are
-- borrowed 1:1 from arbeitslosenzahlen-afa-agenturbezirke.
synthetic_ba_metadata as (
    select
        s.slug::text as dataset_slug,
        'mart_agenturbezirk_sociodata'::text as table_name,
        s.name,
        s.description,
        m.description_long,
        m.unit,
        m.source,
        m.license,
        m.license_url,
        m.algorithm,
        m.number_type,
        null::text as geodata_level,
        m.geodata_year,
        m.updated_at
    from metadata m
    cross join lateral (
        values
            ('ba_arbeitslose', 'Arbeitslose (Agenturbezirk, monatlich)', 'Monatswert der Bundesagentur für Arbeit auf Ebene der Agenturbezirke (nicht auf Kreise umgelegt).'),
            ('ba_unterbeschaeftigung', 'Unterbeschäftigung (Agenturbezirk, monatlich)', 'Monatswert der Bundesagentur für Arbeit auf Ebene der Agenturbezirke (nicht auf Kreise umgelegt).')
    ) as s(slug, name, description)
    where m.dataset_slug = 'arbeitslosenzahlen-afa-agenturbezirke'
),

-- lau_beschaeftigungsquote_weighted_avg: population-weighted average of
-- beschaeftigungsquote-gemeinden
synthetic_lau_quote_metadata as (
    select
        'lau_beschaeftigungsquote_weighted_avg'::text as dataset_slug,
        'mart_nuts3_lau_aggregate'::text as table_name,
        name || ' (NUTS-3, bevölkerungsgewichteter Durchschnitt)' as name,
        description || ' Bevölkerungsgewichteter Durchschnitt aus den Gemeindewerten auf NUTS-3-Ebene aggregiert.' as description,
        description_long,
        unit,
        source,
        license,
        license_url,
        algorithm,
        number_type,
        'NUTS_3'::text as geodata_level,
        geodata_year,
        updated_at
    from metadata
    where dataset_slug = 'beschaeftigungsquote-gemeinden'
),

-- lau_population_sum: sum of lau_integrated.population_count per NUTS-3 region
synthetic_lau_population_metadata as (
    select
        'lau_population_sum'::text as dataset_slug,
        'mart_nuts3_lau_aggregate'::text as table_name,
        'Bevölkerung (NUTS-3, aus Gemeinden aufsummiert)'::text as name,
        'Summe der Gemeinde-Einwohnerzahlen je NUTS-3-Region, als Cross-Check zu den nativen Kreiswerten.'::text as description,
        null::text as description_long,
        'Einwohner'::text as unit,
        'Eurostat LAU-NUTS-Korrespondenztabellen (fetch_lau_population.py)'::text as source,
        'unbekannt'::text as license,
        null::text as license_url,
        null::text as algorithm,
        'absolute'::text as number_type,
        'NUTS_3'::text as geodata_level,
        null::integer as geodata_year,
        null::timestamp as updated_at
),

metadata_with_synthetic as (
    select * from metadata
    union all
    select * from synthetic_nuts3_metadata
    union all
    select * from synthetic_ba_metadata
    union all
    select * from synthetic_lau_quote_metadata
    union all
    select * from synthetic_lau_population_metadata
),

coverage as (
    select
        dataset as dataset_slug,
        min(year) as min_year,
        max(year) as max_year,
        sum(records)::bigint as total_records
    from {{ ref('mart_socioeconomic_inventory') }}
    group by dataset
),

-- When was this dataset's license first observed via snap_dataset_license?
license_tracking as (
    select
        dataset_slug,
        min(valid_from) as license_tracked_since
    from {{ ref('stg_dataset_license_history') }}
    group by dataset_slug
),

final as (
    select
        m.dataset_slug,
        'urn:soziodata:dataset:' || m.dataset_slug as dataset_uri,
        m.name,
        m.description,
        m.description_long,
        m.unit,
        m.source as provenance,
        m.license,
        m.license_url,
        m.algorithm,
        m.number_type,
        m.geodata_level,
        m.geodata_year,
        c.min_year,
        c.max_year,
        c.total_records,
        m.updated_at,
        lt.license_tracked_since,
        -- true if the dataset has years older than the start of license
        -- tracking (license_tracked_since). If that is missing (e.g. dbt run
        -- without a prior dbt snapshot) -> false.
        coalesce(
            c.min_year is not null
            and lt.license_tracked_since is not null
            and c.min_year < extract(year from lt.license_tracked_since)::int,
            false
        ) as license_backfilled
    from metadata_with_synthetic m
    left join coverage c on c.dataset_slug = m.dataset_slug
    left join license_tracking lt on lt.dataset_slug = m.dataset_slug
)

select * from final
