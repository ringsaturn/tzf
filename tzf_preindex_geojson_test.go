package tzf

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"
)

// TestPreindexGeoJSONCoverage pins that every constructed finder exports its
// FUZZY preindex tiles through [GeoJSONer].
func TestPreindexGeoJSONCoverage(t *testing.T) {
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
			raw, err := exporter.GetTZPreindexGeoJSON("Asia/Shanghai")
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) == 0 {
				t.Fatal("empty preindex export")
			}
			if _, err := exporter.GetTZPreindexGeoJSON("Invalid/Timezone"); !errors.Is(err, ErrNoTimezoneFound) {
				t.Fatalf("unknown name: want ErrNoTimezoneFound, got %v", err)
			}
		})
	}
}

// TestPreindexGeoJSONShape checks the exported document: one Feature whose
// MultiPolygon holds closed 5-point tile rectangles, one of which covers a
// point the preindex fast path answers.
func TestPreindexGeoJSONShape(t *testing.T) {
	finder, err := NewDefaultFinder()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := finder.(GeoJSONer).GetTZPreindexGeoJSON("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Type     string `json:"type"`
		Features []struct {
			Type       string `json:"type"`
			Properties struct {
				Tzid string `json:"tzid"`
			} `json:"properties"`
			Geometry struct {
				Type        string           `json:"type"`
				Coordinates [][][][2]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Type != "FeatureCollection" || len(doc.Features) != 1 {
		t.Fatalf("want FeatureCollection with one Feature, got %s with %d", doc.Type, len(doc.Features))
	}
	feature := doc.Features[0]
	if feature.Properties.Tzid != "Asia/Shanghai" || feature.Geometry.Type != "MultiPolygon" {
		t.Fatalf("unexpected feature: %+v", feature)
	}
	if len(feature.Geometry.Coordinates) == 0 {
		t.Fatal("no tiles exported")
	}
	covered := false
	lng, lat := 116.3883, 39.9289
	for _, polygon := range feature.Geometry.Coordinates {
		if len(polygon) != 1 {
			t.Fatalf("tile polygons carry no holes, got %d rings", len(polygon))
		}
		ring := polygon[0]
		if len(ring) != 5 || ring[0] != ring[4] {
			t.Fatalf("want closed 5-point rectangle ring, got %v", ring)
		}
		if ring[0][0] <= lng && lng <= ring[2][0] && ring[0][1] <= lat && lat <= ring[2][1] {
			covered = true
		}
	}
	if !covered {
		t.Fatal("no preindex tile covers the Beijing sample point")
	}
}

// TestInPlacePreindexGeoJSONMatchesExpanded pins byte-identical preindex
// exports between the expanded composition and the in-place finder over the
// same file: both order tiles by ascending packed key.
func TestInPlacePreindexGeoJSONMatchesExpanded(t *testing.T) {
	expanded, err := NewFinderFromTZB(tzfdist.LiteTZB)
	if err != nil {
		t.Fatal(err)
	}
	inplace, err := NewEmbeddedFinder()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Asia/Shanghai", "Europe/Berlin", "America/Chicago"} {
		a, err := expanded.(GeoJSONer).GetTZPreindexGeoJSON(name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := inplace.(GeoJSONer).GetTZPreindexGeoJSON(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("preindex export mismatch for %s", name)
		}
	}
	a, err := expanded.(GeoJSONer).GetPreindexGeoJSON()
	if err != nil {
		t.Fatal(err)
	}
	b, err := inplace.(GeoJSONer).GetPreindexGeoJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("whole-preindex export mismatch")
	}
}
