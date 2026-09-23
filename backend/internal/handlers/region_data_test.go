package handlers

import (
	"reflect"
	"testing"
)

func TestParseRegionScope(t *testing.T) {
	tests := []struct {
		in      string
		want    regionScope
		wantErr bool
	}{
		{"", regionScope{true, true}, false},
		{"both", regionScope{true, true}, false},
		{"raw", regionScope{Raw: true}, false},
		{"aggregated", regionScope{Aggregated: true}, false},
		{"raw, aggregated", regionScope{true, true}, false},
		{"RAW", regionScope{Raw: true}, false},
		{"foo", regionScope{}, true},
	}
	for _, tt := range tests {
		got, err := parseRegionScope(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseRegionScope(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Errorf("parseRegionScope(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseDatasetList(t *testing.T) {
	got := parseDatasetList(" Bevoelkerung, arbeitslose,,bevoelkerung ")
	want := []string{"bevoelkerung", "arbeitslose"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDatasetList = %v, want %v", got, want)
	}
	if got := parseDatasetList(""); len(got) != 0 {
		t.Errorf("parseDatasetList(\"\") = %v, want empty", got)
	}
}

func TestValidateRegionDatasets(t *testing.T) {
	if err := validateRegionDatasets([]string{"bevoelkerung", slugBAArbeitslose, slugLauPopulationSum}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := validateRegionDatasets([]string{"gibt-es-nicht"}); err == nil {
		t.Error("expected error for unknown slug")
	}
}

func TestAddYearRange(t *testing.T) {
	from, to := 2010, 2020
	q, args := addYearRange("SELECT 1 WHERE a = $1", []interface{}{"x"}, "year", regionDataFilter{FromYear: &from, ToYear: &to})
	if want := "SELECT 1 WHERE a = $1 AND year >= $2 AND year <= $3"; q != want {
		t.Errorf("query = %q, want %q", q, want)
	}
	if !reflect.DeepEqual(args, []interface{}{"x", 2010, 2020}) {
		t.Errorf("args = %v", args)
	}
}
