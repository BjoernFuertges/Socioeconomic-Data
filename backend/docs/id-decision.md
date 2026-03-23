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
- Generische Soziodaten-Endpunkte liefern die vorhandenen Referenzfelder (`geodata_id_nuts`, `geodata_id_lau`, `geodata_id_ars`) statt einer neuen technischen ID.

## Begründung

- Vermeidet doppelte Identitätskonzepte und Mapping-Aufwand.
- Erhält Kompatibilität zu bestehenden Datenquellen und Geodaten-Mappings.
- Minimiert Fehlerquellen bei Join- und Filterlogik.
- Hält API und Datenmodell transparent (fachliche IDs statt künstlicher Surrogate).

## Konsequenz

- Bei neuen Datensätzen ist sicherzustellen, dass mindestens ein valider Geodaten-Referenzschlüssel je Zeile vorhanden ist.
- Falls mehrere IDs vorliegen, bleibt `geodata_id_nuts` der bevorzugte Vergleichsschlüssel für überregionale Auswertungen.
