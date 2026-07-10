{{ config(materialized='view') }}

/*
  Bevölkerung je NUTS-3-Region und Jahr.

  Aggregiert aus den Kreisdaten (results_bevoelkerung_kreise), da diese bereits
  auf NUTS-3 gemappt sind und einen direkten Bevölkerungswert enthalten.

  Wird als Gewichtsbasis für den bevölkerungsgewichteten Versions-Crosswalk
  verwendet: Je größer eine Region, desto stärker geht ihr Wert in die
  anteilige Zuweisung ein wenn Grenzen sich verschoben haben.
*/

select
    geodata_id_nuts          as nuts3_id,
    year::integer            as year,
    sum(value::numeric)      as total_population

from {{ source('loaded_data', 'results_bevoelkerung_kreise') }}

where geodata_id_nuts is not null
  and geodata_id_nuts != ''
  and value is not null

group by geodata_id_nuts, year::integer
