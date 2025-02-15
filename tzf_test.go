package tzf_test

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfrellite "github.com/ringsaturn/tzf-rel-lite"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/tidwall/lotsa"
	"google.golang.org/protobuf/proto"
)

var finder tzf.F = func() tzf.F {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrellite.LiteData, input); err != nil {
		panic(err)
	}
	finder, err := tzf.NewFinderFromPB(input)
	if err != nil {
		panic(err)
	}
	return finder
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

func ExampleFinder_GetTimezoneName() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrellite.LiteData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func ExampleFinder_GetTimezoneNames() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrellite.LiteData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	fmt.Println(finder.GetTimezoneNames(87.6168, 43.8254))
	// Output: [Asia/Shanghai Asia/Urumqi] <nil>
}

func ExampleFinder_TimezoneNames() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrellite.LiteData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
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
