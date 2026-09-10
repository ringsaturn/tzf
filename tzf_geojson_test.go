package tzf

import (
	"bytes"
	"errors"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"
)

// TestGeoJSONerCoverage pins the contract documented on GeoJSONer: every
// finder this package constructs can export its geometry, so callers may
// assert the behavior instead of a concrete type.
func TestGeoJSONerCoverage(t *testing.T) {
	build := map[string]func() (F, error){
		"NewDefaultFinder":  NewDefaultFinder,
		"NewEmbeddedFinder": NewEmbeddedFinder,
		"NewFullFinder":     NewFullFinder,
		"NewFinderFromTZB":  func() (F, error) { return NewFinderFromTZB(tzfdist.LiteTZB) },
		"NewFinderFromTZM":  func() (F, error) { return NewFinderFromTZM(tzfdist.LiteTZM) },
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
	inPlace, err := NewEmbeddedFinder()
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := NewFinderFromTZB(tzfdist.LiteTZB)
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
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: in-place GeoJSON differs from expanded", name)
		}
	}

	gotAll := inPlace.(GeoJSONer).GetGeoJSON()
	wantAll := expanded.(GeoJSONer).GetGeoJSON()
	if !bytes.Equal(gotAll, wantAll) {
		t.Fatalf("whole-world GeoJSON differs: in place %d bytes, expanded %d bytes",
			len(gotAll), len(wantAll))
	}
}
