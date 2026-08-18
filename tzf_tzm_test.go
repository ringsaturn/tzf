package tzf

import (
	"errors"
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
	tzmTestOnce      sync.Once
	tzmTestData      []byte
	tzmFuzzyTestData []byte
	tzmTestErr       error
)

// loadTZMTestData encodes the bundled lite dataset as M-profile files, one
// bare and one with the preindex embedded as the FUZZY section.
func loadTZMTestData(t testing.TB) (bare, withFuzzy []byte) {
	t.Helper()
	tzmTestOnce.Do(func() {
		topo := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, topo); err != nil {
			tzmTestErr = err
			return
		}
		preindex := &pb.PreindexTimezones{}
		if err := proto.Unmarshal(tzfdist.PreindexData, preindex); err != nil {
			tzmTestErr = err
			return
		}
		if tzmTestData, tzmTestErr = embedbin.EncodeM(topo, embedbin.EncodeOptions{}); tzmTestErr != nil {
			return
		}
		tzmFuzzyTestData, tzmTestErr = embedbin.EncodeM(topo, embedbin.EncodeOptions{Preindex: preindex})
	})
	if tzmTestErr != nil {
		t.Fatal(tzmTestErr)
	}
	return tzmTestData, tzmFuzzyTestData
}

func TestFinderFromTZMParity(t *testing.T) {
	data, _ := loadTZMTestData(t)
	got, err := NewFinderFromTZM(data)
	if err != nil {
		t.Fatal(err)
	}
	_, topo := loadTZBTestData(t)
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
		t.Fatalf("tzm GetTimezoneName allocations = %v", allocs)
	}
	if _, err := got.(*Finder).GetTZGeoJSON("Asia/Tokyo"); err != nil {
		t.Fatalf("GetTZGeoJSON = %v", err)
	}
}

func TestDefaultFinderFromTZM(t *testing.T) {
	_, data := loadTZMTestData(t)
	finder, err := NewDefaultFinderFromTZM(data)
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
	if allocs := testing.AllocsPerRun(100, func() {
		_ = finder.GetTimezoneName(139.6917, 35.6895)
	}); allocs != 0 {
		t.Fatalf("default tzm GetTimezoneName allocations = %v", allocs)
	}
}

func TestTZMDefaultFinderRequiresFuzzySection(t *testing.T) {
	data, _ := loadTZMTestData(t)
	if _, err := NewDefaultFinderFromTZM(data); err != ErrNoFuzzySection {
		t.Fatalf("NewDefaultFinderFromTZM without FUZZY = %v", err)
	}
}

func TestTZMProfileMismatch(t *testing.T) {
	tzb, _ := loadTZBTestData(t)
	tzm, _ := loadTZMTestData(t)
	if _, err := NewFinderFromTZM(tzb); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("NewFinderFromTZM on .tzb = %v", err)
	}
	if _, err := NewFinderFromTZB(tzm); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("NewFinderFromTZB on .tzm = %v", err)
	}
	if _, err := NewFinderFromTZBExpanded(tzm); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("NewFinderFromTZBExpanded on .tzm = %v", err)
	}
}
