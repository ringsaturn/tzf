package x_test

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"sync"
	"testing"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"github.com/ringsaturn/tzf/x"
	"google.golang.org/protobuf/proto"
)

var (
	testDataOnce sync.Once
	testData     []byte
	testDataErr  error
)

func loadTZB(t *testing.T) []byte {
	t.Helper()
	testDataOnce.Do(func() {
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			testDataErr = err
			return
		}
		testData, testDataErr = embedbin.Encode(input, embedbin.EncodeOptions{AllowShortcut: true})
	})
	if testDataErr != nil {
		t.Fatal(testDataErr)
	}
	return testData
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
	if allocs := testing.AllocsPerRun(100, func() {
		_ = finder.GetTimezoneName(151.2093, -33.8688)
	}); allocs != 0 {
		t.Fatalf("ReaderAt GetTimezoneName allocations = %v", allocs)
	}
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
		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		wantJSON, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Fatalf("%s: GeoJSON differs between ReaderAt and byte-backed finders", name)
		}
	}
}
