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
  Ein Katalog-Eintrag pro Datensatz- (Herkunft, Lizenz, Metadaten, persistente ID):

  - Provenance/Lizenz/Beschreibung kommt aus stg_dataset_metadata (dort
    aus meta.yaml über den dlt-Loader befüllt, siehe
    load_soziodata_generic.py).
  - coverage_* kommt aggregiert aus mart_socioeconomic_inventory, damit
    Konsumenten (API/Frontend) nicht zusätzlich dorthin joinen müssen.
  - dataset_uri ist eine deterministisch aus dataset_slug abgeleitete,
    stabile Kennung nach dem Schema `urn:soziodata:dataset:<slug>` -
    bewusst KEINE neue künstliche Zufalls-/Sequence-ID. Sie ändert sich nur,
    wenn der Slug selbst umbenannt wird.

  Enthält jeden Datensatz, dessen Loader eine loaded_data.dataset_metadata-
  Zeile upserted - das schließt neben den meta.yaml-basierten Kreis-/
  Gemeinden-Datensätzen (load_soziodata_generic.py) auch
  arbeitslosenzahlen-afa-agenturbezirke ein (eigener Upsert in
  load_arbeitslosenzahlen_afa.py, gleicher Mechanismus, siehe stg_dataset_metadata.sql).

  Zusätzlich vier synthetische Zeilen (analog zu beschaeftigungsquote-
  nuts3 unten) für Slugs, die nur im kombinierten Regions-Export
  auftauchen
*/

with metadata as (
    select * from {{ ref('stg_dataset_metadata') }}
),

-- beschaeftigungsquote-nuts3 hat (anders als alle anderen Datensätze hier)
-- keinen eigenen Loader/keine eigene meta.yaml - es ist ein reiner
-- dbt-Aggregations-Wrapper über beschaeftigungsquote-gemeinden (siehe
-- stg_beschaeftigungsquote_nuts3.sql -> mart_beschaeftigungsquote_nuts3.sql)
-- und bekäme daher nie eine eigene loaded_data.dataset_metadata-Zeile.
-- Ohne diesen synthetischen Eintrag bliebe der Slug für GET /datasets (und
-- damit den Datenkatalog im Frontend) dauerhaft unsichtbar, obwohl er über
-- die dbt-Graph-Discovery in mart_region_datasets.sql/
-- mart_socioeconomic_inventory.sql längst als eigener dataset_slug
-- existiert und dort Daten hat. Übernimmt Provenance/Lizenz 1:1 vom
-- Gemeinden-Quelldatensatz (dieselbe Quelle, nur auf NUTS-3 aggregiert).
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

-- Vier weitere Slugs, die nur im kombinierten Regions-Export auftauchen
-- Lizenz werden 1:1 von dort übernommen.
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

-- lau_beschaeftigungsquote_weighted_avg: bevölkerungsgewichteter
-- Durchschnitt aus beschaeftigungsquote-gemeinden
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

-- lau_population_sum: Summe von lau_integrated.population_count je NUTS-3-Region 
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

-- Wann wurde die Lizenz dieses Datensatzes erstmals per snap_dataset_license beobachtet?
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
        -- true, wenn der Datensatz Jahre enthält, die älter sind als der Beginn des Lizenz-Trackings (license_tracked_since). Fehlt license_tracked_since
        -- (z. B. dbt run ohne vorherigen dbt snapshot) --> false (sicher ist sicher).
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
