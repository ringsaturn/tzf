package tzf_test

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/tidwall/lotsa"
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
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	_finder, err := tzf.NewFinderFromCompressedTopo(input)
	if err != nil {
		panic(err)
	}
	return _finder
}()

func BenchmarkGetTimezoneName(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		_ = finder.GetTimezoneName(116.6386, 40.0786)
	}
}

func BenchmarkGetTimezoneNameAtEdge(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		_ = finder.GetTimezoneName(110.8571, 43.1483)
	}
}

func BenchmarkGetTimezoneName_Random_WorldCities(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Random()
		_ = finder.GetTimezoneName(p.Lng, p.Lat)
	}
}

func BenchmarkGetTimezoneName_Random_WorldCities_FullFinder(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Random()
		_ = fullFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}

func BenchmarkGetTimezoneName_Random_WorldCities_FullFinderWithoutPreindex(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Random()
		_ = fullFinderWithoutPreindex.GetTimezoneName(p.Lng, p.Lat)
	}
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

func Test_Finder_GetTimezoneName_Random_WorldCities_All(t *testing.T) {
	wri := bytes.NewBufferString("")
	lotsa.Output = wri
	lotsa.Ops(len(gocitiesjson.Cities), runtime.NumCPU(), func(i, _ int) {
		city := gocitiesjson.Cities[i]
		_ = finder.GetTimezoneName(city.Lng, city.Lat)
	})
	testing.Verbose()
	t.Log(wri.String())
}
