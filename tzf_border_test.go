// Regression tests for https://github.com/ringsaturn/tzf-rs/issues/207:
// a query landing exactly on a shared polygon border used to match neither
// neighbour and return an empty result.

package tzf_test

import (
	"slices"
	"testing"

	"github.com/ringsaturn/tzf"
)

func TestNauticalBorderIsNotAGap(t *testing.T) {
	f, err := tzf.NewDefaultFinder()
	if err != nil {
		t.Fatal(err)
	}

	// The nautical zones are 15-degree-wide strips, so their borders sit on
	// whole meridians. The two coordinates from the issue report:
	for _, tc := range []struct {
		lng, lat float64
		want     []string
	}{
		{7.5, 54.5, []string{"Etc/GMT", "Etc/GMT-1"}},
		{-22.5, 54.5, []string{"Etc/GMT+1", "Etc/GMT+2"}},
	} {
		if got := f.GetTimezoneName(tc.lng, tc.lat); got == "" {
			t.Errorf("GetTimezoneName(%v, %v) is empty", tc.lng, tc.lat)
		}
		// Both sides of the border claim the point.
		got, err := f.GetTimezoneNames(tc.lng, tc.lat)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("GetTimezoneNames(%v, %v) = %v, want %v", tc.lng, tc.lat, got, tc.want)
		}
	}
}

func TestBorderNeighboursStillResolveToOneSideEach(t *testing.T) {
	f, err := tzf.NewDefaultFinder()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		lng, lat float64
		want     string
	}{
		{7.4999, 54.5, "Etc/GMT"},
		{7.5001, 54.5, "Etc/GMT-1"},
		{-22.4999, 54.5, "Etc/GMT+1"},
		{-22.5001, 54.5, "Etc/GMT+2"},
	} {
		if got := f.GetTimezoneName(tc.lng, tc.lat); got != tc.want {
			t.Errorf("GetTimezoneName(%v, %v) = %q, want %q", tc.lng, tc.lat, got, tc.want)
		}
	}
}

// TestGlobalGridHasNoHoles sweeps the whole globe at 1 degree. Every point used
// to miss on the 24 nautical meridians (2753 empty results); nothing may miss
// now.
func TestGlobalGridHasNoHoles(t *testing.T) {
	f, err := tzf.NewDefaultFinder()
	if err != nil {
		t.Fatal(err)
	}

	var empty [][2]float64
	for lng := -179.5; lng <= 179.5; lng += 1.0 {
		for lat := -89.5; lat <= 89.5; lat += 1.0 {
			if f.GetTimezoneName(lng, lat) == "" {
				empty = append(empty, [2]float64{lng, lat})
			}
		}
	}

	if len(empty) != 0 {
		t.Errorf("%d empty results, first few: %v", len(empty), empty[:min(len(empty), 10)])
	}
}
