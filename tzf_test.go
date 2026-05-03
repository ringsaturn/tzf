package tzf_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"google.golang.org/protobuf/proto"
)

var finder tzf.F = func() tzf.F {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	finder, err := tzf.NewFinderFromCompressedTopo(input)
	if err != nil {
		panic(err)
	}
	return finder
}()

var fullFinder tzf.F = func() tzf.F {
	_finder, err := tzf.NewFullFinder()
	if err != nil {
		panic(err)
	}
	return _finder
}()

var fullFinderWithoutPreindex tzf.F = func() tzf.F {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.CompressTopoData, input); err != nil {
		panic(err)
	}
	_finder, err := tzf.NewFinderFromCompressedTopo(input)
	if err != nil {
		panic(err)
	}
	return _finder
}()

// finderNoGrid is a Finder loaded without a GridIndex, used to measure the
// tail-latency impact of GridIndex in benchmark comparisons.
var finderNoGrid tzf.F = func() tzf.F {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	input.GridIndex = nil
	f, err := tzf.NewFinderFromCompressedTopo(input)
	if err != nil {
		panic(err)
	}
	return f
}()

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
		p := pool[i%edgeCasePoolSize]
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

func makeEdgePool(n int) []*gocitiesjson.City {
	pool := make([]*gocitiesjson.City, 0, n)
	for city := range gocitiesjson.All(true) {
		fuzzyRes := fuzzyFinder.GetTimezoneName(city.Lng, city.Lat)
		if fuzzyRes == "" {
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

func BenchmarkGetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, finder)
}

func BenchmarkGetTimezoneNameAtEdge_FullFinder(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, fullFinder)
}

func BenchmarkGetTimezoneNameAtEdge_FullFinderWithoutPreindex(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, fullFinderWithoutPreindex)
}

func BenchmarkGetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, finder)
}

func BenchmarkGetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, finder)
}

func BenchmarkGetTimezoneName_Random_WorldCities_FullFinder(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, fullFinder)
}

func BenchmarkGetTimezoneNames_Random_WorldCities_FullFinder(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, fullFinder)
}

func BenchmarkGetTimezoneName_Random_WorldCities_FullFinderWithoutPreindex(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, fullFinderWithoutPreindex)
}

func BenchmarkGetTimezoneName_Random_WorldCities_GridIndex_WithGrid(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, finder)
}

func BenchmarkGetTimezoneName_Random_WorldCities_GridIndex_NoGrid(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, finderNoGrid)
}

func ExampleFinder_GetTimezoneName() {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromCompressedTopo(input)
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func ExampleFinder_GetTimezoneNames() {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromCompressedTopo(input)
	fmt.Println(finder.GetTimezoneNames(87.6168, 43.8254))
	// Output: [Asia/Shanghai Asia/Urumqi] <nil>
}

func ExampleFinder_TimezoneNames() {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromCompressedTopo(input)
	fmt.Println(finder.TimezoneNames())
}

func ExampleNewFullFinder() {
	finder, err := tzf.NewFullFinder()
	if err != nil {
		panic(err)
	}

	fmt.Println(finder.GetTimezoneName(139.6917, 35.6895))
	// Output: Asia/Tokyo
}
