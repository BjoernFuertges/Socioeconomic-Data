{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_ts_interp_id on {{ this }} (target_version, gisco_id, dataset_slug, year)",
            "create index if not exists idx_mart_lau_ts_interp_ds on {{ this }} (target_version, dataset_slug, year)"
        ]
    )
}}

/*
  Analog zu mart_nuts3_timeseries_interpolated.sql, aber für die Gemeinde-
  (LAU-)Ebene. Liest von mart_lau_sociodata_harmonized statt roher Staging-
  Quellen, damit Gap-Filling je LAU-Zielversion (target_version) unabhängig
  läuft - siehe dortigen Kommentar zur target_version-Strategie.

  Regionsschlüssel: gisco_id (konsistent mit dem harmonisierten Mart).
  Peer-Gruppe für Methode 3 (Fortschreibung): erste 5 Stellen des Amtlichen
  Gemeindeschlüssels (Kreis-Präfix), ermittelt über
  stg_lau_population.lau_nat_code - LAU-IDs haben (anders als NUTS-Codes)
  keine hierarchische Präfix-Struktur, daher der Umweg über den AGS.

  Siehe mart_nuts3_timeseries_interpolated.sql für die ausführliche
  Dokumentation von Methode 1 (linear) / Methode 3 (peer_trend); die Logik
  ist 1:1 übertragen, nur mit gisco_id/kreis_prefix statt nuts3_id/nuts2_prefix
  und je target_version statt einer einzelnen Zielversion.

  Hinweis: Mit den aktuell lokal geladenen Testdaten bleibt
  mart_lau_sociodata_harmonized leer (siehe dortigen Kommentar) - dieser
  Mart baut zwar, bleibt hier aber ebenfalls leer. Mit korrekt gemappten
  Produktionsdaten liefert er reguläre Zeilen.
*/

with source as (
    select
        dataset_slug,
        target_gisco_id as gisco_id,
        target_version,
        year,
        harmonized_value as value
    from {{ ref('mart_lau_sociodata_harmonized') }}
),

-- Kreisschlüssel je gisco_id (für Peer-Gruppierung) über den amtlichen
-- Gemeindeschlüssel aus der Populationsquelle.
gisco_kreis as (
    select distinct on (gisco_id)
        gisco_id,
        left(regexp_replace(cast(lau_nat_code as text), '\.0+$', ''), 5) as kreis_prefix
    from {{ ref('stg_lau_population') }}
    where gisco_id is not null and lau_nat_code is not null
    order by gisco_id, year desc
),

-- Jahresspanne je Region, Dataset und Zielversion
year_ranges as (
    select
        dataset_slug,
        gisco_id,
        target_version,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by dataset_slug, gisco_id, target_version
),

-- Vollständige Jahresreihe ohne Lücken
all_years as (
    select
        r.gisco_id,
        r.dataset_slug,
        r.target_version,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Originaldaten einjoinen (NULL für fehlende Jahre)
with_values as (
    select
        a.gisco_id,
        a.dataset_slug,
        a.target_version,
        a.year,
        s.value as value
    from all_years a
    left join source s
        on  s.gisco_id       = a.gisco_id
        and s.dataset_slug    = a.dataset_slug
        and s.target_version  = a.target_version
        and s.year            = a.year
),

with_prev_group as (
    select
        *,
        count(value) over (
            partition by gisco_id, dataset_slug, target_version
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by gisco_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by gisco_id, dataset_slug, target_version, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

with_next_group as (
    select
        *,
        count(value) over (
            partition by gisco_id, dataset_slug, target_version
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by gisco_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by gisco_id, dataset_slug, target_version, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

-- Methode 3: Peer-basierte Fortschreibung über den Kreis
with_peer_base as (
    select
        v.gisco_id,
        v.dataset_slug,
        v.target_version,
        v.year,
        v.value,
        k.kreis_prefix
    from with_values v
    left join gisco_kreis k
        on k.gisco_id = v.gisco_id
),

peer_rel_change as (
    select
        cur.kreis_prefix,
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
        on  prev.gisco_id       = cur.gisco_id
        and prev.dataset_slug    = cur.dataset_slug
        and prev.target_version  = cur.target_version
        and prev.year            = cur.year - 1
    where cur.kreis_prefix is not null
),

peer_avg_change as (
    select
        kreis_prefix,
        dataset_slug,
        target_version,
        year,
        avg(rel_change)   as avg_rel_change,
        count(rel_change) as n_peer_regions
    from peer_rel_change
    where rel_change is not null
    group by kreis_prefix, dataset_slug, target_version, year
),

own_prev_actual as (
    select
        gisco_id,
        dataset_slug,
        target_version,
        year + 1 as target_year,
        value    as own_prev_value
    from with_values
    where value is not null
),

interpolated as (
    select
        n.gisco_id,
        n.dataset_slug,
        n.target_version,
        n.year,
        case
            when n.value is not null
                then n.value
            when n.prev_value is null or n.next_value is null
                then null
            else n.prev_value
                + (n.next_value - n.prev_value)
                * (n.year       - n.prev_year )::numeric
                / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value,
        n.value is null
            and n.prev_value is not null
            and n.next_value is not null                      as is_interpolated,
        case
            when n.value is null
                and n.prev_value is not null
                and n.next_value is not null
                then n.prev_value
                    + (n.next_value - n.prev_value)
                    * (n.year       - n.prev_year )::numeric
                    / nullif((n.next_year - n.prev_year)::numeric, 0)
        end                                                   as value_linear,
        case
            when n.value is null
                then opa.own_prev_value * (1 + pac.avg_rel_change)
        end                                                   as value_peer_trend,
        case when n.value is null then pac.n_peer_regions end as n_peer_regions
    from with_next n
    left join gisco_kreis lk
        on lk.gisco_id = n.gisco_id
    left join peer_avg_change pac
        on  pac.kreis_prefix   = lk.kreis_prefix
        and pac.dataset_slug   = n.dataset_slug
        and pac.target_version = n.target_version
        and pac.year           = n.year
    left join own_prev_actual opa
        on  opa.gisco_id       = n.gisco_id
        and opa.dataset_slug   = n.dataset_slug
        and opa.target_version = n.target_version
        and opa.target_year    = n.year
)

select *
from interpolated
where value is not null
order by target_version, gisco_id, dataset_slug, year
