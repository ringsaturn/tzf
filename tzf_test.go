package tzf_test

import (
	"fmt"
	"strings"
	"testing"

	"sort"

	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var f *tzf.Finder

func init() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrel.LiteData, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	f = finder
}

func BenchmarkGetTimezoneName(b *testing.B) {
	for i := 0; i <= b.N; i++ {
		_ = f.GetTimezoneName(116.6386, 40.0786)
	}
}

func BenchmarkGetTimezoneNameAtEdge(b *testing.B) {
	for i := 0; i <= b.N; i++ {
		_ = f.GetTimezoneName(110.8571, 43.1483)
	}
}

func ExampleFinder_GetTimezoneName() {
	input := &pb.Timezones{}

	// Lite data, about 16.7MB
	dataFile := tzfrel.LiteData

	// Full data, about 83.5MB
	// dataFile := tzfrel.FullData

	if err := proto.Unmarshal(dataFile, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	fmt.Println(finder.GetTimezoneName(116.6386, 40.0786))
	// Output: Asia/Shanghai
}

func ExampleFinder_GetTimezoneLoc() {
	input := &pb.Timezones{}

	// Lite data, about 16.7MB
	dataFile := tzfrel.LiteData

	// Full data, about 83.5MB
	// dataFile := tzfrel.FullData

	if err := proto.Unmarshal(dataFile, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	fmt.Println(finder.GetTimezoneLoc(116.6386, 40.0786))
	// Output: Asia/Shanghai <nil>
}

func ExampleFinder_GetTimezoneShapeByName() {
	input := &pb.Timezones{}

	// Lite data, about 16.7MB
	dataFile := tzfrel.LiteData

	if err := proto.Unmarshal(dataFile, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	pbtz, err := finder.GetTimezoneShapeByName("Asia/Shanghai")
	fmt.Printf("%v %v\n", pbtz.Name, err)
	// Output: Asia/Shanghai <nil>
}

func ExampleFinder_GetTimezoneShapeByShift() {
	input := &pb.Timezones{}

	// Lite data, about 16.7MB
	dataFile := tzfrel.LiteData

	if err := proto.Unmarshal(dataFile, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	pbtzs, _ := finder.GetTimezoneShapeByShift(28800)

	pbnames := make([]string, 0)
	for _, pbtz := range pbtzs {
		pbnames = append(pbnames, pbtz.Name)
	}
	sort.Strings(pbnames)

	fmt.Println(strings.Join(pbnames, ","))
	// Output: Asia/Brunei,Asia/Choibalsan,Asia/Hong_Kong,Asia/Irkutsk,Asia/Kuala_Lumpur,Asia/Kuching,Asia/Macau,Asia/Makassar,Asia/Manila,Asia/Shanghai,Asia/Singapore,Asia/Taipei,Asia/Ulaanbaatar,Australia/Perth,Etc/GMT-8
}
