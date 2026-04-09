package models

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FormulaSource beschreibt eine Datenquelle in einer Formel (Zähler oder Nenner).
// Entweder Dataset+Column (RouteSlug eines Soziodata-Datensatzes) oder Builtin.
type FormulaSource struct {
	Dataset string `yaml:"dataset" json:"dataset,omitempty" example:"bevoelkerung"`
	Column  string `yaml:"column"  json:"column,omitempty"  example:"bevoelkerung"`
	Builtin string `yaml:"builtin" json:"builtin,omitempty" example:"nuts_geometry_area_sqkm"`
}

// FormulaDefinition beschreibt eine Berechnungsformel für eine abgeleitete Metrik.
// Unterstützter Typ: "ratio" (Zähler / Nenner × Scale).
type FormulaDefinition struct {
	Type        string        `yaml:"type"        json:"type"        example:"ratio"`
	Numerator   FormulaSource `yaml:"numerator"   json:"numerator"`
	Denominator FormulaSource `yaml:"denominator" json:"denominator"`
	Scale       float64       `yaml:"scale"       json:"scale"       example:"100"`
}

// DerivedMetricEntry beschreibt eine abgeleitete Metrik, die per Formel aus
// mehreren Datensätzen berechnet wird (z.B. Bevölkerungsdichte, Arbeitslosenquote).
type DerivedMetricEntry struct {
	Slug        string            `yaml:"slug"        json:"slug"        example:"bevoelkerungsdichte"`
	Label       string            `yaml:"label"       json:"label"       example:"Bevölkerungsdichte"`
	Description string            `yaml:"description" json:"description" example:"Einwohner je km²"`
	Unit        string            `yaml:"unit"        json:"unit"        example:"EW/km²"`
	Formula     FormulaDefinition `yaml:"formula"     json:"formula"`
}

// DerivedMetricsCatalog ist eine geordnete Liste abgeleiteter Metriken.
type DerivedMetricsCatalog []DerivedMetricEntry

// FindBySlug gibt einen abgeleiteten Katalogeintrag anhand seines Slugs zurück.
func (c DerivedMetricsCatalog) FindBySlug(slug string) (DerivedMetricEntry, bool) {
	for _, entry := range c {
		if entry.Slug == slug {
			return entry, true
		}
	}
	return DerivedMetricEntry{}, false
}

// LoadMetricsCatalog lädt den Metriken-Katalog aus einer YAML-Datei.
// Sucht nacheinander in ./configs/ und ../configs/ relativ zum Arbeitsverzeichnis.
func LoadMetricsCatalog(filename string) (DerivedMetricsCatalog, error) {
	candidates := []string{
		filepath.Join("configs", filename),
		filepath.Join("..", "configs", filename),
	}

	var data []byte
	var readErr error
	for _, path := range candidates {
		data, readErr = os.ReadFile(path)
		if readErr == nil {
			break
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("Metriken-Katalog %q nicht gefunden: %w", filename, readErr)
	}

	var raw struct {
		DerivedMetrics DerivedMetricsCatalog `yaml:"derived_metrics"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Fehler beim Parsen des Metriken-Katalogs: %w", err)
	}
	return raw.DerivedMetrics, nil
}
