// TZF's Python binding
package main

//#cgo LDFLAGS:
//#include <stdlib.h>
import "C"
import (
	"unsafe"

	"github.com/ringsaturn/tzf"
)

var (
	finder         *tzf.DefaultFinder
	timezoneCounts int
)

func init() {
	_finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	finder = _finder
	timezoneCounts = len(finder.TimezoneNames())
}

// GetTZ for Python calling, please ensure calling [FreeChar] afer got value.
//
//export GetTZ
func GetTZ(lng *C.float, lat *C.float) *C.char {
	return C.CString(finder.GetTimezoneName(float64(*lng), float64(*lat)))
}

// FreeChar for other side to free *C.char which come free [GetTZ]
//
//export FreeChar
func FreeChar(input unsafe.Pointer) {
	C.free(input)
}

func goStringSliceToC(stringSlice []string) **C.char {
	cArray := C.malloc(C.size_t(len(stringSlice)) * C.size_t(unsafe.Sizeof(uintptr(0))))

	a := (*[1<<30 - 1]*C.char)(cArray)

	for i, value := range stringSlice {
		a[i] = C.CString(value)
	}

	return (**C.char)(cArray)
}

// NOTE(ringsaturn): this func should only call once since no free mem method for return value.
//
//export TimezoneNames
func TimezoneNames() **C.char {
	return goStringSliceToC(finder.TimezoneNames())
}

// NOTE(ringsaturn): this func should only call once since no free mem method for return value.
//
//export CountTimezoneNames
func CountTimezoneNames() C.long {
	return C.long(timezoneCounts)
}

func main() {
}
