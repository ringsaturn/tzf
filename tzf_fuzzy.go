package tzf

import (
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// fuzzyIndex answers queries from the preindex tile maps rebuilt out of a
// file's FUZZY section. It is the fast path inside the composed finders and
// is not a public mechanism in v2 (spec §5.2): a query it cannot answer
// unambiguously falls through to the polygon finder.
//
// Tiles are split into two maps for memory efficiency:
//   - single: tiles that belong to exactly one timezone (vast majority); value is a names index.
//   - multi:  tiles that straddle a timezone boundary; value is a slice of names indices.
type fuzzyIndex struct {
	idxZoom int
	aggZoom int
	single  map[geom.TileID]uint16   // tile → single timezone index into names
	multi   map[geom.TileID][]uint16 // tile → multiple timezone indices (boundary tiles only)
	names   []string
}

// newFuzzyIndexFromReader rebuilds the preindex hash maps from a file's FUZZY
// section: one pass over the sorted tile keys (~2.4 MB heap on the bundled
// preindex). The source bytes are not retained. It fails when the file has no
// FUZZY section.
func newFuzzyIndexFromReader(reader *embedbin.Reader) (*fuzzyIndex, error) {
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
	return &fuzzyIndex{
		idxZoom: idxZoom,
		aggZoom: aggZoom,
		single:  single,
		multi:   multi,
		names:   names,
	}, nil
}

// getTimezoneName returns the tile answer, or "" when no tile covers the
// point (including ambiguity-free misses); the caller falls back to polygons.
func (f *fuzzyIndex) getTimezoneName(lng float64, lat float64) string {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		t := tile.Shift(uint8(f.idxZoom - z))
		if idx, ok := f.single[t]; ok {
			return f.names[idx]
		}
		if idxs, ok := f.multi[t]; ok {
			return f.names[idxs[0]]
		}
	}
	return ""
}
