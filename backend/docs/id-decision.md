# ID-Entscheidung für Sozio- und Geodaten

## Status

Beschlossen

## Frage

> Ist eine zusätzliche, eigene (künstliche) "Sozio-Daten-ID" erforderlich?

## Entscheidung

Nein. Es wird **keine** zusätzliche künstliche Sozio-ID eingeführt.

Stattdessen gelten die vorhandenen Referenz-IDs als fachliche Schlüssel je Ebene:

- `geodata_id_nuts`: Primärer Regionsbezug für vergleichende/aggregierte Soziodaten (v. a. NUTS-3)
- `geodata_id_lau`: Regionsbezug auf Gemeindeebene (LAU)
- `geodata_id_ars`: Amtlicher Schlüssel, wenn vorhanden (insbesondere bei NUTS-3/Kreisbezug)

Für API-Responses gilt:

- NUTS-Endpunkte verwenden `nuts_id` als Pfad-/Antwortschlüssel.
- LAU-Endpunkte verwenden `gisco_id` als Pfadschlüssel; in Datenfeldern bleibt der Bezug über `geodata_id_lau` erhalten.
- Agentur-Endpunkte (`/agenturbezirke/*`, `/geo?geo_type=agentur`) verwenden `ba_schl` (amtlicher BA-Schlüssel der Bundesagentur für Arbeit) als Pfad-/Antwortschlüssel. Agenturbezirke sind eine eigenständige Gebietsgliederung ohne NUTS-/LAU-/ARS-Bezug (kein `geodata_id_*`-Äquivalent) - `ba_schl` ist hier, analog zu `nuts_id`/`gisco_id`, der bereits vorhandene fachliche Schlüssel der Quelle, keine neue künstliche ID.
- Generische Soziodaten-Endpunkte liefern die vorhandenen Referenzfelder (`geodata_id_nuts`, `geodata_id_lau`, `geodata_id_ars`) statt einer neuen technischen ID.

## Begründung

- Vermeidet doppelte Identitätskonzepte und Mapping-Aufwand.
- Erhält Kompatibilität zu bestehenden Datenquellen und Geodaten-Mappings.
- Minimiert Fehlerquellen bei Join- und Filterlogik.
- Hält API und Datenmodell transparent (fachliche IDs statt künstlicher Surrogate).

## Konsequenz

- Bei neuen Datensätzen ist sicherzustellen, dass mindestens ein valider Geodaten-Referenzschlüssel je Zeile vorhanden ist.
- Falls mehrere IDs vorliegen, bleibt `geodata_id_nuts` der bevorzugte Vergleichsschlüssel für überregionale Auswertungen.

## Fallstricke

- `gisco_id` ist **nicht** auf Deutschland oder ein `DE_<Ziffern>`-Format
  beschränkt. Die Rohdaten (GISCO LAU-GeoJSONs, siehe
  `dlt/load_geodata_to_postgis.py`) und der Pfad, der tatsächlich
  `transformed_data.lau_integrated` speist
  (`stg_lau_geometry` → `mart_lau_geometry_historized` → `lau_integrated`), filtern nicht auf
  `country_code = 'DE'` - das tun nur einzelne, andere Zwecke dienende
  Zwischenmodelle (`int_lau_geometry_canonical`,
  `int_lau_nuts3_membership`, `int_lau_population_geo`,
  `int_lau_spatial_crosswalk`). In den Rohdaten tauchen `gisco_id`-Werte
  für ~35 Länder auf, und der Teil nach dem `<Ländercode>_`-Präfix ist
  nicht immer rein numerisch (z. B. `CH_CH6194`, `BG_VRC32`,
  `AL_AL141`, `EL_05030403` - nur die deutschen, AGS-abgeleiteten IDs wie
  `DE_01058132` sind zufällig rein numerisch).
- Wer aus einer `gisco_id` per Format-Heuristik auf Land/Typ schließen
  will (z. B. `detectGeoType` in
  [backend/internal/handlers/geo.go](../internal/handlers/geo.go)), darf
  sich daher nicht auf ein `DE_<Ziffern>`-Pattern verlassen - `lau` ist
  dort bewusst der Default-Fall (nach Ausschluss von `ba_schl`- und
  NUTS-Pattern) statt eines eigenen LAU-Pattern-Checks.
