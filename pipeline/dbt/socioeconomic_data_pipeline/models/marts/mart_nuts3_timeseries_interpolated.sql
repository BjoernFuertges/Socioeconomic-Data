{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts3_ts_interp_id on {{ this }} (nuts3_id, dataset_slug, year)",
            "create index if not exists idx_mart_nuts3_ts_interp_ds on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Lückenlose Zeitreihen mit linearer Interpolation für fehlende Jahre.

  Für jede (nuts3_id, dataset_slug)-Kombination wird die bekannte Jahresspanne
  vollständig aufgefüllt (GENERATE_SERIES von min_year bis max_year).
  Fehlende Werte werden linear zwischen den benachbarten Beobachtungen
  interpoliert.

  Kein Extrapolieren: Werte außerhalb der bekannten Spanne bleiben NULL
  (werden gefiltert) — Randwerte werden nicht fortgeschrieben.

  is_interpolated = false  → Originalwert aus mart_nuts3_sociodata_harmonized
  is_interpolated = true   → Wert linear interpoliert zwischen Nachbarn

  Interpolationsformel:
    value = v_prev + (v_next - v_prev) × (year - y_prev) / (y_next - y_prev)

  Technischer Hinweis zum LOCF-Trick (Last Observation Carried Forward):
    count(value) zählt kumulativ nicht-null Werte in der Fensterrahmung.
    Alle Zeilen bis zum nächsten nicht-null Wert teilen dieselbe Gruppen-ID →
    first_value() innerhalb dieser Gruppe liefert den letzten bekannten Wert.
    Rückwärts angewendet (ORDER BY year DESC) ergibt den nächsten Wert.
*/

with source as (
    select * from {{ ref('mart_nuts3_sociodata_harmonized') }}
),

-- Jahresspanne je Region und Dataset
year_ranges as (
    select
        target_nuts_id,
        dataset_slug,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by target_nuts_id, dataset_slug
),

-- Vollständige Jahresreihe ohne Lücken
all_years as (
    select
        r.target_nuts_id   as nuts3_id,
        r.dataset_slug,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Originaldaten einjoinen (NULL für fehlende Jahre)
with_values as (
    select
        a.nuts3_id,
        a.dataset_slug,
        a.year,
        s.harmonized_value as value
    from all_years a
    left join source s
        on  s.target_nuts_id = a.nuts3_id
        and s.dataset_slug   = a.dataset_slug
        and s.year           = a.year
),

/*
  Schritt 1: Gruppen für letzten bekannten Wert (vorwärts).
  count(value) OVER (ORDER BY year ROWS UNBOUNDED PRECEDING) wächst
  nur bei Nicht-NULL-Werten → bildet Gruppen wo der Wert konstant ist.
*/
with_prev_group as (
    select
        *,
        count(value) over (
            partition by nuts3_id, dataset_slug
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

-- Schritt 2: Gruppen für nächsten bekannten Wert (rückwärts)
with_next_group as (
    select
        *,
        count(value) over (
            partition by nuts3_id, dataset_slug
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

-- Schritt 3: Interpolation anwenden
interpolated as (
    select
        nuts3_id,
        dataset_slug,
        year,
        case
            -- Originalwert vorhanden → unverändert
            when value is not null
                then value
            -- Kein Vorgänger oder kein Nachfolger → kein Extrapolieren
            when prev_value is null or next_value is null
                then null
            -- Lineare Interpolation
            else prev_value
                + (next_value - prev_value)
                * (year       - prev_year )::numeric
                / nullif((next_year - prev_year)::numeric, 0)
        end                                                   as value,
        value is null
            and prev_value is not null
            and next_value is not null                        as is_interpolated
    from with_next
)

-- Zeilen ohne Wert (außerhalb Randbereich) rausfiltern
select *
from interpolated
where value is not null
order by nuts3_id, dataset_slug, year
