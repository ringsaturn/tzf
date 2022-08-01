// TZF's Python binding
package main

import "C"
import (
	"unsafe"

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

func goStringSliceToC(stringSlice []string) **C.char {
	cArray := C.malloc(C.size_t(len(stringSlice)) * C.size_t(unsafe.Sizeof(uintptr(0))))

	a := (*[1<<30 - 1]*C.char)(cArray)

	for i, value := range stringSlice {
		a[i] = C.CString(value)
	}

	return (**C.char)(cArray)
}

//export TimezoneNames
func TimezoneNames() **C.char {
	return goStringSliceToC(finder.TimezoneNames())
}

func main() {
}
