## [1.0.5](https://github.com/BjoernFuertges/Socioeconomic-Data/compare/v1.0.4...v1.0.5) (2026-11-16)


### Bug Fixes

* **frontend:** test & deploy ([d1ea2d4](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d1ea2d4ad36ad32a1fb561a385617960f0294fa9))

## [1.0.4](https://github.com/BjoernFuertges/Socioeconomic-Data/compare/v1.0.3...v1.0.4) (2026-11-16)


### Bug Fixes

* **dbt:** mart_lau_geometry_historized quadratic scan ([9b0e70e](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9b0e70e60a412717f1ddc6cce297ec81dfc3bfe3))

## [1.0.3](https://github.com/BjoernFuertges/Socioeconomic-Data/compare/v1.0.2...v1.0.3) (2026-11-16)


### Bug Fixes

* **frontend:** link to docs, repo & licence ([26793d7](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/26793d705f563c33a0e68fca870a5b6c8ea4f0d3))

## [1.0.2](https://github.com/BjoernFuertges/Socioeconomic-Data/compare/v1.0.1...v1.0.2) (2026-11-15)


### Bug Fixes

* **scripts:** nextcloud sync cache ([f721531](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/f72153168e81ccb28623998af55020629d35d5eb))

## [1.0.1](https://github.com/BjoernFuertges/Socioeconomic-Data/compare/v1.0.0...v1.0.1) (2026-11-15)


### Bug Fixes

* **dlt:** path fix ([8e8ddaa](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/8e8ddaaa0e86ef4e323e3263268c28b3c95a632a))

# 1.0.0 (2026-11-15)


* feat!: serve NUTS crosswalk by majority assignment, add cache/deploy hardening ([ff49b30](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ff49b3068c7fd4c587d64df160df0645fef82a45))


### Bug Fixes

* address codebase review findings across backend, pipeline and frontend ([83faf0a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/83faf0a42648997f5f324a22517b79a0ab3f02ba))
* adminer prefix ([2315728](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2315728460b31c043e1eb6370a1bfa11c44f54c0))
* adminer routing ([6fa2709](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/6fa27095f8cccca7de0c00d5c7ab23924ac4a03e))
* **backend, dbt:** higher min_share for agenturbezirks <-> nuts mapping ([4710462](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/4710462eeda0e758ae28cf4cf129d626328b65ef))
* **backend, deployment:** adminer ipv4 instead of ipv6 ([ea05686](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ea056861ff7c955d83fb93e2027b5d5db5fe763e))
* **backend, deployment:** domain, password ([e54319c](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e54319caebabfa34fbc28754709cc025d65d819f))
* **backend, deployment:** password ([ee55381](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ee553812c2e3d325c4e38e345b208120e18568fd))
* **backend, deployment:** swagger route ([36ccda2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/36ccda27b15f74758feda61dad084168cdd441a0))
* **backend, docker:** add .env.example ([38b5739](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/38b5739329e4bbb84af61ed88f78fd223f59b224))
* **backend, geo:** resolved `null` error ([b2d5377](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/b2d537700401b867ff8102779bf8d50f27677c0f))
* **backend:** harden pagination, GeoJSON input and error handling ([a6beedd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a6beedd81da4c8cda4ec4456f88692f40fb250b2))
* **backend:** load LAU data ([33285ff](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/33285ff1b34349f798fb2cc7276b75483edc434e))
* **backend:** update unemployment model and handler for new mart schema ([342cf61](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/342cf6154884a81ba847a0ffc95973dbd68f1262))
* **ci:** POSIX characters ([7b691bd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/7b691bdc0b49dc3c8b25e949e89f35e30c21e7a1))
* **ci:** validator ([415add6](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/415add6f578d4b19e970485d803b2f28be6b6fbe))
* clean up dbt leftovers, remove ausbildungsquote metric, pin Go image ([3edb6b1](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/3edb6b1d69c6c2dccb554b41575d9a446ddd17fa))
* **compose:** dbt & dlt scheduler config ([dcf37fd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/dcf37fdb61a4857640262e83661f1d5729a5ec91))
* **compose:** healthcheck ([4dc92f9](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/4dc92f9493a10f23193bea072cd95e30ce27ea85))
* **compose:** install ([45354e8](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/45354e82ff798382febc1556e5284d6642ea5435))
* **compose:** ofelia version ([ca2d4a7](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ca2d4a7be950dc65d13a8da59c95048cdc6aa990))
* **data:** typo auzubildende -> auszubildende ([e929b52](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e929b52c8215a28c2b780549afbb20abbb22727a))
* **dbt,dlt:** use generic loader format for arbeitslose data ([8affa22](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/8affa229d245dbb19dec9c478f4648091c760e25))
* **dbt:** arbeitslosenzahlen in inventory ([627c04c](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/627c04c42a4eeaaea90b7a52c1573535758da98d))
* **dbt:** beschäftigungsquote, lau_geometry source ([11ff5f4](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/11ff5f4c6439648611bfee062a4c6c91567ed146))
* **dbt:** bigint type cast ([2540eaf](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2540eaf03066c1b7dd77d1dd9e5f4f973e1e57c2))
* **dbt:** cast population_count to bigint to match Go int64 model ([da57888](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/da578887b2532fcb99be721fc117c47b81e4fbf3))
* **dbt:** convert geometry_wgs84 to GeoJSON and cast population_count to bigint ([146cffd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/146cffd628d7b7e0767a0af2384bb810b4cf2542))
* **dbt:** don't compare empty strings, upgrade dbt ([2303b9f](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2303b9fd2892a372e41a2db4f57ebe1604cd6e83))
* **dbt:** eurostat fill up 0 in NUTS ([f3e1491](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/f3e1491cb235b4d7c1837a567228b32dcd3cdbb9))
* **dbt:** inventory depends on ([e8d4ff0](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e8d4ff040e853141d96f159567265b7590c7fa35))
* **dbt:** type cast ([d1f5b8c](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d1f5b8c80afe74ed014763f60da9d0891c7f13a1))
* **dbt:** type cast (inventory) ([1ee1bf1](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1ee1bf1d61d3e5e0b6c2372f964964801e4cb990))
* **deploy:** scheduling geo mapper, sync nc for Friedrich [#13](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/13) ([8936f77](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/8936f7714e17a8d13c9fca6c7ccfd9e896b35823))
* **dlt:** correct arbeitslosenzahlen path to arbeitslose ([6592c2d](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/6592c2d22d4b7f660c6f1ed502520821e1502ed7))
* **dlt:** handling missing data ([492a1c4](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/492a1c480b9f0976d5cc35915c5ee7060a4e5162))
* **dlt:** replace loaded data ([55b7eea](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/55b7eeae8e9e17580647dc3a88cbb10add9848d5))
* **dockerfile:** location ([2a82673](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2a826739f3ef02028d7ab8f93032bd5e6e041182))
* **docker:** remove swag from Docker build, docs are pre-generated by pre-commit ([920c0e3](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/920c0e333c8689fcb05dd5555d24fe4ca55bef0c))
* dq checks for LAU ([699b4c6](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/699b4c608610aa66f614ba7560a530fbcc1783d6))
* fill_gaps/nuts_version drift, missing lau filter ([dc723ce](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/dc723ce0da41fc8fc6237acc1a57c95da7538222))
* **frontend, regionen:** agentur daten in regular tab below chart ([5767abf](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/5767abff2d83686309979ffe3c0569db29111944))
* **frontend, regionen:** correct history, so you can go back to the last region ([1758f2d](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1758f2d13fb49fd22e7c41d22b86298e132505f8))
* **frontend, regionen:** for agentur chart: only show data who can be aggregated on agentur level (e.g., beschaeftigungsquote-gemeinden can not) ([3f4f1f2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/3f4f1f2d26dda9b3d58a6155ec0b2388ccd29243))
* **geo endpoint:** drop time_type ([2ff3ef2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2ff3ef298b3b34933b32e63af9948005d13dff35))
* **geodata:** path for load data ([046c3cd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/046c3cd5674d60c493f7f7233d9e22fc2a4560e8))
* **geodata:** path for load data ([1801f39](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1801f397625b68ea4b8a94a95b71fc7d9941ac00))
* **geodata:** skip pipeline.run when no files found, log base path ([1e8204a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1e8204a7929b07f317e51e5b9dd409d500862ecc))
* **geodata:** stream GeoJSON with ijson to avoid OOM, fix GEODATA_BASE_PATH ([bd1718f](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/bd1718fb4fb81e17d082ae8dec9f7be5ac7d2367))
* historization referenc ([901f739](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/901f7399b182190c4d1fa18ac9d075493da6dbcc))
* interpolation factor ([aff5d15](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/aff5d1523da0f660ad91ba78d872f44ea4f5b928))
* load data from data exchange, run pipeline more often (every hour) ([118f36a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/118f36a0ecf423f7434957376fd32cc5975f5ad1))
* load Geodaten, Verhältnisse ([6293d84](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/6293d84577d2cdafbaa5dd084210d50d3c7095f1))
* nuts/lau historization (WIP) ([a39cd13](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a39cd136c5735bba54bb1ab9dccf5e9a5812b96f))
* **pipeline:** correct aggregation of rates/means and restore leading zeros in region keys ([a5c78e2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a5c78e242b99a9c238552a7dc5ab14df5bda114f))
* **routes:** rename /aggregate to /aggregated to match swagger docs ([1e23fa7](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1e23fa750523143f1ba0b4bc1cba427ec7bfa75b))
* versions endpoint, added LAU version endpoint ([092fc2d](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/092fc2dc9be3e6091972a0bebd342a3d30087e6e))
* year -> vintage year ([6a9fbf0](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/6a9fbf0d613082510656f31b446c0ee9bb67ad56))


### Features

* add coverage flags to aggregates, flag LAU geometry gaps, resolve point filter by target version ([cee26e1](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/cee26e1f6dbe5eb3be5814205f4f8b66a7446a73))
* **api:** add ARS to NUTS ([c529019](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/c529019d28492a4d1e15500a794c49b9856ed9e5)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** add NUTS-3 layer data ([94c10e6](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/94c10e627a3fbd5402875b0bc5777c6283c7f9d1)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** data inventory endpoint ([89ddc62](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/89ddc62218b45a55d47409f53844dcfdd53aef22)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** get all NUTS regions ([56924f3](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/56924f3a3c40e077e630cfa070637a50a5b7154a)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** get versions of regions ([e0668b8](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e0668b806593c49de3da62d35cc4277151e712e7)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** limit and pagination ([48d2a5f](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/48d2a5f356318007028e82ac5c4d56f4c6b7b0ac)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **api:** split NUTS response into geodata and socioeconomic (with metadata) sections ([13ff36a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/13ff36a9e12c4a8c32099c13768e4957b48e199b)), closes [#11](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/11)
* **backend, agentur:** combine agentur data with other data in one endpoint ([ede918e](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ede918e5d07f3053f59751c490c421fc76c513c9))
* **backend, agentur:** different aggregation methods, use population ([39a19a1](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/39a19a14ec9ca5f054749fb3f6ca82188e658b2b))
* **backend, agentur:** integrate agenturbezirke into NUTS-hierarchy ([1eb5163](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/1eb51631338b0e8ae05194a04edf3f7d55d17dde))
* **backend, api:** custom geo requests: requests based on your own GeoJSON ([acc39fb](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/acc39fb41bbc873f7563efb89bb557856dd92107))
* **backend, api:** download source data, dbt model therefore ([b712140](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/b71214077254be44012d0c27923abe7ac1f6103a))
* **backend, api:** meta data download ([727c1df](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/727c1df7a66ebc439e9b0392af1e34a253e71481))
* **backend, custom region:** identify exact same region as in NUTS/LAU/... use the official region with values ([2a8b60a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2a8b60a0a79262b5388f3398ce8351d39f4a3aef))
* **backend:** add geodata_id_lau ([44f7afd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/44f7afd09b671471baf6c25ad07c1d26dc587023))
* **backend:** AfA-Arbeitslosenzahl auf INKAR mapping ([b6a5fbf](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/b6a5fbfc6731f7f09b7b33a1bf949a6b2989706f))
* **backend:** Agenturbezirke n:m Kreise ([e26cb6d](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e26cb6d21bdebbc5287ae1b7f7266d983c9617c6))
* **backend:** agenturbezirks periods for map ([9796c4d](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9796c4da1dcf768108e7cffdf56c4c1f022f27c4))
* **backend:** aggreagate Kreis to Agenturbezirk data ([9df4513](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9df4513cdcad68aaf00742f9eb7462755a46c6a9))
* **backend:** download api ([ce69cd2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ce69cd25dae57fd2bd70de0e6d2ea29a52ea35c6))
* **backend:** load and deliver arbeitslosenzahlen Arbeitsagentur ([d16e122](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d16e122ee7f591c099d6123433e930d37beee567))
* **backend:** map data between different NUTS levels ([410c2fe](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/410c2fe66c300f1db278ad782019d88c23badc0a))
* **backend:** version tag ([671f7d2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/671f7d2562bdc0f0eed93a1104ac78211e35180d))
* **data:** add new data: arbeitslose, arbeitsvolumen, bruttoverdienst, durchschnittsalter, sgb ii, wohngeld ([9e3f2d0](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9e3f2d0ef1eeba8749c9411eb59bb52b560b972c)), closes [#12](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/12)
* **data:** add new data: beschäftigungsquote (LAU data -> new "pipeline") ([e099fd4](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/e099fd40f0fadcfa077d4dec44eef974df277465)), closes [#12](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/12)
* **dbt, lau:** basicc dbt pipeline ([842d3c0](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/842d3c02b9a3217a54a5c4c2fe38cd652d1edf6c))
* **dbt:** is_estimated flag added ([768ad00](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/768ad006f91e2eb0c478c9c1dbb95b4b32671818))
* **deploy:** deploy on server via ci ([#14](https://github.com/BjoernFuertges/Socioeconomic-Data/issues/14)) ([065e2d6](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/065e2d68822e3c99219ad52a8df35423d47f146d))
* from and to year ([3388dbd](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/3388dbd349647857c07b3dc322d859abc65b512c))
* **frontend, custom map:** select tiles, similiar chart as in Regionen ([d6dd2f2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d6dd2f2800701d12838ecffa9c3ed6694629d48a))
* **frontend, custom region:** requests based on your own GeoJSON ([ecc683c](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ecc683c9ad335712331756d8bcff7f7997d2f09f))
* **frontend, daten:** aggregation note for agentur ([f00f65c](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/f00f65cafb65bdd66ec79adad6c8f3d63e73139d))
* **frontend, download:** download source data, link to data catalog card ([008f903](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/008f903d54f7e28d328cdb430c8d4fb86e0b3935))
* **frontend, map:** select nuts level ([cf1a90b](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/cf1a90bd516d06522643ab4b9d5af0e7dc8687e1))
* **frontend, region list:** tpye badge ([7110921](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/7110921d1831bf0c6542a507915fa715e6538eb6))
* **frontend, regionen:** agentur in NUTS hierarchy ([7bd60aa](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/7bd60aa30871046d0d41ce00e6bae6c1fb19a8b3))
* **frontend, regionen:** combine agentur data with other data in a chart ([077203b](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/077203bdcd92eccae8b30d924856f92b6f459aed))
* **frontend, regionen:** download data set ([f9cba41](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/f9cba419e01f64f6591935cc4f7bbf16e85404a3))
* **frontend, regionen:** meta data download ([691e3f2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/691e3f24cc1bbbd331fe4774624711cec7f1295b))
* **frontend:** Arbeitslosenzahlen von Arbeitsagentur improved ([f7af904](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/f7af904476395a48266093482b439ffeffb3bd0b))
* **frontend:** combined search, hierarchy ([3a934b2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/3a934b288cf4d86f9e66c3fbcc80eed1215e6f29))
* **frontend:** data catalog ([8698de5](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/8698de56719a418ad6633e7bb70e3ddc8af827e9))
* **frontend:** draw our own map ([a3572a5](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a3572a56b54ebab317d8b22b905361ea75c56bb7))
* **frontend:** map with Arbeitslosenzahlen von Arbeitsagentur ([cb47373](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/cb473736dc1e4fa6baed8cb9c5e4c46bf5bcbbed))
* **frontend:** only one region tab, arbeitslosenzahlen von AfA ([b320ca9](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/b320ca9b5b4318f21d2e8f7e6423906a6109bcc2))
* **frontend:** router added, compare different data sets in regions, link map <--> regions <--> data catalog, new effects on map ([ee23930](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ee23930b68c9b163a3d488e4692bc2eab756bbbf))
* **frontend:** show different NUTS level with data ([13b79a0](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/13b79a04274abca25d34f3103d24386428d9f1d1))
* **frontend:** show n:m link between Arbeitsagenturbezirken & Kreisen ([fcb0fc4](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/fcb0fc4ba5a8cfc0aba16ad623e950830f1ae691))
* generic route registration, generation with githook ([ef97752](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/ef97752fe5429eb845577a5c58c3750f2dcfc549))
* **geo endpoint:** add aggregation and geo endpoint ([0eace64](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/0eace649efcd7e93085fddc87dddbb2cdf50a596))
* **geo endpoint:** aggregate nuts-0/1/2 data ([2949091](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/2949091c989eb75da422dfd113b71f04c0a15b24))
* index for optimizing ([d6c71a7](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d6c71a7fe7bccd1b02d772bffb5029b078df5494))
* init go models from dbt ([d0bb301](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/d0bb301fd18589371c59f852b865ba8321479fcb))
* is_estimated flag added ([6cf3faf](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/6cf3faf0b2701f8d4387ff31aee0b6d4984dfb12))
* LAU crosswalk ([401ad54](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/401ad54df4aad800fd7fb88df3acd42bfe9630c3))
* licence ([026bbbe](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/026bbbe8387a81d88d47378abf3e782c29f69a7d))
* meta data incl. licence ([c891429](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/c89142963131ff64d755b13786d8a07963ff51dc))
* metrics (calculate individual, stored) ([5516fcc](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/5516fcc97103b3d764fbd8045acefdcc12633255))
* **metrics:** add derived metric auszubildende-absolut ([35224eb](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/35224eb0b77b4fb400778cf14b053026e1c77d6c))
* **metrics:** read harmonized values and report excluded data ([4c8144b](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/4c8144b39839c5cc9fb50a9c1b6598a70f26766d))
* nextcloud sync ([c16e128](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/c16e128287034be59bd15d04d4bfe729b4649f36))
* nextcloud sync (adjusted default path) ([40bd6ff](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/40bd6ff80aa909faa250fe78dfbb0c9fd3f9f6aa))
* nextcloud sync (adjusted interval) ([4750a8a](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/4750a8ae98ae99ed871cd09b26db767b7adac74b))
* nextcloud sync (share link) ([2425716](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/24257166784c065002e19ac77f2aa0b3ad802c39))
* **nuts-version:** NUTS-3-Zielversion zur Laufzeit wählbar (2016/2021/2024) ([a635b06](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a635b06d699e6ed8ba166c43a7cfe72ac93f48a2))
* optimized lau ([a2b8430](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/a2b84304676b208ebbf347a9a31db90aab5e2d9c))
* **pipeline:** add sv_beschaeftigte dataset from the BA employment series ([c738db2](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/c738db2a873bf1e2e13f83958ed828f11829f3cd))
* query by area ([8d21b49](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/8d21b497d2fa1694af7428d3ee0fe0651cf5d187))
* query with year, geo, get data for region ([fd85036](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/fd850365f6d350e969947527122acb6c1c931526))
* timeseries (lückenlose, interpolierte Zeitreihen) ([9ee66aa](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9ee66aad078534257933ddb6e8119355bd65a736))
* traefik ([9e5328f](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/9e5328f7d8f6f27e0e8666d78c273c7b80311957))
* **website:** migrate website to vue, add map ([068a8c7](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/068a8c7a788fbd29bb1d62dbce0f3c9f5f853081))
* **website:** simple website for visualization of data (first small version) ([289d389](https://github.com/BjoernFuertges/Socioeconomic-Data/commit/289d38939924b8ce2ac7801a4b7531d3119242ec))


### BREAKING CHANGES

* NUTS values for target versions other than the newest
change because the crosswalk now assigns whole Gemeinden (majority) instead
of proportional pieces; responses gain `is_estimated`. Deployments require
IMAGE_TAG to be set and now run Traefik/ofelia behind docker-socket-proxy
containers (`docker compose up --remove-orphans`).
* GET /metrics/derived/ausbildungsquote now returns 404.
