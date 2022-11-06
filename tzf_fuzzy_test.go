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
	fuzzyFinder, _ = tzf.NewFuzzyFinderFromPB(input)
	// fmt.Println(fuzzyFinder.GetTimezoneName(116.3883, 39.9289))
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
