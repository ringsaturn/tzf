package tzf_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	"github.com/tidwall/lotsa"
)

var (
	defaultFinder tzf.F
)

func init() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	defaultFinder = finder
}

func ExampleDefaultFinder_GetTimezoneName() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func ExampleDefaultFinder_GetTimezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneNames(87.6168, 43.8254))
	// Output: [Asia/Shanghai Asia/Urumqi] <nil>
}

func ExampleDefaultFinder_TimezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.TimezoneNames())
}

func BenchmarkDefaultFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Cities[rand.Intn(len(gocitiesjson.Cities))]
		_ = defaultFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}

func Test_DefaultFinder_GetTimezoneName_Random_WorldCities_Alll(t *testing.T) {
	wri := bytes.NewBufferString("")
	lotsa.Output = wri
	lotsa.Ops(len(gocitiesjson.Cities), runtime.NumCPU(), func(i, _ int) {
		city := gocitiesjson.Cities[i]
		_ = defaultFinder.GetTimezoneName(city.Lng, city.Lat)
	})
	testing.Verbose()
	t.Log(wri.String())
}
