# Architecture

## Component overview

```mermaid
architecture-beta
    group api(cloud)[API]

    service db(database)[Database] in api
    service disk1(disk)[Storage] in api
    service disk2(disk)[Storage] in api
    service server(server)[Server] in api

    db:L -- R:server
    disk1:T -- B:server
    disk2:T -- B:db
```

## Data flow

From the raw data all the way to the API, every dataset follows the same
path: **dlt** loads raw data unchanged into `loaded_data`, **dbt** builds
staging views, intermediate models, and marts (tables) in `transformed_data`
from it, which are then served by the **backend**.

```mermaid
flowchart LR
    subgraph sources["Source data (data-exchange/1_friedrich-bjoern/)"]
        direction TB
        src_arb[/"arbeitslosenzahlen/\nmapped_pairs.csv\nmeta.json"/]
        src_geo[/"geodata/lau/\nLAU_RG_*.geojson.gz"/]
        src_pop[/"statistics/\nlau_population_*.csv"/]
        src_bev[/"bevoelkerung/results_*/\nmapped_pairs.csv + meta.yaml"/]
        src_sq[/"schuldnerquote/results_*/\nmapped_pairs.csv + meta.yaml"/]
    end

    subgraph dlt["dlt Pipelines"]
        direction TB
        dlt_arb["load_arbeitslosenzahlen_afa.py"]
        dlt_geo["load_geodata_to_postgis.py"]
        dlt_gen["load_soziodata_generic.py"]
    end

    subgraph pg_loaded["PostgreSQL: Schema loaded_data"]
        direction TB
        t_arb[("arbeitslosenzahlen_\nagentur_arbeit")]
        t_meta[("arbeitslosenzahlen_\nmetadata")]
        t_geo[("lau_geometry")]
        t_pop[("lau_population")]
        t_bev[("results_bevoelkerung_\nkreise")]
        t_sq[("results_schuldnerquote_\nkreise")]
    end

    subgraph pg_transformed["PostgreSQL: Schema transformed_data"]
        direction TB
        subgraph stg["Staging (Views)"]
            stg_arb["stg_arbeitslosenzahlen"]
            stg_geo["stg_lau_geometry"]
            stg_pop["stg_lau_population"]
            stg_bev["stg_bevoelkerung_kreise"]
            stg_sq["stg_schuldnerquote_kreise"]
        end
        subgraph marts["Marts (Tables)"]
            mart_arb["mart_arbeitslosenzahlen"]
            mart_lau["lau_integrated"]
            mart_dq["lau_data_quality"]
        end
    end

    api["Go Backend API"]

    src_arb --> dlt_arb
    src_geo & src_pop --> dlt_geo
    src_bev & src_sq --> dlt_gen

    dlt_arb --> t_arb & t_meta
    dlt_geo --> t_geo & t_pop
    dlt_gen --> t_bev & t_sq

    t_arb --> stg_arb
    t_geo --> stg_geo
    t_pop --> stg_pop
    t_bev --> stg_bev
    t_sq --> stg_sq

    stg_arb & t_meta --> mart_arb
    stg_geo & stg_pop --> mart_lau
    mart_lau --> mart_dq

    mart_arb & mart_lau & mart_dq --> api
```

Details on each step:

- **dlt** (Extract & Load): [pipeline/dlt.md](pipeline/dlt.md)
- **dbt** (Transform): [pipeline/dbt.md](pipeline/dbt.md)
- **Backend/API**: [backend/index.md](backend/index.md)

## Other modules

These modules prepare data *before* dlt runs and are only briefly placed in
context here (details in the respective READMEs in the repository):

- **`geo-importer/`** – downloads and cleans official NUTS/LAU geodata
  (GeoJSON, population statistics) as input for `geo-mapper/` and the
  dlt geodata pipeline.
- **`geo-mapper/`** – matches the socioeconomic raw datasets (from
  `data-exchange/`) to geodata IDs (LAU/NUTS) and produces the
  `mapped_pairs.csv`/`meta.yaml` files consumed by
  `dlt/load_soziodata_generic.py`.
- **`data-exchange/`** – exchange directory for intermediate raw-data
  states between project contributors; no code of its own, only data.

```mermaid
flowchart LR
    raw["Raw data from\ndomain teams"] --> gi["geo-importer\n(prepare geodata)"]
    gi --> gm["geo-mapper\n(socioeconomic raw data ↔ geodata ID)"]
    gm --> de["data-exchange/\n(staging area)"]
    de --> dltp["dlt pipelines"]
```
