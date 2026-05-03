package tzf_test

import (
	"fmt"
	"testing"

	"github.com/ringsaturn/tzf"
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
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786)) // In longitude-latitude order
	// Output: Asia/Shanghai
}

func ExampleDefaultFinder_GetTimezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneNames(87.6168, 43.8254)) // In longitude-latitude order
	// Output: [Asia/Shanghai Asia/Urumqi] <nil>
}

func ExampleDefaultFinder_TimezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.TimezoneNames())
}

func BenchmarkDefaultFinder_GetTimezoneNameAtEdge(b *testing.B) {
	b.ReportAllocs()
	benchEdge(b, defaultFinder)
}

func BenchmarkDefaultFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandom(b, defaultFinder)
}

func BenchmarkDefaultFinder_GetTimezoneNames_Random_WorldCities(b *testing.B) {
	b.ReportAllocs()
	benchRandomNames(b, defaultFinder)
}
