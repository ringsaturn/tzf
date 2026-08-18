package tzf

import (
	"github.com/ringsaturn/tzf/internal/embedbin"
)

// NewFinderFromTZBExpanded builds a [Finder] by expanding a TZF embedded
// binary file's geometry into the materialized int32 polygon engine at load
// time (one sequential decode pass, no protobuf). Queries then run at full
// [Finder] speed and return the same results as [NewFinderFromCompressedTopo]
// over the source dataset.
//
// The expansion removes the duplicated junction vertices that the compressed
// topology path retains; they are zero-length PIP no-ops, so query results
// are identical, but ring vertex lists exported via [Finder.GetGeoJSON] may
// omit those duplicates. When the file carries a GRID section the grid
// candidate index is loaded too, including its single-candidate shortcut;
// the file's shortcut flag governs only the in-place reader
// ([NewFinderFromTZB]).
func NewFinderFromTZBExpanded(data []byte, opts ...OptionFunc) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	return newFinderFromTZBReader(reader, opts...)
}

func newFinderFromTZBReader(reader *embedbin.Reader, opts ...OptionFunc) (*Finder, error) {
	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}
	expanded, err := reader.Expand()
	if err != nil {
		return nil, err
	}
	items := assembleI32Items(expanded.Names, expanded.Polygons)
	return &Finder{
		core:    &finderImpl[int32]{items: items, grid: expanded.Grid},
		names:   expanded.Names,
		opt:     opt,
		version: expanded.Version,
	}, nil
}

// NewFuzzyFinderFromTZB builds a [FuzzyFinder] from a TZF embedded binary
// file's FUZZY section: one pass over the sorted tile keys rebuilds the same
// hash maps [NewFuzzyFinderFromPB] produces (~2.4 MB heap on the bundled
// preindex), so query latency matches the protobuf-loaded fuzzy finder. The
// source bytes are not retained. It fails when the file has no FUZZY section.
//
// Unlike [NewFuzzyFinderFromPB], which derives its name list from the
// preindex keys, TimezoneNames reports the file's full NAMES list.
func NewFuzzyFinderFromTZB(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	return newFuzzyFinderFromTZBReader(reader)
}

func newFuzzyFinderFromTZBReader(reader *embedbin.Reader) (*FuzzyFinder, error) {
	if !reader.HasFuzzy() {
		return nil, embedbin.ErrNoFuzzy
	}
	names := make([]string, reader.TimezoneCount())
	for i := range names {
		name, err := reader.Name(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
	}
	single, multi, err := reader.FuzzyMaps()
	if err != nil {
		return nil, err
	}
	idxZoom, aggZoom := reader.FuzzyZooms()
	return &FuzzyFinder{
		idxZoom: idxZoom,
		aggZoom: aggZoom,
		single:  single,
		multi:   multi,
		version: reader.DataVersion(),
		names:   names,
	}, nil
}

// NewDefaultFinderFromTZB builds a [DefaultFinder] from one TZF embedded
// binary file carrying both geometry and a FUZZY section: a [FuzzyFinder]
// rebuilt from FUZZY handles the fast path, polygon fallback uses the
// expanded finder. Both views share the file's data version, so no
// cross-artifact version check is needed, and the source bytes are released
// after loading.
func NewDefaultFinderFromTZB(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	fuzzyFinder, err := newFuzzyFinderFromTZBReader(reader)
	if err != nil {
		return nil, err
	}
	finder, err := newFinderFromTZBReader(reader, SetDropPBTZ)
	if err != nil {
		return nil, err
	}
	return &DefaultFinder{fuzzyFinder: fuzzyFinder, finder: finder}, nil
}

// ErrNoFuzzySection reports a .tzb file without an embedded FUZZY section.
var ErrNoFuzzySection = embedbin.ErrNoFuzzy
