{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts3_harm_target on {{ this }} (target_version, target_nuts_id, dataset_slug, year)",
            "create index if not exists idx_mart_nuts3_harm_ds_year on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Harmonisierte sozioökonomische Daten auf mehrere NUTS-3-Zielversionen
  gleichzeitig (siehe target_versions unten), unterscheidbar über die Spalte
  target_version.

  Erreichbar sind nur Versionen, die über mart_nuts_version_crosswalk vom
  jeweiligen Quelljahr (geodata_year) aus in höchstens 2 Hops erreichbar
  sind. Da alle aktuell geladenen Rohdatensätze geodata_year = 2024 führen
  (die Quelle liefert historische Werte bereits rückwirkend auf den
  aktuellen Gebietsstand umgerechnet), sind das ab 2024 aktuell: 2024 selbst
  (0 Hops), 2021 (1 Hop), 2016 (2 Hops via 2021). 2013 bräuchte einen
  3. Hop, den mart_nuts_version_crosswalk nicht berechnet, und ist daher
  bewusst nicht in der Liste.

  Strategie je Zielversion:
  - Regionen, deren geodata_year bereits der Zielversion entspricht → weight = 1,0
  - Alle anderen → Crosswalk aus mart_nuts_version_crosswalk
  - Bei Zusammenlegungen (merge): Werte mehrerer Quell-Regionen werden
    anteilig summiert (harmonized_value = Σ value_i × weight_i)

  is_crosswalked = true   → Wert wurde über Versionsgrenze umgerechnet
  is_crosswalked = false  → Direktwert, kein Umrechnen nötig
*/

{% set target_versions = [2016, 2021, 2024] %}

-- Slugs entsprechen bewusst 1:1 den RouteSlugs der generierten Kreis-API-
-- Endpoints (backend/internal/models/models_soziodata_generated.go), damit
-- das Backend beide Seiten ohne Mapping-Tabelle zusammenführen kann
-- (siehe kreis_generic.go, fill_gaps-Query-Parameter).
{% set source_datasets = [
    {'slug': 'arbeitslose',        'table': 'results_arbeitslose_kreise'},
    {'slug': 'arbeitsvolumen',     'table': 'results_arbeitsvolumen_kreise'},
    {'slug': 'auzubildende',       'table': 'results_auzubildende_kreise'},
    {'slug': 'bevoelkerung',       'table': 'results_bevoelkerung_kreise'},
    {'slug': 'bruttoverdienst',    'table': 'results_bruttoverdienst_kreise'},
    {'slug': 'durchschnittsalter', 'table': 'results_durchschnittsalter_kreise'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'table': 'results_erwerbstaetige_pro_1000_einwohner_kreise'},
    {'slug': 'kinderarmut',        'table': 'results_kinderarmut_kreise'},
    {'slug': 'medianeinkommen',    'table': 'results_medianeinkommen_kreise'},
    {'slug': 'schuldnerquote',     'table': 'results_schuldnerquote_kreise'},
    {'slug': 'sgb_ii_quote',       'table': 'results_sgb_ii_quote_kreise'},
    {'slug': 'studierende',        'table': 'results_studierende_kreise'},
    {'slug': 'wohngeldhaushalte',  'table': 'results_wohngeldhaushalte_kreise'},
] %}

with all_source_data as (

    -- Alle Kreis-Rohdatensätze vereinen (Sources werden mit
    -- write_disposition=replace geladen, daher keine Duplikate je
    -- (geodata_id_nuts, year) zu erwarten)
    {% for ds in source_datasets %}
    select
        '{{ ds.slug }}'       as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer as geodata_year,
        year::integer         as year,
        value::numeric        as value
    from {{ source('loaded_data', ds.table) }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and value           is not null
    union all
    {% endfor %}

    -- beschaeftigungsquote-nuts3 kommt nicht aus einer rohen loaded_data-
    -- Quelle, sondern ist bereits über die Gemeinde(LAU)-Bevölkerung auf
    -- NUTS-3 (2024) aggregiert (siehe stg_beschaeftigungsquote_nuts3.sql /
    -- mart_beschaeftigungsquote_nuts3.sql) - daher ref() statt source().
    select
        'beschaeftigungsquote-nuts3' as dataset_slug,
        geodata_id_nuts,
        geodata_year::integer        as geodata_year,
        year::integer                as year,
        beschaeftigungsquote_prozent::numeric as value
    from {{ ref('stg_beschaeftigungsquote_nuts3') }}
    where geodata_id_nuts is not null
      and geodata_id_nuts != ''
      and beschaeftigungsquote_prozent is not null

),

{% for tv in target_versions %}
crosswalk_{{ tv }} as (
    select *
    from {{ ref('mart_nuts_version_crosswalk') }}
    where target_version = {{ tv }}
),

harmonized_{{ tv }} as (

    -- Datensätze in der Zielversion → direkt übernehmen, kein Crosswalk
    select
        s.dataset_slug,
        s.geodata_id_nuts   as target_nuts_id,
        s.year,
        s.value             as harmonized_value,
        false               as is_crosswalked,
        s.geodata_id_nuts   as source_nuts_id,
        s.geodata_year      as source_version,
        0                   as n_hops
    from all_source_data s
    where s.geodata_year = {{ tv }}

    union all

    -- Datensätze in anderen Versionen → über Crosswalk umrechnen
    select
        s.dataset_slug,
        c.target_nuts_id,
        s.year,
        s.value * c.population_weight as harmonized_value,
        true                          as is_crosswalked,
        s.geodata_id_nuts             as source_nuts_id,
        s.geodata_year                as source_version,
        c.n_hops
    from all_source_data s
    join crosswalk_{{ tv }} c
        on  c.source_nuts_id = s.geodata_id_nuts
        and c.source_version = s.geodata_year
    where s.geodata_year != {{ tv }}

),

-- Zusammenlegungen aufsummieren (mehrere source-Zeilen → eine target-Zeile)
aggregated_{{ tv }} as (
    select
        dataset_slug,
        target_nuts_id,
        year,
        {{ tv }}                as target_version,
        sum(harmonized_value)   as harmonized_value,
        bool_or(is_crosswalked) as is_crosswalked,
        min(source_version)     as source_version,
        max(n_hops)             as n_hops
    from harmonized_{{ tv }}
    group by dataset_slug, target_nuts_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_nuts_id, year
