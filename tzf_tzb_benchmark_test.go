package tzf_test

import (
	"bytes"
	"testing"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
)

var tzbBenchmarkData = func() []byte {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		panic(err)
	}
	data, err := embedbin.Encode(&input, embedbin.EncodeOptions{AllowShortcut: true})
	if err != nil {
		panic(err)
	}
	return data
}()

var tzbBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewFinderFromTZB(tzbBenchmarkData)
	if err != nil {
		panic(err)
	}
	return finder
}()

var tzbReaderAtBenchmarkFinder = func() tzf.F {
	finder, err := tzf.NewFinderFromTZBReaderAt(
		bytes.NewReader(tzbBenchmarkData),
		int64(len(tzbBenchmarkData)),
	)
	if err != nil {
		panic(err)
	}
	return finder
}()

func BenchmarkTZBFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, tzbBenchmarkFinder)
}

func BenchmarkTZBFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbBenchmarkFinder)
}

func BenchmarkTZBFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, tzbBenchmarkFinder)
}

func BenchmarkTZBFinderReaderAt_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, tzbReaderAtBenchmarkFinder)
}

func BenchmarkNewFinderFromTZB(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewFinderFromTZB(tzbBenchmarkData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewFinderFromTZBReaderAt(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tzf.NewFinderFromTZBReaderAt(
			bytes.NewReader(tzbBenchmarkData),
			int64(len(tzbBenchmarkData)),
		); err != nil {
			b.Fatal(err)
		}
	}
}
