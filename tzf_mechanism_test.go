package tzf_test

import (
	"math/rand"
	"slices"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"

	tzf "github.com/ringsaturn/tzf/v2"
)

// TestLiteMechanismParity checks the three mechanisms over the lite dataset
// answer identically: the in-place .tzb finder, the expanded .tzb finder,
// and the .tzm memory image all compose the same FUZZY fast path over the
// same geometry, so both query methods must agree point for point.
func TestLiteMechanismParity(t *testing.T) {
	finders := map[string]tzf.F{
		"NewEmbeddedFinder": embeddedFinder, // lite .tzb in place
		"NewFinderFromTZB":  tzbFinder,      // lite .tzb expanded
		"NewDefaultFinder":  defaultFinder,  // lite .tzm memory image
	}
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		want := defaultFinder.GetTimezoneName(lng, lat)
		for name, f := range finders {
			if got := f.GetTimezoneName(lng, lat); got != want {
				t.Fatalf("%s single at (%f,%f): got %q want %q", name, lng, lat, got, want)
			}
		}
		if i%100 == 0 {
			want, err := defaultFinder.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			for name, f := range finders {
				got, err := f.GetTimezoneNames(lng, lat)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(got, want) {
					t.Fatalf("%s multi at (%f,%f): got %v want %v", name, lng, lat, got, want)
				}
			}
		}
	}
}

func TestRejectsCorruption(t *testing.T) {
	corrupt := slices.Clone(tzfdist.LiteTZB)
	corrupt[len(corrupt)/2] ^= 1
	if _, err := tzf.NewFinderFromTZB(corrupt); err == nil {
		t.Fatal("NewFinderFromTZB accepted a file with an invalid CRC")
	}
}

func TestProfileMismatch(t *testing.T) {
	if _, err := tzf.NewFinderFromTZM(tzfdist.LiteTZB); err == nil {
		t.Fatal("NewFinderFromTZM accepted an E-profile file")
	}
	if _, err := tzf.NewFinderFromTZB(tzfdist.LiteTZM); err == nil {
		t.Fatal("NewFinderFromTZB accepted an M-profile file")
	}
}
