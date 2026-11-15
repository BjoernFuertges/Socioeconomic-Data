{{ config(severity='error') }}

-- Guard for the central target versions in dbt_project.yml (vars
-- reference_nuts_version, reference_lau_version, nuts_versions,
-- lau_versions). Each returned row is one problem.
--
-- Fails when the vars lag behind (or run ahead of) the loaded geometry
-- vintages. The vintages are read from stg_nuts_geometry / stg_lau_geometry
-- (column `year`, the source of mart_nuts_geometry_historized /
-- mart_lau_geometry_historized, which collapse vintages into valid_from /
-- valid_to periods).
--
-- New NUTS/LAU version: load the new geodata (geo-importer -> dlt, incl. the
-- crosswalk inputs), then update the four vars in dbt_project.yml (append to
-- the list and bump the reference version) and rebuild all marts.

with nuts_loaded as (
    select distinct year::int as v from {{ ref('stg_nuts_geometry') }}
),
lau_loaded as (
    select distinct year::int as v from {{ ref('stg_lau_geometry') }}
),
nuts_cfg as (
    select v::int as v
    from unnest(array[{{ var('nuts_versions') | join(', ') }}]::int[]) as v
),
lau_cfg as (
    select v::int as v
    from unnest(array[{{ var('lau_versions') | join(', ') }}]::int[]) as v
)

select 'reference_nuts_version ' || {{ var('reference_nuts_version') }}
       || ' != newest loaded NUTS version ' || max(v) as problem
from nuts_loaded
having max(v) is distinct from {{ var('reference_nuts_version') }}

union all
select 'reference_lau_version ' || {{ var('reference_lau_version') }}
       || ' != newest loaded LAU vintage ' || max(v)
from lau_loaded
having max(v) is distinct from {{ var('reference_lau_version') }}

union all
select 'nuts_versions is missing loaded NUTS version ' || v
from nuts_loaded where v not in (select v from nuts_cfg)

union all
select 'nuts_versions contains NUTS version ' || v || ' that is not loaded'
from nuts_cfg where v not in (select v from nuts_loaded)

union all
select 'lau_versions is missing loaded LAU vintage ' || v
from lau_loaded where v not in (select v from lau_cfg)

union all
select 'lau_versions contains LAU vintage ' || v || ' that is not loaded'
from lau_cfg where v not in (select v from lau_loaded)
