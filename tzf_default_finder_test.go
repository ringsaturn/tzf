package tzf_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ringsaturn/tzf/v2"
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

func ExampleNewDefaultFinder() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786)) // In longitude-latitude order
	// Output: Asia/Shanghai
}

func ExampleNewDefaultFinder_getTimezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.GetTimezoneNames(87.6168, 43.8254)) // In longitude-latitude order
	// Output: [Asia/Shanghai Asia/Urumqi] <nil>
}

func ExampleNewDefaultFinder_timezoneNames() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	fmt.Println(finder.TimezoneNames())
}

// ExampleGeoJSONer shows how to reach the geometry export: constructors
// return tzf.F, so the behavior is asserted rather than a concrete type.
func ExampleGeoJSONer() {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}

	exporter, ok := finder.(tzf.GeoJSONer)
	fmt.Println("exports geometry:", ok)

	boundary, err := exporter.GetTZGeoJSON("Asia/Tokyo")
	if err != nil {
		panic(err)
	}
	fmt.Println("FeatureCollection:", bytes.HasPrefix(boundary, []byte(`{"type":"FeatureCollection"`)))

	// The tiles GetTimezoneName answers from without exact
	// point-in-polygon work are exportable too.
	tiles, err := exporter.GetTZPreindexGeoJSON("Asia/Tokyo")
	if err != nil {
		panic(err)
	}
	fmt.Println("preindex tiles:", bytes.HasPrefix(tiles, []byte(`{"type":"FeatureCollection"`)))

	// Output:
	// exports geometry: true
	// FeatureCollection: true
	// preindex tiles: true
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
