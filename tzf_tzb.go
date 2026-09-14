package tzf

import (
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/inplace"
)

// NewFinderFromTZB builds a finder from a TZF embedded binary (.tzb) file by
// expanding its geometry into the materialized int32 polygon engine at load
// time (one parallel decode pass, no protobuf). data is released after
// loading. Queries run at full polygon-finder speed.
//
// When the file carries a FUZZY section, GetTimezoneName consults the
// preindex tiles first and falls back to the polygon scan on a miss;
// GetTimezoneNames always uses the polygon scan. Files without FUZZY get the
// plain polygon finder.
//
// The expansion removes the duplicated junction vertices the compressed
// chunks retain; they are zero-length PIP no-ops, so query results are
// identical, but ring vertex lists exported via [GeoJSONer] may omit those
// duplicates.
//
// For in-place queries over caller-owned bytes (no expansion, ~30 KB heap),
// wrap the data in a [bytes.Reader] and use
// [github.com/ringsaturn/tzf/v2/x.NewFinderFromTZBReaderAt]; note the x
// package's stability caveat.
func NewFinderFromTZB(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	f, err := newFinderFromTZBReader(reader)
	if err != nil {
		return nil, err
	}
	if !reader.HasFuzzy() {
		return f, nil
	}
	fuzzy, err := newFuzzyIndexFromReader(reader)
	if err != nil {
		return nil, err
	}
	return &defaultFinder{fuzzy: fuzzy, finder: f}, nil
}

func newFinderFromTZBReader(reader *embedbin.Reader) (*finder, error) {
	expanded, err := reader.Expand()
	if err != nil {
		return nil, err
	}
	items := assembleI32Items(expanded.Names, expanded.Polygons)
	return &finder{
		core:    &finderImpl[int32]{items: items, grid: expanded.Grid},
		names:   expanded.Names,
		version: expanded.Version,
	}, nil
}

// ErrNoFuzzySection reports a .tzb file without an embedded FUZZY section.
var ErrNoFuzzySection = embedbin.ErrNoFuzzy

var (
	_ F         = (*inplace.Finder)(nil)
	_ GeoJSONer = (*inplace.Finder)(nil)
)
