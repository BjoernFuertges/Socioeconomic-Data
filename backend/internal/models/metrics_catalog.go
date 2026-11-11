package models

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FormulaSource describes a data source in a formula (numerator or denominator).
// Either Dataset+Column (RouteSlug of a Soziodata dataset) or Builtin.
type FormulaSource struct {
	Dataset string `yaml:"dataset" json:"dataset,omitempty" example:"bevoelkerung"`
	Column  string `yaml:"column"  json:"column,omitempty"  example:"bevoelkerung"`
	Builtin string `yaml:"builtin" json:"builtin,omitempty" example:"nuts_geometry_area_sqkm"`
}

// FormulaDefinition describes a calculation formula for a derived metric.
// Supported type: "ratio" (numerator / denominator x Scale).
type FormulaDefinition struct {
	Type        string        `yaml:"type"        json:"type"        example:"ratio"`
	Numerator   FormulaSource `yaml:"numerator"   json:"numerator"`
	Denominator FormulaSource `yaml:"denominator" json:"denominator"`
	Scale       float64       `yaml:"scale"       json:"scale"       example:"100"`
}

// DerivedMetricEntry describes a derived metric computed by formula from
// several datasets (e.g. population density, Arbeitslosenquote).
type DerivedMetricEntry struct {
	Slug        string            `yaml:"slug"        json:"slug"        example:"bevoelkerungsdichte"`
	Label       string            `yaml:"label"       json:"label"       example:"Bevölkerungsdichte"`
	Description string            `yaml:"description" json:"description" example:"Einwohner je km²"`
	Unit        string            `yaml:"unit"        json:"unit"        example:"EW/km²"`
	Formula     FormulaDefinition `yaml:"formula"     json:"formula"`
}

// DerivedMetricsCatalog is an ordered list of derived metrics.
type DerivedMetricsCatalog []DerivedMetricEntry

// FindBySlug returns a derived catalog entry by its slug.
func (c DerivedMetricsCatalog) FindBySlug(slug string) (DerivedMetricEntry, bool) {
	for _, entry := range c {
		if entry.Slug == slug {
			return entry, true
		}
	}
	return DerivedMetricEntry{}, false
}

// LoadMetricsCatalog loads the metrics catalog from a YAML file.
// Searches ./configs/ then ../configs/ relative to the working directory.
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
