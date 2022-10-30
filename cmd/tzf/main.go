// tzf-cli tool for local query.
package main

import (
	"flag"
	"fmt"

	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var finder *tzf.Finder

func init() {
	input := &pb.CompressedTimezones{}
	dataFile := tzfrel.LiteCompressData
	err := proto.Unmarshal(dataFile, input)
	if err != nil {
		panic(err)
	}
	finder, err = tzf.NewFinderFromCompressed(input)
	if err != nil {
		panic(err)
	}
}

func main() {
	var lng float64
	var lat float64
	flag.Float64Var(&lng, "lng", 116.3883, "longitude")
	flag.Float64Var(&lat, "lat", 39.9289, "lontitude")
	flag.Parse()

	fmt.Println(finder.GetTimezoneName(lng, lat))
}
