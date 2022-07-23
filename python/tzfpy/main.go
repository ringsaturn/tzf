// TZF's Python binding
package main

import "C"
import (
	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var (
	finder *tzf.Finder
)

func init() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrel.FullData, input); err != nil {
		panic(err)
	}
	_finder, _ := tzf.NewFinderFromPB(input)
	finder = _finder
}

//export GetTZ
func GetTZ(lng *C.float, lat *C.float) *C.char {
	return C.CString(finder.GetTimezoneName(float64(*lng), float64(*lat)))
}

func main() {
}
