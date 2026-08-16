package tzf_test

import (
	"testing"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
)

var tzbFuzzyBenchmarkData = func() []byte {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		panic(err)
	}
	preindex := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfdist.PreindexData, preindex); err != nil {
		panic(err)
	}
	data, err := embedbin.Encode(&input, embedbin.EncodeOptions{AllowShortcut: true, Preindex: preindex})
	if err != nil {
		panic(err)
	}
	return data
}()

var tzbExpandedBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewFinderFromTZBExpanded(tzbBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

var tzbDefaultBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewDefaultFinderFromTZB(tzbFuzzyBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

var tzbFuzzyBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewFuzzyFinderFromTZB(tzbFuzzyBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

func BenchmarkExpandedTZBFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, tzbExpandedBenchmarkFinder)
}

func BenchmarkExpandedTZBFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbExpandedBenchmarkFinder)
}

func BenchmarkExpandedTZBFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, tzbExpandedBenchmarkFinder)
}

func BenchmarkTZBFuzzyFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbFuzzyBenchmarkFinder)
}

func BenchmarkDefaultFinderTZB_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbDefaultBenchmarkFinder)
}

func BenchmarkNewFinderFromTZBExpanded(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewFinderFromTZBExpanded(tzbBenchmarkData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewDefaultFinderFromTZB(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewDefaultFinderFromTZB(tzbFuzzyBenchmarkData); err != nil {
			b.Fatal(err)
		}
	}
}
