{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_harm_target on {{ this }} (target_version, target_gisco_id, dataset_slug, year)",
            "create index if not exists idx_mart_lau_harm_ds_year on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Harmonisierte LAU-Datensätze auf mehrere LAU-Zielversionen gleichzeitig
  (target_version), analog zu mart_nuts3_sociodata_harmonized.sql - siehe
  dortigen Kommentar für die grundsätzliche Strategie (direkt vs.
  gecrosswalkt, Zusammenlegungen).

  measure_type je Datensatz (lau_source_datasets unten) steuert die
  Aggregationsformel bei Zusammenlegungen mehrerer Quellregionen in eine
  Zielregion:
    - 'sum':          harmonized_value = Σ value_i * weight_i
                       (richtig für Zählgrößen, z. B. Bevölkerung)
    - 'weighted_avg':  harmonized_value = Σ value_i * absolute_weight_i
                                            / Σ absolute_weight_i
                       (richtig für Quoten/Durchschnitte - ein reines
                       Aufsummieren würde hier falsche, zu kleine Werte
                       erzeugen, siehe absolute_weight-Kommentar in
                       mart_lau_version_crosswalk.sql)

  Region-Key ist gisco_id (konsistent mit Geometrie-/Crosswalk-Seite).
  stg_beschaeftigungsquote_gemeinden führt aktuell keine gisco_id, sondern
  amtliche Gemeindeschlüssel (geodata_id_ars/geodata_id_lau) - die Auflösung
  auf gisco_id erfolgt hier über stg_lau_population.lau_nat_code (dieselbe
  Verknüpfung, die stg_beschaeftigungsquote_gemeinden.sql bereits nutzt, um
  umgekehrt geodata_id_nuts/geodata_id_ars aus der Populationsquelle zu
  befüllen). Setzt voraus, dass loaded_data.lau_population tatsächlich
  gisco_id führt - dafür muss fetch_lau_population.py gelaufen sein
  (Teil von run_all_pipelines.sh), sonst fällt load_geodata_to_postgis.py
  auf einen Fallback ohne gisco_id/Bevölkerung zurück und dieser Mart
  bleibt leer, obwohl er baut (siehe dlt/CLAUDE.md).
*/

{% set target_versions = [2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024] %}

-- Muss 1:1 mit harmonizedLauRouteSlugs in backend/cmd/gen-models/main.go
-- übereinstimmen (siehe Kommentar dort).
{% set lau_source_datasets = [
    {'slug': 'beschaeftigungsquote-gemeinden', 'model': 'stg_beschaeftigungsquote_gemeinden', 'value_col': 'beschaeftigungsquote_prozent', 'measure_type': 'weighted_avg'},
] %}

with gisco_lookup as (
    select distinct on (lau_nat_code)
        regexp_replace(cast(lau_nat_code as text), '\.0+$', '') as ars_code,
        gisco_id
    from {{ ref('stg_lau_population') }}
    where lau_nat_code is not null and gisco_id is not null
    order by lau_nat_code, year desc
),

all_source_data as (
    {% for ds in lau_source_datasets %}
    select
        '{{ ds.slug }}'        as dataset_slug,
        gl.gisco_id            as gisco_id,
        s.geodata_year::integer as geodata_year,
        s.year::integer        as year,
        s.{{ ds.value_col }}::numeric as value
    from {{ ref(ds.model) }} s
    join gisco_lookup gl
        on gl.ars_code = coalesce(s.geodata_id_ars, s.geodata_id_lau)
    where s.{{ ds.value_col }} is not null
    {% if not loop.last %}union all{% endif %}
    {% endfor %}
),

{% for tv in target_versions %}
crosswalk_{{ tv }} as (
    select *
    from {{ ref('mart_lau_version_crosswalk') }}
    where target_version = {{ tv }}
),

harmonized_{{ tv }} as (

    -- Datensätze in der Zielversion → direkt übernehmen, kein Crosswalk
    select
        s.dataset_slug,
        s.gisco_id     as target_gisco_id,
        s.year,
        s.value,
        1.0::numeric   as weight,
        1.0::numeric   as absolute_weight,
        false          as is_crosswalked,
        s.gisco_id     as source_gisco_id,
        s.geodata_year as source_version,
        0              as n_hops
    from all_source_data s
    where s.geodata_year = {{ tv }}

    union all

    -- Datensätze in anderen Versionen → über Crosswalk umrechnen
    select
        s.dataset_slug,
        c.target_gisco_id,
        s.year,
        s.value,
        c.weight,
        c.absolute_weight,
        true            as is_crosswalked,
        s.gisco_id      as source_gisco_id,
        s.geodata_year  as source_version,
        1               as n_hops
    from all_source_data s
    join crosswalk_{{ tv }} c
        on  c.source_gisco_id = s.gisco_id
        and c.source_version  = s.geodata_year
    where s.geodata_year != {{ tv }}

),

-- Zusammenlegungen aufsummieren bzw. gewichtet mitteln (siehe measure_type
-- oben; mehrere source-Zeilen → eine target-Zeile)
aggregated_{{ tv }} as (
    select
        dataset_slug,
        target_gisco_id,
        year,
        {{ tv }} as target_version,
        case dataset_slug
        {% for ds in lau_source_datasets %}
            when '{{ ds.slug }}' then
                {% if ds.measure_type == 'weighted_avg' %}
                sum(value * absolute_weight) / nullif(sum(absolute_weight), 0)
                {% else %}
                sum(value * weight)
                {% endif %}
        {% endfor %}
        end                      as harmonized_value,
        bool_or(is_crosswalked)  as is_crosswalked,
        min(source_version)      as source_version,
        max(n_hops)              as n_hops
    from harmonized_{{ tv }}
    group by dataset_slug, target_gisco_id, year
){% if not loop.last %},{% endif %}
{% endfor %}

{% for tv in target_versions %}
select * from aggregated_{{ tv }}
{% if not loop.last %}union all{% endif %}
{% endfor %}
order by target_version, dataset_slug, target_gisco_id, year
