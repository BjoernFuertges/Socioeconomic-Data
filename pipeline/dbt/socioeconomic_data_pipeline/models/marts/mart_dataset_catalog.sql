-- depends_on: {{ ref('stg_dataset_metadata') }}
-- depends_on: {{ ref('mart_socioeconomic_inventory') }}
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

  Datensätze ohne meta.yaml-Eintrag (aktuell: die AfA-Arbeitslosenzahlen,
  die einen eigenen, älteren Metadaten-Mechanismus haben) sind hier bewusst NICHT
  enthalten
*/

with metadata as (
    select * from {{ ref('stg_dataset_metadata') }}
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
        m.updated_at
    from metadata m
    left join coverage c on c.dataset_slug = m.dataset_slug
)

select * from final
