package tzf

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// TestGeoJSONerCoverage pins the contract documented on GeoJSONer: every
// finder this package constructs can export its geometry, so callers may
// assert the behavior instead of a concrete type.
func TestGeoJSONerCoverage(t *testing.T) {
	tzb, _ := loadTZBTestData(t)
	tzbFuzzy, _ := loadTZBWithFuzzy(t)
	_, tzmFuzzy := loadTZMTestData(t)

	build := map[string]func() (F, error){
		"NewFinderFromTZB":         func() (F, error) { return NewFinderFromTZB(tzb) },
		"NewFinderFromTZBExpanded": func() (F, error) { return NewFinderFromTZBExpanded(tzb) },
		"NewFuzzyFinderFromTZB":    func() (F, error) { return NewFuzzyFinderFromTZB(tzbFuzzy) },
		"NewDefaultFinderFromTZB":  func() (F, error) { return NewDefaultFinderFromTZB(tzbFuzzy) },
		"NewFinderFromTZM":         func() (F, error) { return NewFinderFromTZM(tzmFuzzy) },
		"NewDefaultFinderFromTZM":  func() (F, error) { return NewDefaultFinderFromTZM(tzmFuzzy) },
	}
	for name, newFinder := range build {
		t.Run(name, func(t *testing.T) {
			finder, err := newFinder()
			if err != nil {
				t.Fatal(err)
			}
			exporter, ok := finder.(GeoJSONer)
			if !ok {
				t.Fatalf("%s result does not satisfy GeoJSONer", name)
			}
			if _, err := exporter.GetTZGeoJSON("Asia/Tokyo"); err != nil {
				t.Fatalf("GetTZGeoJSON: %v", err)
			}
			if _, err := exporter.GetTZGeoJSON("Not/AZone"); !errors.Is(err, ErrNoTimezoneFound) {
				t.Fatalf("unknown name error = %v, want ErrNoTimezoneFound", err)
			}
		})
	}
}

// TestInPlaceGeoJSONMatchesExpanded checks that decoding geometry on demand
// from the file produces exactly what the expanded finder holds in memory:
// both go through the same embedbin ring expansion, so the exported
// FeatureCollections must be byte-identical, whole-world included.
func TestInPlaceGeoJSONMatchesExpanded(t *testing.T) {
	tzb, _ := loadTZBTestData(t)
	inPlace, err := NewFinderFromTZB(tzb)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := NewFinderFromTZBExpanded(tzb)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Asia/Tokyo", "America/New_York", "Europe/Berlin", "Etc/GMT-12"} {
		got, err := inPlace.(GeoJSONer).GetTZGeoJSON(name)
		if err != nil {
			t.Fatalf("%s in place: %v", name, err)
		}
		want, err := expanded.(GeoJSONer).GetTZGeoJSON(name)
		if err != nil {
			t.Fatalf("%s expanded: %v", name, err)
		}
		if !bytes.Equal(mustJSON(t, got), mustJSON(t, want)) {
			t.Fatalf("%s: in-place GeoJSON differs from expanded", name)
		}
	}

	gotAll := mustJSON(t, inPlace.(GeoJSONer).GetGeoJSON())
	wantAll := mustJSON(t, expanded.(GeoJSONer).GetGeoJSON())
	if !bytes.Equal(gotAll, wantAll) {
		t.Fatalf("whole-world GeoJSON differs: in place %d bytes, expanded %d bytes",
			len(gotAll), len(wantAll))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
