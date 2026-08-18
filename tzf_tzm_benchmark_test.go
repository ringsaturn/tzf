package tzf_test

import (
	"testing"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
)

var tzmBenchmarkData = func() []byte {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		panic(err)
	}
	data, err := embedbin.EncodeM(&input, embedbin.EncodeOptions{})
	if err != nil {
		panic(err)
	}
	return data
}()

var tzmFuzzyBenchmarkData = func() []byte {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		panic(err)
	}
	preindex := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfdist.PreindexData, preindex); err != nil {
		panic(err)
	}
	data, err := embedbin.EncodeM(&input, embedbin.EncodeOptions{Preindex: preindex})
	if err != nil {
		panic(err)
	}
	return data
}()

var tzmBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewFinderFromTZM(tzmBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

var tzmDefaultBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewDefaultFinderFromTZM(tzmFuzzyBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

func BenchmarkTZMFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, tzmBenchmarkFinder)
}

func BenchmarkTZMFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzmBenchmarkFinder)
}

func BenchmarkTZMFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, tzmBenchmarkFinder)
}

func BenchmarkDefaultFinderTZM_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzmDefaultBenchmarkFinder)
}

func BenchmarkNewFinderFromTZM(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewFinderFromTZM(tzmBenchmarkData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewDefaultFinderFromTZM(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewDefaultFinderFromTZM(tzmFuzzyBenchmarkData); err != nil {
			b.Fatal(err)
		}
	}
}
