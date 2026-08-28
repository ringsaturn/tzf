package tzf_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	gocitiesjson "github.com/ringsaturn/go-cities.json"
	tzfdist "github.com/ringsaturn/tzf-dist"

	tzf "github.com/ringsaturn/tzf/v2"
)

// The three pre-defined finders share one dataset (§3 of the v2 plan); the
// byte constructors are exercised over the same embedded artifacts.
var embeddedFinder tzf.F = func() tzf.F {
	f, err := tzf.NewEmbeddedFinder()
	if err != nil {
		panic(err)
	}
	return f
}()

var fullFinder tzf.F = func() tzf.F {
	f, err := tzf.NewFullFinder()
	if err != nil {
		panic(err)
	}
	return f
}()

var tzbFinder tzf.F = func() tzf.F {
	f, err := tzf.NewFinderFromTZB(tzfdist.LiteTZB)
	if err != nil {
		panic(err)
	}
	return f
}()

func TestDataVersionsMatch(t *testing.T) {
	want := defaultFinder.DataVersion()
	if want == "" {
		t.Fatal("empty data version")
	}
	for name, f := range map[string]tzf.F{
		"NewEmbeddedFinder": embeddedFinder,
		"NewFullFinder":     fullFinder,
		"NewFinderFromTZB":  tzbFinder,
	} {
		if got := f.DataVersion(); got != want {
			t.Errorf("%s DataVersion = %q, want %q", name, got, want)
		}
	}
}

func TestTimezoneNamesConsistent(t *testing.T) {
	want := defaultFinder.TimezoneNames()
	if len(want) == 0 {
		t.Fatal("no timezone names")
	}
	for name, f := range map[string]tzf.F{
		"NewEmbeddedFinder": embeddedFinder,
		"NewFullFinder":     fullFinder,
	} {
		if got := f.TimezoneNames(); !slices.Equal(got, want) {
			t.Errorf("%s TimezoneNames differ from NewDefaultFinder", name)
		}
	}
}

func TestKnownCities(t *testing.T) {
	for _, tc := range []struct {
		lng, lat float64
		want     string
	}{
		{139.6917, 35.6895, "Asia/Tokyo"},
		{-74.006, 40.7128, "America/New_York"},
		{151.2093, -33.8688, "Australia/Sydney"},
		{116.3833, 39.9167, "Asia/Shanghai"},
		{13.4050, 52.5200, "Europe/Berlin"},
	} {
		for name, f := range map[string]tzf.F{
			"NewDefaultFinder":  defaultFinder,
			"NewEmbeddedFinder": embeddedFinder,
			"NewFullFinder":     fullFinder,
		} {
			if got := f.GetTimezoneName(tc.lng, tc.lat); got != tc.want {
				t.Errorf("%s GetTimezoneName(%v,%v) = %q, want %q", name, tc.lng, tc.lat, got, tc.want)
			}
		}
	}
}

func TestQueriesAreAllocationFree(t *testing.T) {
	for name, f := range map[string]tzf.F{
		"NewDefaultFinder":  defaultFinder,
		"NewEmbeddedFinder": embeddedFinder,
		"NewFullFinder":     fullFinder,
	} {
		if allocs := testing.AllocsPerRun(100, func() {
			_ = f.GetTimezoneName(139.6917, 35.6895)
		}); allocs != 0 {
			t.Errorf("%s GetTimezoneName allocations = %v", name, allocs)
		}
	}
}

// Benchmark helpers use time.Now() per-call timing (nanosecond precision) and
// report p50/p90/p99 via b.ReportMetric. Output format is parsed by
// scripts/bench2summary.py.

const benchPoolSize = 10_000

const edgeCasePoolSize = 4_000

func benchEdge(b *testing.B, f tzf.F) {
	b.Helper()
	pool := makeEdgePool(edgeCasePoolSize)
	ns := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := pool[i%len(pool)]
		start := time.Now()
		_ = f.GetTimezoneName(p.Lng, p.Lat)
		ns[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	reportPercentiles(b, ns)
}

func benchRandom(b *testing.B, f tzf.F) {
	b.Helper()
	pool := makePool(benchPoolSize)
	ns := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := pool[i%benchPoolSize]
		start := time.Now()
		_ = f.GetTimezoneName(p.Lng, p.Lat)
		ns[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	reportPercentiles(b, ns)
}

func benchRandomNames(b *testing.B, f tzf.F) {
	b.Helper()
	pool := makePool(benchPoolSize)
	ns := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := pool[i%benchPoolSize]
		start := time.Now()
		_, _ = f.GetTimezoneNames(p.Lng, p.Lat)
		ns[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	reportPercentiles(b, ns)
}

func makePool(n int) []*gocitiesjson.City {
	pool := make([]*gocitiesjson.City, n)
	for i := range pool {
		pool[i] = gocitiesjson.Random()
	}
	return pool
}

// makeEdgePool collects cities the FUZZY fast path cannot answer, so edge
// benchmarks measure the polygon slow path.
func makeEdgePool(n int) []*gocitiesjson.City {
	pool := make([]*gocitiesjson.City, 0, n)
	for city := range gocitiesjson.All(true) {
		if tzf.FuzzyMissForTest(defaultFinder, city.Lng, city.Lat) {
			pool = append(pool, city)
		}
		if len(pool) >= n {
			break
		}
	}
	return pool
}

func reportPercentiles(b *testing.B, ns []int64) {
	b.Helper()
	slices.Sort(ns)
	n := len(ns)
	b.ReportMetric(float64(ns[n/2]), "ns/p50")
	b.ReportMetric(float64(ns[n*9/10]), "ns/p90")
	b.ReportMetric(float64(ns[min(n*99/100, n-1)]), "ns/p99")
}

func BenchmarkEmbeddedFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, embeddedFinder)
}

func BenchmarkEmbeddedFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, embeddedFinder)
}

func BenchmarkEmbeddedFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, embeddedFinder)
}

func BenchmarkFullFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, fullFinder)
}

func BenchmarkFullFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, fullFinder)
}

func BenchmarkFullFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, fullFinder)
}

func BenchmarkFinderFromTZB_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbFinder)
}

func ExampleNewEmbeddedFinder() {
	finder, err := tzf.NewEmbeddedFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func ExampleNewFullFinder() {
	finder, err := tzf.NewFullFinder()
	if err != nil {
		panic(err)
	}

	fmt.Println(finder.GetTimezoneName(139.6917, 35.6895))
	// Output: Asia/Tokyo
}
