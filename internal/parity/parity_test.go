// Package parity holds the pb-reference parity tests for the runtime
// module's public loaders: the .tzb/.tzm files built by embedenc are queried
// through github.com/ringsaturn/tzf/v2 and compared against pbref, the
// reference implementation over the pipeline's source protobuf.
//
// The source dataset is read from files so the tests keep working after
// tzf-dist stops publishing pb artifacts: set TZF_PARITY_TOPO to a
// *.compress.topo.bin and TZF_PARITY_PREINDEX to a *.preindex.bin (both
// produced by the pipeline); the tests skip when the variables are unset.
package parity

import (
	"errors"
	"math/rand"
	"os"
	"slices"
	"sync"
	"testing"

	tzf "github.com/ringsaturn/tzf/v2"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/embedenc"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/pbref"
)

var (
	loadOnce sync.Once
	topo     *pb.CompressedTopoTimezones
	preindex *pb.PreindexTimezones
	loadErr  error

	tzbBare  []byte // E profile, no FUZZY
	tzbFuzzy []byte // E profile, FUZZY embedded
	tzmBare  []byte // M profile, no FUZZY
	tzmFuzzy []byte // M profile, FUZZY embedded
)

func loadAll(t testing.TB) {
	t.Helper()
	topoPath := os.Getenv("TZF_PARITY_TOPO")
	prePath := os.Getenv("TZF_PARITY_PREINDEX")
	if topoPath == "" || prePath == "" {
		t.Skip("TZF_PARITY_TOPO / TZF_PARITY_PREINDEX not set; skipping pb parity tests")
	}
	loadOnce.Do(func() {
		raw, err := os.ReadFile(topoPath)
		if err != nil {
			loadErr = err
			return
		}
		topo = &pb.CompressedTopoTimezones{}
		if loadErr = pb.Unmarshal(raw, topo); loadErr != nil {
			return
		}
		raw, err = os.ReadFile(prePath)
		if err != nil {
			loadErr = err
			return
		}
		preindex = &pb.PreindexTimezones{}
		if loadErr = pb.Unmarshal(raw, preindex); loadErr != nil {
			return
		}
		if tzbBare, loadErr = embedenc.Encode(topo, embedenc.EncodeOptions{AllowShortcut: true}); loadErr != nil {
			return
		}
		if tzbFuzzy, loadErr = embedenc.Encode(topo, embedenc.EncodeOptions{AllowShortcut: true, Preindex: preindex}); loadErr != nil {
			return
		}
		if tzmBare, loadErr = embedenc.EncodeM(topo, embedenc.EncodeOptions{}); loadErr != nil {
			return
		}
		tzmFuzzy, loadErr = embedenc.EncodeM(topo, embedenc.EncodeOptions{Preindex: preindex})
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
}

func newRefs(t *testing.T) (*pbref.Finder, *pbref.Fuzzy) {
	t.Helper()
	ref, err := pbref.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	fz, err := pbref.NewFuzzy(preindex)
	if err != nil {
		t.Fatal(err)
	}
	return ref, fz
}

// composedWant is the single-name expectation for the public loaders over a
// FUZZY-carrying file: fuzzy tile first, polygon reference on a miss.
func composedWant(ref *pbref.Finder, fz *pbref.Fuzzy, lng, lat float64) string {
	if name := fz.GetTimezoneName(lng, lat); name != "" {
		return name
	}
	return ref.GetTimezoneName(lng, lat)
}

func TestFinderFromTZBSanity(t *testing.T) {
	loadAll(t)
	finder, err := tzf.NewFinderFromTZB(tzbBare)
	if err != nil {
		t.Fatal(err)
	}
	if finder.DataVersion() != topo.Version {
		t.Fatalf("DataVersion = %q, want %q", finder.DataVersion(), topo.Version)
	}
	if len(finder.TimezoneNames()) != len(topo.Timezones) {
		t.Fatalf("TimezoneNames count = %d, want %d", len(finder.TimezoneNames()), len(topo.Timezones))
	}
	if got := finder.GetTimezoneName(139.6917, 35.6895); got != "Asia/Tokyo" {
		t.Fatalf("Tokyo lookup = %q", got)
	}
	if got := finder.GetTimezoneName(0, 100); got != "" {
		t.Fatalf("out-of-domain lookup = %q", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = finder.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("GetTimezoneName allocations = %v", allocs)
	}
}

// TestFinderFromTZBParity checks the expansion loader over a bare file
// against the pure polygon reference.
func TestFinderFromTZBParity(t *testing.T) {
	loadAll(t)
	got, err := tzf.NewFinderFromTZB(tzbBare)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := newRefs(t)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		if a, b := got.GetTimezoneName(lng, lat), want.GetTimezoneName(lng, lat); a != b {
			t.Fatalf("single parity at (%f,%f): got %q want %q", lng, lat, a, b)
		}
		if i%100 == 0 {
			a, err := got.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			b, err := want.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a, b) {
				t.Fatalf("multi parity at (%f,%f): got %v want %v", lng, lat, a, b)
			}
		}
	}
}

// TestFinderFromTZBFuzzyFastPath checks the mode-C FUZZY fast path: over a
// file carrying a FUZZY section, GetTimezoneName answers fuzzy-first while
// GetTimezoneNames stays polygon-exact, and both remain allocation-free.
func TestFinderFromTZBFuzzyFastPath(t *testing.T) {
	loadAll(t)
	got, err := tzf.NewFinderFromTZB(tzbFuzzy)
	if err != nil {
		t.Fatal(err)
	}
	polyRef, fuzzyRef := newRefs(t)
	rng := rand.New(rand.NewSource(42))
	fuzzyHits := 0
	for i := 0; i < 10000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		if fuzzyRef.GetTimezoneName(lng, lat) != "" {
			fuzzyHits++
		}
		want := composedWant(polyRef, fuzzyRef, lng, lat)
		if a := got.GetTimezoneName(lng, lat); a != want {
			t.Fatalf("fast-path parity at (%f,%f): got %q want %q", lng, lat, a, want)
		}
		if i%100 == 0 {
			a, err := got.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			b, err := polyRef.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a, b) {
				t.Fatalf("multi parity at (%f,%f): got %v want %v", lng, lat, a, b)
			}
		}
	}
	if fuzzyHits == 0 {
		t.Fatal("no fuzzy tile hits in 10000 samples; fast path untested")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = got.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("fuzzy fast-path GetTimezoneName allocations = %v", allocs)
	}
}

// TestFinderFromTZMParity checks the memory-image loader against the polygon
// reference (bare file) and the composed expectation (FUZZY file).
func TestFinderFromTZMParity(t *testing.T) {
	loadAll(t)
	polyRef, fuzzyRef := newRefs(t)

	bare, err := tzf.NewFinderFromTZM(tzmBare)
	if err != nil {
		t.Fatal(err)
	}
	if bare.DataVersion() != topo.Version {
		t.Fatalf("DataVersion = %q, want %q", bare.DataVersion(), topo.Version)
	}
	if !slices.Equal(bare.TimezoneNames(), polyRef.TimezoneNames()) {
		t.Fatal("TimezoneNames differ")
	}
	composed, err := tzf.NewFinderFromTZM(tzmFuzzy)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		if a, b := bare.GetTimezoneName(lng, lat), polyRef.GetTimezoneName(lng, lat); a != b {
			t.Fatalf("bare single parity at (%f,%f): got %q want %q", lng, lat, a, b)
		}
		if a, b := composed.GetTimezoneName(lng, lat), composedWant(polyRef, fuzzyRef, lng, lat); a != b {
			t.Fatalf("composed single parity at (%f,%f): got %q want %q", lng, lat, a, b)
		}
		if i%100 == 0 {
			a, err := composed.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			b, err := polyRef.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a, b) {
				t.Fatalf("multi parity at (%f,%f): got %v want %v", lng, lat, a, b)
			}
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = composed.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("tzm GetTimezoneName allocations = %v", allocs)
	}
}

func TestProfileMismatch(t *testing.T) {
	loadAll(t)
	if _, err := tzf.NewFinderFromTZM(tzbBare); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("NewFinderFromTZM on .tzb = %v", err)
	}
	if _, err := tzf.NewFinderFromTZB(tzmBare); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("NewFinderFromTZB on .tzm = %v", err)
	}
}

func TestRejectsCorruption(t *testing.T) {
	loadAll(t)
	corrupt := slices.Clone(tzbBare)
	corrupt[len(corrupt)/2] ^= 1
	if _, err := tzf.NewFinderFromTZB(corrupt); err == nil {
		t.Fatal("NewFinderFromTZB accepted a file with an invalid CRC")
	}
}
