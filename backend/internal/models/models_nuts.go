package models

// NutsRegionData enthält alle sozioökonomischen Indikatoren einer NUTS-Region (Level 0–3) für ein Jahr.
// Bei Level 0–2 entspricht region_name mangels Namenstabelle dem NUTS-Code selbst.
type NutsRegionData struct {
	NutsID                string   `db:"nuts_id"                json:"nuts_id"                example:"DE111"`
	RegionName            string   `db:"region_name"            json:"region_name"            example:"Stuttgart, Stadtkreis"`
	Year                  int      `db:"year"                   json:"year"                   example:"2022"`
	Bevoelkerung          *float64 `db:"bevoelkerung"           json:"bevoelkerung"           example:"634830"`
	SchuldnerquoteProzent *float64 `db:"schuldnerquote_prozent" json:"schuldnerquote_prozent" example:"9.8"`
	// Kann null sein: Arbeitsagenturbezirke entsprechen nicht 1:1 den NUTS-Kreisen
	Arbeitslose      *float64 `db:"arbeitslose"        json:"arbeitslose"        example:"17423"`
	YoYChangePercent *float64 `db:"yoy_change_percent" json:"yoy_change_percent" example:"-2.3"`
}

// NutsRegionTimeSeries enthält die Zeitreihe aller verfügbaren Jahre für eine NUTS-Region (Level 0–3)
type NutsRegionTimeSeries struct {
	NutsID     string           `json:"nuts_id"     example:"DE111"`
	RegionName string           `json:"region_name" example:"Stuttgart, Stadtkreis"`
	Years      []NutsRegionData `json:"years"`
}
