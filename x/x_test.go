package x_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"

	tzf "github.com/ringsaturn/tzf/v2"
	"github.com/ringsaturn/tzf/v2/x"
)

func loadTZB(t *testing.T) []byte {
	t.Helper()
	return tzfdist.LiteTZB
}

func TestNewFinderFromTZBReaderAt(t *testing.T) {
	data := loadTZB(t)
	finder, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got := finder.GetTimezoneName(151.2093, -33.8688); got != "Australia/Sydney" {
		t.Fatalf("Sydney lookup = %q", got)
	}
	if raceEnabled {
		// sync.Pool.Put drops one item in four under the race detector, so
		// the pooled decode views are refilled at random and the steady
		// state is not allocation-free there.
		t.Log("skipping allocation check under -race")
		return
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = finder.GetTimezoneName(151.2093, -33.8688)
	}); allocs != 0 {
		t.Fatalf("ReaderAt GetTimezoneName allocations = %v", allocs)
	}
}

// TestReaderAtConcurrentQueries runs polygon-path queries from many
// goroutines. Each query decodes through a pooled view with its own
// workspace; under -race this catches any buffer the views still share.
func TestReaderAtConcurrentQueries(t *testing.T) {
	data := loadTZB(t)
	finder, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := tzf.NewFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for i := 0; i < 500; i++ {
				lng := rng.Float64()*360 - 180
				lat := rng.Float64()*180 - 90
				if a, b := finder.GetTimezoneName(lng, lat), want.GetTimezoneName(lng, lat); a != b {
					t.Errorf("(%f,%f): got %q want %q", lng, lat, a, b)
					return
				}
				if _, err := finder.GetTimezoneNames(lng, lat); err != nil {
					t.Error(err)
					return
				}
			}
		}(int64(g))
	}
	wg.Wait()
}

// TestReaderAtMatchesByteBacked checks that reading through an io.ReaderAt
// answers exactly like the byte-backed finder over the same file: the two
// share one implementation and differ only in how bytes are fetched.
func TestReaderAtMatchesByteBacked(t *testing.T) {
	data := loadTZB(t)
	got, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := tzf.NewFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataVersion() != want.DataVersion() {
		t.Fatalf("DataVersion = %q, want %q", got.DataVersion(), want.DataVersion())
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 2000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		if a, b := got.GetTimezoneName(lng, lat), want.GetTimezoneName(lng, lat); a != b {
			t.Fatalf("single at (%f,%f): got %q want %q", lng, lat, a, b)
		}
		if i%50 == 0 {
			a, err := got.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			b, err := want.GetTimezoneNames(lng, lat)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a, b) {
				t.Fatalf("multi at (%f,%f): got %v want %v", lng, lat, a, b)
			}
		}
	}
}

// TestReaderAtGeoJSON checks the finder satisfies tzf.GeoJSONer and exports
// the same geometry the byte-backed finder does.
func TestReaderAtGeoJSON(t *testing.T) {
	data := loadTZB(t)
	finder, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	exporter, ok := finder.(tzf.GeoJSONer)
	if !ok {
		t.Fatal("ReaderAt finder does not satisfy tzf.GeoJSONer")
	}
	byteBacked, err := tzf.NewFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Asia/Tokyo", "Pacific/Auckland"} {
		got, err := exporter.GetTZGeoJSON(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, err := byteBacked.(tzf.GeoJSONer).GetTZGeoJSON(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: GeoJSON differs between ReaderAt and byte-backed finders", name)
		}
	}
}

// ExampleNewFinderFromTZBReaderAt shows the in-place route over
// caller-owned bytes: v2 has no InPlace option on the root constructors, so
// wrap the bytes in a bytes.Reader and open them here.
func ExampleNewFinderFromTZBReaderAt() {
	// Any io.ReaderAt works: an *os.File, an mmap'd region, or — as here —
	// bytes already in memory.
	data := tzfdist.LiteTZB

	finder, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneName(116.3883, 39.9289)) // longitude, latitude
	// Output: Asia/Shanghai
}
