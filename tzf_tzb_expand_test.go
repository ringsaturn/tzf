package tzf

import (
	"math/rand"
	"slices"
	"sync"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
)

var (
	tzbFuzzyTestOnce     sync.Once
	tzbFuzzyTestData     []byte
	tzbFuzzyTestPreindex *pb.PreindexTimezones
	tzbFuzzyTestErr      error
)

// loadTZBWithFuzzy encodes the bundled lite dataset with its preindex
// embedded as the FUZZY section.
func loadTZBWithFuzzy(t testing.TB) ([]byte, *pb.PreindexTimezones) {
	t.Helper()
	tzbFuzzyTestOnce.Do(func() {
		topo := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, topo); err != nil {
			tzbFuzzyTestErr = err
			return
		}
		tzbFuzzyTestPreindex = &pb.PreindexTimezones{}
		if err := proto.Unmarshal(tzfdist.PreindexData, tzbFuzzyTestPreindex); err != nil {
			tzbFuzzyTestErr = err
			return
		}
		tzbFuzzyTestData, tzbFuzzyTestErr = embedbin.Encode(topo, embedbin.EncodeOptions{
			AllowShortcut: true, Preindex: tzbFuzzyTestPreindex,
		})
	})
	if tzbFuzzyTestErr != nil {
		t.Fatal(tzbFuzzyTestErr)
	}
	return tzbFuzzyTestData, tzbFuzzyTestPreindex
}

func TestFinderFromTZBExpandedParity(t *testing.T) {
	data, topo := loadTZBTestData(t)
	got, err := NewFinderFromTZBExpanded(data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewFinderFromCompressedTopo(proto.Clone(topo).(*pb.CompressedTopoTimezones))
	if err != nil {
		t.Fatal(err)
	}
	if got.DataVersion() != want.DataVersion() {
		t.Fatalf("DataVersion = %q, want %q", got.DataVersion(), want.DataVersion())
	}
	if !slices.Equal(got.TimezoneNames(), want.TimezoneNames()) {
		t.Fatal("TimezoneNames differ")
	}
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
	if allocs := testing.AllocsPerRun(100, func() {
		_ = got.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("expanded GetTimezoneName allocations = %v", allocs)
	}
}

func TestFuzzyFinderFromTZBParity(t *testing.T) {
	data, preindex := loadTZBWithFuzzy(t)
	got, err := NewFuzzyFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewFuzzyFinderFromPB(proto.Clone(preindex).(*pb.PreindexTimezones))
	if err != nil {
		t.Fatal(err)
	}
	if got.DataVersion() != want.DataVersion() {
		t.Fatalf("DataVersion = %q, want %q", got.DataVersion(), want.DataVersion())
	}
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50000; i++ {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		if a, b := got.GetTimezoneName(lng, lat), want.GetTimezoneName(lng, lat); a != b {
			t.Fatalf("fuzzy single parity at (%f,%f): got %q want %q", lng, lat, a, b)
		}
		if i%100 == 0 {
			a, aErr := got.GetTimezoneNames(lng, lat)
			b, bErr := want.GetTimezoneNames(lng, lat)
			if (aErr == nil) != (bErr == nil) {
				t.Fatalf("fuzzy multi parity at (%f,%f): errors %v vs %v", lng, lat, aErr, bErr)
			}
			if !slices.Equal(a, b) {
				t.Fatalf("fuzzy multi parity at (%f,%f): got %v want %v", lng, lat, a, b)
			}
		}
	}
}

func TestFuzzyFinderFromTZBRequiresSection(t *testing.T) {
	data, _ := loadTZBTestData(t) // encoded without a preindex
	if _, err := NewFuzzyFinderFromTZB(data); err != ErrNoFuzzySection {
		t.Fatalf("NewFuzzyFinderFromTZB without FUZZY = %v", err)
	}
	if _, err := NewDefaultFinderFromTZB(data); err != ErrNoFuzzySection {
		t.Fatalf("NewDefaultFinderFromTZB without FUZZY = %v", err)
	}
}

func TestDefaultFinderFromTZB(t *testing.T) {
	data, _ := loadTZBWithFuzzy(t)
	finder, err := NewDefaultFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		lng, lat float64
		want     string
	}{
		{139.6917, 35.6895, "Asia/Tokyo"},
		{-74.006, 40.7128, "America/New_York"},
		{151.2093, -33.8688, "Australia/Sydney"},
		{116.3833, 39.9167, "Asia/Shanghai"},
	} {
		if got := finder.GetTimezoneName(tc.lng, tc.lat); got != tc.want {
			t.Fatalf("GetTimezoneName(%v,%v) = %q, want %q", tc.lng, tc.lat, got, tc.want)
		}
	}
	names, err := finder.GetTimezoneNames(116.3833, 39.9167)
	if err != nil || !slices.Contains(names, "Asia/Shanghai") {
		t.Fatalf("GetTimezoneNames = %v, %v", names, err)
	}
	if _, err := finder.(*DefaultFinder).GetTZGeoJSON("Asia/Tokyo"); err != nil {
		t.Fatalf("GetTZGeoJSON = %v", err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = finder.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("default GetTimezoneName allocations = %v", allocs)
	}
}
