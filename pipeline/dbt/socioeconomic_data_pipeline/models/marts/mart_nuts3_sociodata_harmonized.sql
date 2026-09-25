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

  mart_nuts_version_crosswalk berechnet mittlerweile für JEDES Paar der in
  stg_nuts_geometry vorhandenen Vintage-Jahre eine direkte (1-Hop)
  Abbildung (siehe int_nuts_spatial_crosswalk) - jede dort vorhandene
  Version ist daher von jeder anderen aus direkt erreichbar, unabhängig vom
  geodata_year der Quelldaten. target_versions listet deshalb einfach alle
  aktuell verfügbaren NUTS-3-Vintage-Jahre.

  Strategie je Zielversion:
  - Regionen, deren geodata_year bereits der Zielversion entspricht → weight = 1,0
  - Alle anderen → Crosswalk aus mart_nuts_version_crosswalk
  - Bei Zusammenlegungen (merge): Werte mehrerer Quell-Regionen werden
    anteilig summiert (harmonized_value = Σ value_i × weight_i)

  is_crosswalked = true   → Wert wurde über Versionsgrenze umgerechnet
  is_crosswalked = false  → Direktwert, kein Umrechnen nötig

  measure_type je Datensatz (source_datasets unten, plus
  beschaeftigungsquote-nuts3 separat) steuert die Aggregationsformel bei
  Zusammenlegungen mehrerer Quellregionen in eine Zielregion:
    - 'sum':          harmonized_value = Σ value_i * weight_i
                       (richtig für Zählgrößen, z. B. Bevölkerung)
    - 'weighted_avg':  harmonized_value = Σ value_i * absolute_weight_i
                                            / Σ absolute_weight_i
                       (richtig für Quoten/Durchschnitte - ein reines
                       Aufsummieren würde hier falsche, zu kleine Werte
                       erzeugen, siehe absolute_weight-Kommentar in
                       mart_nuts_version_crosswalk.sql)
*/

{% set target_versions = [2003, 2006, 2010, 2013, 2016, 2021, 2024] %}

-- Slugs entsprechen bewusst 1:1 den RouteSlugs der generierten Kreis-API-
-- Endpoints (backend/internal/models/models_soziodata_generated.go), damit
-- das Backend beide Seiten ohne Mapping-Tabelle zusammenführen kann
-- (siehe kreis_generic.go, fill_gaps-Query-Parameter). measure_type wurde
-- anhand der Spaltenbeschreibungen/meta.yaml je Datensatz eingeordnet
-- (Zählgröße = 'sum', Quote/Durchschnitt = 'weighted_avg').
{% set source_datasets = [
    {'slug': 'arbeitslose',        'table': 'results_arbeitslose_kreise', 'measure_type': 'sum'},
    {'slug': 'arbeitsvolumen',     'table': 'results_arbeitsvolumen_kreise', 'measure_type': 'sum'},
    {'slug': 'auszubildende',      'table': 'results_auszubildende_kreise', 'measure_type': 'sum'},
    {'slug': 'bevoelkerung',       'table': 'results_bevoelkerung_kreise', 'measure_type': 'sum'},
    {'slug': 'bruttoverdienst',    'table': 'results_bruttoverdienst_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'durchschnittsalter', 'table': 'results_durchschnittsalter_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'erwerbstaetige_pro_1000_einwohner', 'table': 'results_erwerbstaetige_pro_1000_einwohner_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'kinderarmut',        'table': 'results_kinderarmut_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'medianeinkommen',    'table': 'results_medianeinkommen_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'schuldnerquote',     'table': 'results_schuldnerquote_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'sgb_ii_quote',       'table': 'results_sgb_ii_quote_kreise', 'measure_type': 'weighted_avg'},
    {'slug': 'studierende',        'table': 'results_studierende_kreise', 'measure_type': 'sum'},
    {'slug': 'wohngeldhaushalte',  'table': 'results_wohngeldhaushalte_kreise', 'measure_type': 'sum'},
] %}
{% set beschaeftigungsquote_nuts3_measure_type = 'weighted_avg' %}

with all_source_data as (

    -- Alle Kreis-Rohdatensätze vereinen. Sources werden mit
    -- write_disposition=replace geladen (siehe pipeline/dlt/load_soziodata_generic.py),
    -- daher keine Duplikate je (geodata_id_nuts, year) zu erwarten - DISTINCT
    -- ist trotzdem defensiv gesetzt: sum()-Measure-Types (siehe measure_type
    -- unten) würden jede versehentlich erneut geladene Dopplung stillschweigend
    -- mitaufsummieren, ohne dass das an falsch hohen Werten offensichtlich wäre.
    {% for ds in source_datasets %}
    select distinct
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
        s.value,
        1.0::numeric        as weight,
        1.0::numeric        as absolute_weight,
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
        s.value,
        c.population_weight as weight,
        c.absolute_weight,
        true                 as is_crosswalked,
        s.geodata_id_nuts    as source_nuts_id,
        s.geodata_year       as source_version,
        c.n_hops
    from all_source_data s
    join crosswalk_{{ tv }} c
        on  c.source_nuts_id = s.geodata_id_nuts
        and c.source_version = s.geodata_year
    where s.geodata_year != {{ tv }}

),

-- Zusammenlegungen aufsummieren bzw. gewichtet mitteln (siehe measure_type
-- oben; mehrere source-Zeilen → eine target-Zeile)
aggregated_{{ tv }} as (
    select
        dataset_slug,
        target_nuts_id,
        year,
        {{ tv }} as target_version,
        case dataset_slug
        {% for ds in source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        {% endfor %}
            when 'beschaeftigungsquote-nuts3' then
                {% if beschaeftigungsquote_nuts3_measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        end                      as harmonized_value,
        bool_or(is_crosswalked)  as is_crosswalked,
        min(source_version)      as source_version,
        max(n_hops)              as n_hops
    from harmonized_{{ tv }}
    group by dataset_slug, target_nuts_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_nuts_id, year
