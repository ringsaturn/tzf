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

//export GetTZ
func GetTZ(lng *C.float, lat *C.float) *C.char {
	ret := C.CString(finder.GetTimezoneName(float64(*lng), float64(*lat)))
	return ret
}

//export FreeChar
func FreeChar(input *C.char) {
	// fmt.Println("input", input)
	ptr := unsafe.Pointer(input)
	// fmt.Println("ptr", ptr)
	C.free(ptr)
	// fmt.Println("d")
	// runtime.GC()
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

//export CountTimezoneNames
func CountTimezoneNames() C.long {
	return C.long(timezoneCounts)
}

func main() {
}
