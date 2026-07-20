package tzf

import (
	"bytes"
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
	tzbTestOnce sync.Once
	tzbTestData []byte
	tzbTestTopo *pb.CompressedTopoTimezones
	tzbTestErr  error
)

func loadTZBTestData(t *testing.T) ([]byte, *pb.CompressedTopoTimezones) {
	t.Helper()
	tzbTestOnce.Do(func() {
		tzbTestTopo = &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, tzbTestTopo); err != nil {
			tzbTestErr = err
			return
		}
		tzbTestData, tzbTestErr = embedbin.Encode(tzbTestTopo, embedbin.EncodeOptions{AllowShortcut: true})
	})
	if tzbTestErr != nil {
		t.Fatal(tzbTestErr)
	}
	return tzbTestData, tzbTestTopo
}

func TestFinderFromTZB(t *testing.T) {
	data, topo := loadTZBTestData(t)
	finder, err := NewFinderFromTZB(data)
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
	if got := finder.GetTimezoneName(-74.006, 40.7128); got != "America/New_York" {
		t.Fatalf("New York lookup = %q", got)
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

func TestFinderFromTZBReaderAt(t *testing.T) {
	data, _ := loadTZBTestData(t)
	finder, err := NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
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

func TestFinderFromTZBParity(t *testing.T) {
	data, topo := loadTZBTestData(t)
	got, err := NewFinderFromTZB(data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewFinderFromCompressedTopo(proto.Clone(topo).(*pb.CompressedTopoTimezones))
	if err != nil {
		t.Fatal(err)
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
}

func TestFinderFromTZBRejectsCorruption(t *testing.T) {
	data, _ := loadTZBTestData(t)
	corrupt := slices.Clone(data)
	corrupt[len(corrupt)/2] ^= 1
	if _, err := NewFinderFromTZB(corrupt); err == nil {
		t.Fatal("NewFinderFromTZB accepted a file with an invalid CRC")
	}
}
