package tzf_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var (
	fuzzyFinder tzf.F
)

func init() {
	input := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfrel.PreindexData, input); err != nil {
		panic(err)
	}
	_fuzzyFinder, err := tzf.NewFuzzyFinderFromPB(input)
	if err != nil {
		panic(err)
	}
	fuzzyFinder = _fuzzyFinder
}

func TestFuzzySupports(t *testing.T) {
	failCount := 0
	for _, city := range gocitiesjson.Cities {
		name := fuzzyFinder.GetTimezoneName(city.Lng, city.Lat)
		if name == "" {
			failCount += 1
		}
	}
	// more than 10%
	if failCount/len(gocitiesjson.Cities)*100 > 10 {
		t.Errorf("has too many covered cities %v", failCount)
	}
}

func ExampleFuzzyFinder_GetTimezoneName() {
	input := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfrel.PreindexData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFuzzyFinderFromPB(input)
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func BenchmarkFuzzyFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Cities[rand.Intn(len(gocitiesjson.Cities))]
		_ = fuzzyFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}

func FuzzFuzzyFinder_GetTimezoneName(f *testing.F) {
	f.Add(116.3883, 39.9289)
	f.Fuzz(func(t *testing.T, a float64, b float64) {
		ret, err := fuzzyFinder.GetTimezoneNames(a, b)
		if err == nil && len(ret) == 0 {
			t.Errorf("bad return %v, %v", ret, err)
		}
	})
}
