package tzf

import (
	"github.com/ringsaturn/tzf/internal/embedbin"
	"github.com/ringsaturn/tzf/internal/inplace"
)

// NewFinderFromTZB builds a Finder over a byte-backed TZF embedded binary
// file. The file is validated when opened and retained without expanding its
// geometry into Go polygon objects.
//
// When the file carries a FUZZY section, GetTimezoneName consults it before
// the point-in-polygon scan — an in-place binary search, still allocation
// free — mirroring [DefaultFinder]: a tile hit answers directly (which may
// differ slightly from the polygon answer near tile borders), a miss falls
// through to the exact scan. GetTimezoneNames always uses the polygon scan.
//
// The returned value also satisfies [GeoJSONer], decoding geometry from the
// file on demand.
//
// To read from an [io.ReaderAt] instead of a byte slice, use
// [github.com/ringsaturn/tzf/x.NewFinderFromTZBReaderAt].
func NewFinderFromTZB(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	return inplace.New(reader)
}

var (
	_ F         = (*inplace.Finder)(nil)
	_ GeoJSONer = (*inplace.Finder)(nil)
)
