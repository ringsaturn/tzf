package tzf_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
)

var (
	defaultFinder *tzf.DefaultFinder
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

func BenchmarkDefaultFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Cities[rand.Intn(len(gocitiesjson.Cities))]
		_ = defaultFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}
