package tzf_test

import (
	"os"
	"testing"

	"github.com/loov/hrtime/hrtesting"
	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var bitmapFinder *tzf.BitmapFinder

func init() {
	fp := "preindex-bitmap.pb"
	inputData, err := os.ReadFile(fp)
	if err != nil {
		panic(err)
	}

	input := &pb.PreindexBitmaps{}

	if err := proto.Unmarshal(inputData, input); err != nil {
		panic(err)
	}

	bitmapFinder, err = tzf.NewBitmapFinderFrompb(input)
	if err != nil {
		panic(err)
	}
}

func BenchmarkBitmapFinder_GetTimezoneName_Random_WorldCities(b *testing.B) {
	bench := hrtesting.NewBenchmark(b)
	defer bench.Report()
	for bench.Next() {
		p := gocitiesjson.Random()
		_ = bitmapFinder.GetTimezoneName(p.Lng, p.Lat)
	}
}
