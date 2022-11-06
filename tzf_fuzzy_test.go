package tzf_test

import (
	"fmt"
	"math/rand"
	"testing"

	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var (
	fuzzyFinder *tzf.FuzzyFinder
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
	for i := 0; i <= b.N; i++ {
		p := gocitiesjson.Cities[rand.Intn(len(gocitiesjson.Cities))]
		_ = fuzzyFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}
