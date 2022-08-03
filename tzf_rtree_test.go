package tzf_test

import (
	"testing"

	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var treeFinder *tzf.FinderByRTree

func init() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrel.FullData, input); err != nil {
		panic(err)
	}
	_f, err := tzf.NewFinderByRTreeFromPB(input)
	if err != nil {
		panic(err)
	}
	treeFinder = _f
}

func BenchmarkFinderByRTree_GetTimezoneName(b *testing.B) {
	for i := 0; i <= b.N; i++ {
		treeFinder.GetTimezoneName(116.6386, 40.0786)
	}
}
