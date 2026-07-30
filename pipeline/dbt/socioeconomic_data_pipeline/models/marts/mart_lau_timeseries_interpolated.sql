{{
    config(
        materialized='table',
        schema='transformed_data',
        post_hook=[
            "create index if not exists idx_mart_lau_ts_interp_id on {{ this }} (lau_id, dataset_slug, year)",
            "create index if not exists idx_mart_lau_ts_interp_ds on {{ this }} (dataset_slug, year)"
        ]
    )
}}

/*
  Analog zu mart_nuts3_timeseries_interpolated.sql, aber für die Gemeinde-
  (LAU-)Ebene, wo aktuell (nur) beschaeftigungsquote-gemeinden angesiedelt ist.

  Regionsschlüssel: lau_id = coalesce(geodata_id_lau, geodata_id_ars) - LAU-IDs
  haben (anders als NUTS-Codes) keine hierarchische Präfix-Struktur, daher wird
  für Methode 3 (Peer-Fortschreibung) stattdessen der Kreisschlüssel
  (erste 5 Stellen des Amtlichen Gemeindeschlüssels geodata_id_ars) als
  Peer-Gruppe verwendet - Gemeinden desselben Kreises.

  Siehe mart_nuts3_timeseries_interpolated.sql für die ausführliche
  Dokumentation von Methode 1 (linear) / Methode 3 (peer_trend); die Logik
  ist 1:1 übertragen, nur mit lau_id/kreis_prefix statt nuts3_id/nuts2_prefix.

  Hinweis: Mit den aktuell lokal geladenen Testdaten ist geodata_id_lau/ars
  für beschaeftigungsquote-gemeinden durchgehend NULL (das mapped_pairs.csv
  aus dem lokalen Testdatensatz enthält nur geodata_name, keine amtlichen
  IDs) - der Mart baut zwar, bleibt hier aber leer. Mit korrekt gemappten
  Produktionsdaten (geodata_id_lau/ars gesetzt) liefert er reguläre Zeilen.
*/

{% set lau_source_datasets = [
    {'slug': 'beschaeftigungsquote-gemeinden', 'model': 'stg_beschaeftigungsquote_gemeinden', 'value_col': 'beschaeftigungsquote_prozent'},
] %}

with source as (
    {% for ds in lau_source_datasets %}
    select
        '{{ ds.slug }}'                                   as dataset_slug,
        coalesce(geodata_id_lau, geodata_id_ars)           as lau_id,
        geodata_id_ars,
        year::integer                                     as year,
        {{ ds.value_col }}::numeric                        as value
    from {{ ref(ds.model) }}
    where coalesce(geodata_id_lau, geodata_id_ars) is not null
      and {{ ds.value_col }} is not null
    {% if not loop.last %}union all{% endif %}
    {% endfor %}
),

-- Kreisschlüssel je lau_id ermitteln (für Peer-Gruppierung), erste
-- gefundene, nicht-null Ausprägung je lau_id/dataset_slug reicht - der
-- Kreis einer Gemeinde ändert sich nicht innerhalb der Zeitreihe.
lau_kreis as (
    select distinct
        dataset_slug,
        lau_id,
        left(geodata_id_ars, 5) as kreis_prefix
    from source
    where geodata_id_ars is not null
),

-- Jahresspanne je Region und Dataset
year_ranges as (
    select
        dataset_slug,
        lau_id,
        min(year) as min_year,
        max(year) as max_year
    from source
    group by dataset_slug, lau_id
),

-- Vollständige Jahresreihe ohne Lücken
all_years as (
    select
        r.lau_id,
        r.dataset_slug,
        g.year
    from year_ranges r
    cross join lateral generate_series(r.min_year, r.max_year) as g(year)
),

-- Originaldaten einjoinen (NULL für fehlende Jahre)
with_values as (
    select
        a.lau_id,
        a.dataset_slug,
        a.year,
        s.value as value
    from all_years a
    left join source s
        on  s.lau_id       = a.lau_id
        and s.dataset_slug = a.dataset_slug
        and s.year         = a.year
),

with_prev_group as (
    select
        *,
        count(value) over (
            partition by lau_id, dataset_slug
            order by year
            rows between unbounded preceding and current row
        ) as prev_grp
    from with_values
),

with_prev as (
    select
        *,
        first_value(value) over (
            partition by lau_id, dataset_slug, prev_grp
            order by year
        ) as prev_value,
        first_value(year) over (
            partition by lau_id, dataset_slug, prev_grp
            order by year
        ) as prev_year
    from with_prev_group
),

with_next_group as (
    select
        *,
        count(value) over (
            partition by lau_id, dataset_slug
            order by year desc
            rows between unbounded preceding and current row
        ) as next_grp
    from with_prev
),

with_next as (
    select
        *,
        first_value(value) over (
            partition by lau_id, dataset_slug, next_grp
            order by year desc
        ) as next_value,
        first_value(year) over (
            partition by lau_id, dataset_slug, next_grp
            order by year desc
        ) as next_year
    from with_next_group
),

-- Methode 3: Peer-basierte Fortschreibung über den Kreis
with_peer_base as (
    select
        v.lau_id,
        v.dataset_slug,
        v.year,
        v.value,
        k.kreis_prefix
    from with_values v
    left join lau_kreis k
        on  k.lau_id       = v.lau_id
        and k.dataset_slug = v.dataset_slug
),

peer_rel_change as (
    select
        cur.kreis_prefix,
        cur.dataset_slug,
        cur.year,
        case
            when prev.value is not null
                and prev.value != 0
                and cur.value is not null
                then (cur.value - prev.value) / prev.value
        end as rel_change
    from with_peer_base cur
    join with_peer_base prev
        on  prev.lau_id       = cur.lau_id
        and prev.dataset_slug = cur.dataset_slug
        and prev.year         = cur.year - 1
    where cur.kreis_prefix is not null
),

peer_avg_change as (
    select
        kreis_prefix,
        dataset_slug,
        year,
        avg(rel_change)   as avg_rel_change,
        count(rel_change) as n_peer_regions
    from peer_rel_change
    where rel_change is not null
    group by kreis_prefix, dataset_slug, year
),

own_prev_actual as (
    select
        lau_id,
        dataset_slug,
        year + 1 as target_year,
        value    as own_prev_value
    from with_values
    where value is not null
),

interpolated as (
    select
        n.lau_id,
        n.dataset_slug,
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
    left join lau_kreis lk
        on  lk.lau_id       = n.lau_id
        and lk.dataset_slug = n.dataset_slug
    left join peer_avg_change pac
        on  pac.kreis_prefix = lk.kreis_prefix
        and pac.dataset_slug = n.dataset_slug
        and pac.year         = n.year
    left join own_prev_actual opa
        on  opa.lau_id       = n.lau_id
        and opa.dataset_slug = n.dataset_slug
        and opa.target_year  = n.year
)

select *
from interpolated
where value is not null
order by lau_id, dataset_slug, year
