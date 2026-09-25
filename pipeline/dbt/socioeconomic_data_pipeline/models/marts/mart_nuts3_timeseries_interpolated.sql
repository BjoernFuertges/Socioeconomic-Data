{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_nuts3_ts_interp_id on {{ this }} (target_version, nuts3_id, dataset_slug, year)",
            "create index if not exists idx_mart_nuts3_ts_interp_ds on {{ this }} (target_version, dataset_slug, year)"
        ]
    )
}}

/*
  Lückenlose Zeitreihen mit zwei alternativen Imputationsmethoden für
  fehlende Jahre je (nuts3_id, dataset_slug, target_version):

    Methode 1 (linear)      value_linear      - lineare Interpolation
      zwischen dem letzten bekannten Vor- und dem nächsten bekannten
      Nachjahr (ggf. über mehrere Lückenjahre hinweg).
    Methode 3 (peer_trend)  value_peer_trend  - Fortschreibung des
      zuletzt bekannten Eigenwerts (Jahr - 1) mit der durchschnittlichen
      relativen Veränderung anderer Regionen derselben NUTS-2-Gruppe
      (erste 4 Zeichen der NUTS-3-ID) von (Jahr - 1) auf Jahr. Im
      Unterschied zu Methode 1 wird dabei NICHT über mehrere Jahre
      hinweg fortgeschrieben - ist bereits der Vorjahreswert selbst
      unbekannt, bleibt value_peer_trend NULL.

  target_version: mart_nuts3_sociodata_harmonized führt dieselben Werte
  bereits auf mehreren NUTS-3-Zielversionen parallel (siehe dort). Gap-
  Filling und Peer-Trend laufen hier je target_version unabhängig, da eine
  Region in unterschiedlichen Versionen unterschiedliche Nachbarregionen
  und Wertelücken haben kann.

  `value` bleibt aus Gründen der Abwärtskompatibilität der "amtliche"
  Ausgabewert (Originalwert, sonst Methode-1-Schätzung). `value_linear`
  und `value_peer_trend` werden nur für tatsächlich interpolierte Zeilen
  befüllt (is_interpolated = true), damit Konsumenten (z.B. die API)
  beide Schätzungen transparent nebeneinander ausgeben können, statt sie
  stillschweigend zu vermischen.

  Für jede (nuts3_id, dataset_slug, target_version)-Kombination wird die
  bekannte Jahresspanne vollständig aufgefüllt (GENERATE_SERIES von
  min_year bis max_year).

  Kein Extrapolieren: Werte außerhalb der bekannten Spanne bleiben NULL
  (werden gefiltert) — Randwerte werden nicht fortgeschrieben.

  is_interpolated = false  → Originalwert aus mart_nuts3_sociodata_harmonized
  is_interpolated = true   → Wert linear interpoliert zwischen Nachbarn

  Interpolationsformel (Methode 1):
    value = v_prev + (v_next - v_prev) × (year - y_prev) / (y_next - y_prev)

  Fortschreibungsformel (Methode 3):
    value_peer_trend = v_own_prev × (1 + ⌀ rel_change_peers)
    rel_change_peers  = (v_peer_year - v_peer_prevyear) / v_peer_prevyear
    (nur Peers mit bekanntem, von 0 verschiedenem Vorjahreswert)

  Laut Evaluation (analysis/data-analysis/impute_missing_values.py, Leave-one-out über
  520 Stichproben aus allen 13 Kreis-Datensätzen, siehe
  docs/pipeline/dbt.md#interpolationsmethoden-vergleich): Methode 1 liefert
  im Mittel die genaueren Schätzungen (MAPE 2.45 %, Trefferquote 51 %),
  Methode 3 ist bei einigen Datensätzen (u.a. schuldnerquote,
  wohngeldhaushalte, medianeinkommen) knapp überlegen (Trefferquote 42 %).

  Technischer Hinweis zum LOCF-Trick (Last Observation Carried Forward):
    count(value) zählt kumulativ nicht-null Werte in der Fensterrahmung.
    Alle Zeilen bis zum nächsten nicht-null Wert teilen dieselbe Gruppen-ID →
    first_value() innerhalb dieser Gruppe liefert den letzten bekannten Wert.
    Rückwärts angewendet (ORDER BY year DESC) ergibt den nächsten Wert.
*/

with source as (
    select * from {{ ref('mart_nuts3_sociodata_harmonized') }}
),

-- Jahresspanne je Region, Dataset und Zielversion
year_ranges as (
    select
        target_nuts_id,
        dataset_slug,
        target_version,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by target_nuts_id, dataset_slug, target_version
),

-- Vollständige Jahresreihe ohne Lücken
all_years as (
    select
        r.target_nuts_id   as nuts3_id,
        r.dataset_slug,
        r.target_version,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Originaldaten einjoinen (NULL für fehlende Jahre)
with_values as (
    select
        a.nuts3_id,
        a.dataset_slug,
        a.target_version,
        a.year,
        s.harmonized_value as value
    from all_years a
    left join source s
        on  s.target_nuts_id  = a.nuts3_id
        and s.dataset_slug    = a.dataset_slug
        and s.target_version  = a.target_version
        and s.year            = a.year
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
            partition by nuts3_id, dataset_slug, target_version
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

-- Schritt 2: Gruppen für nächsten bekannten Wert (rückwärts)
with_next_group as (
    select
        *,
        count(value) over (
            partition by nuts3_id, dataset_slug, target_version
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by nuts3_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by nuts3_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

/*
  Methode 3 - Peer-basierte Fortschreibung.

  Schritt A: NUTS-2-Gruppe je Region (erste 4 Zeichen der NUTS-3-ID).
  Schritt B: Relative Veränderung jeder Region von (Jahr - 1) auf Jahr,
             basierend ausschließlich auf Originalwerten (with_values),
             nicht auf bereits interpolierten/fortgeschriebenen Werten.
  Schritt C: Durchschnittliche relative Veränderung je (NUTS-2-Gruppe,
             Dataset, Zielversion, Jahr) über alle Regionen mit bekanntem
             Vor- und Jahreswert (Vorjahreswert 0 wird ausgeschlossen,
             Division durch 0). Die Zielregion selbst trägt hierzu nichts
             bei, da ihr eigener Jahreswert per Definition unbekannt ist.
  Schritt D: Eigener Vorjahreswert (unmittelbares Jahr - 1, aus
             with_values, kein LOCF über mehrere Jahre) je Zielregion.
*/
with_peer_base as (
    select
        nuts3_id,
        dataset_slug,
        target_version,
        year,
        value,
        substr(nuts3_id, 1, 4) as nuts2_prefix
    from with_values
),

peer_rel_change as (
    select
        cur.nuts2_prefix,
        cur.dataset_slug,
        cur.target_version,
        cur.year,
        case
            when prev.value is not null
                and prev.value != 0
                and cur.value is not null
                then (cur.value - prev.value) / prev.value
        end as rel_change
    from with_peer_base cur
    join with_peer_base prev
        on  prev.nuts3_id       = cur.nuts3_id
        and prev.dataset_slug   = cur.dataset_slug
        and prev.target_version = cur.target_version
        and prev.year           = cur.year - 1
),

peer_avg_change as (
    select
        nuts2_prefix,
        dataset_slug,
        target_version,
        year,
        avg(rel_change)   as avg_rel_change,
        count(rel_change) as n_peer_regions
    from peer_rel_change
    where rel_change is not null
    group by nuts2_prefix, dataset_slug, target_version, year
),

own_prev_actual as (
    select
        nuts3_id,
        dataset_slug,
        target_version,
        year + 1 as target_year,
        value    as own_prev_value
    from with_values
    where value is not null
),

-- Schritt 3: Interpolation anwenden
interpolated as (
    select
        n.nuts3_id,
        n.dataset_slug,
        n.target_version,
        n.year,
        case
            -- Originalwert vorhanden → unverändert
            when n.value is not null
                then n.value
            -- Kein Vorgänger oder kein Nachfolger → kein Extrapolieren
            when n.prev_value is null or n.next_value is null
                then null
            -- Lineare Interpolation (Methode 1)
            else n.prev_value
                + (n.next_value - n.prev_value)
                * (n.year       - n.prev_year )::numeric
                / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value,
        n.value is null
            and n.prev_value is not null
            and n.next_value is not null                      as is_interpolated,
        -- Methode 1: nur für tatsächlich interpolierte Zeilen befüllt,
        -- ansonsten identisch mit dem obigen "value" (kein Duplikat nötig).
        case
            when n.value is null
                and n.prev_value is not null
                and n.next_value is not null
                then n.prev_value
                    + (n.next_value - n.prev_value)
                    * (n.year       - n.prev_year )::numeric
                    / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value_linear,
        -- Methode 3: nur befüllt, wenn sowohl der eigene Vorjahreswert als
        -- auch mindestens ein Peer-Wert für dieses (Jahr, Dataset,
        -- Zielversion) bekannt ist.
        case
            when n.value is null
                then opa.own_prev_value * (1 + pac.avg_rel_change)
        end                                                   as value_peer_trend,
        case when n.value is null then pac.n_peer_regions end as n_peer_regions
    from with_next n
    left join peer_avg_change pac
        on  pac.nuts2_prefix    = substr(n.nuts3_id, 1, 4)
        and pac.dataset_slug    = n.dataset_slug
        and pac.target_version  = n.target_version
        and pac.year            = n.year
    left join own_prev_actual opa
        on  opa.nuts3_id        = n.nuts3_id
        and opa.dataset_slug    = n.dataset_slug
        and opa.target_version  = n.target_version
        and opa.target_year     = n.year
)

-- Zeilen ohne Wert (außerhalb Randbereich) rausfiltern
select *
from interpolated
where value is not null
order by target_version, nuts3_id, dataset_slug, year
