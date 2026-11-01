package tzf

import (
	"math"

	"github.com/ringsaturn/tzf/v2/internal/convert"
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

	// zoomRanges holds, per 1°×1° cell (zoomCellsLng × zoomCellsLat,
	// row-major from lat -90, lng -180), the coarsest and finest zoom of any
	// tile touching the cell, packed as zmin | zmax<<8; noTiles when none
	// does. A lookup probes only that zoom range: a tile covering a point
	// always touches the point's cell, so the skipped zooms can never match,
	// and the probe order (coarsest first) is unchanged.
	zoomRanges []uint16
}

const (
	zoomCellsLng = 360
	zoomCellsLat = 180
	noTiles      = math.MaxUint16
)

// zoomCell is the 1°×1° cell holding (lng, lat); the caller has
// range-checked both. lng = 180 and lat = 90 fold into the last column / row.
func zoomCell(lng, lat float64) int {
	cx := min(int(lng+180), zoomCellsLng-1)
	cy := min(int(lat+90), zoomCellsLat-1)
	return cy*zoomCellsLng + cx
}

// buildZoomRanges builds fuzzyIndex.zoomRanges from the reader's FUZZY keys.
//
// The keys arrive in ascending order, so zooms arrive coarsest first: the
// first zoom a cell sees is its minimum and the last is its maximum, and
// each touched cell costs one store.
func buildZoomRanges(reader *embedbin.Reader) ([]uint16, error) {
	ranges := make([]uint16, zoomCellsLng*zoomCellsLat)
	for i := range ranges {
		ranges[i] = noTiles
	}
	var spans [32]*zoomSpans
	err := reader.FuzzyKeys(func(t geom.TileID) {
		x, y, z := t.XYZ()
		s := spans[z]
		if s == nil {
			s = newZoomSpans(z)
			spans[z] = s
		}
		cx, cy := s.col[x], s.row[y]
		for r := int(cy[0]); r <= int(cy[1]); r++ {
			row := ranges[r*zoomCellsLng+int(cx[0]) : r*zoomCellsLng+int(cx[1])+1]
			for c, v := range row {
				if v == noTiles {
					row[c] = uint16(z) | uint16(z)<<8
				} else {
					row[c] = v&0xff | uint16(z)<<8
				}
			}
		}
	})
	return ranges, err
}

// zoomSpans holds, per tile column (lng) and per tile row (lat) of one zoom,
// the first and last 1° cell index the column / row touches. A zoom has 2^z
// columns and rows but can hold up to 4^z tiles, so tiles share these.
type zoomSpans struct {
	col [][2]uint16
	row [][2]uint16
}

func newZoomSpans(z uint8) *zoomSpans {
	// Widen each tile by a hair so float error in the projection can only
	// mark extra cells, never miss one.
	const margin = 1e-7
	// Every shifted value is > -1 (the margin is the only way below 0), so
	// int() truncation equals floor once clamped at 0.
	cell := func(v float64, limit int) uint16 {
		return uint16(min(max(int(v), 0), limit-1))
	}
	rows := 1 << z
	n := float64(rows)
	// Row edge latitudes: the inverse of the Web-Mercator row in geom.tileXY.
	edge := func(y int) float64 {
		return math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi
	}
	s := &zoomSpans{col: make([][2]uint16, rows), row: make([][2]uint16, rows)}
	for x := range rows {
		lng0 := float64(x)/n*360 - 180 - margin
		lng1 := float64(x+1)/n*360 - 180 + margin
		s.col[x] = [2]uint16{cell(lng0+180, zoomCellsLng), cell(lng1+180, zoomCellsLng)}
	}
	for y := range rows {
		// geom.tileXY clamps latitudes beyond ±85.0511° into the first / last
		// row, so those rows reach the poles.
		latTop := 90.0
		if y != 0 {
			latTop = edge(y) + margin
		}
		latBottom := -90.0
		if y+1 < rows {
			latBottom = edge(y+1) - margin
		}
		s.row[y] = [2]uint16{cell(latBottom+90, zoomCellsLat), cell(latTop+90, zoomCellsLat)}
	}
	return s
}

// newFuzzyIndexFromReader rebuilds the preindex hash maps from a file's FUZZY
// section: one pass over the sorted tile keys (~2.4 MB heap on the bundled
// preindex), plus a second pass for the ~130 KB zoomRanges table. The source
// bytes are not retained. It fails when the file has no FUZZY section.
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
	zoomRanges, err := buildZoomRanges(reader)
	if err != nil {
		return nil, err
	}
	return &fuzzyIndex{
		idxZoom:    idxZoom,
		aggZoom:    aggZoom,
		single:     single,
		multi:      multi,
		names:      names,
		zoomRanges: zoomRanges,
	}, nil
}

// getTimezoneName returns the tile answer, or "" when no tile covers the
// point (including ambiguity-free misses); the caller falls back to polygons.
func (f *fuzzyIndex) getTimezoneName(lng float64, lat float64) string {
	// NaN fails every comparison, so it is rejected here too.
	if !(lng >= -180 && lng <= 180 && lat >= -90 && lat <= 90) {
		return ""
	}
	r := f.zoomRanges[zoomCell(lng, lat)]
	if r == noTiles {
		return ""
	}
	lo := max(int(r&0xff), f.aggZoom)
	hi := min(int(r>>8), f.idxZoom)
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := lo; z <= hi; z++ {
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

// preindexFeature builds the preindex-tile Feature for tzName, or nil when
// the dataset does not contain the name or no tile names it. The same name
// may map to more than one directory item, so all matching indices count.
func (f *fuzzyIndex) preindexFeature(tzName string) *convert.FeatureItem {
	var targets []uint16
	for i, name := range f.names {
		if name == tzName {
			targets = append(targets, uint16(i))
		}
	}
	if len(targets) == 0 {
		return nil
	}
	tiles := convert.PreindexTilesFor(f.single, f.multi, targets)
	if len(tiles) == 0 {
		return nil
	}
	return convert.RevertItemFromTiles(tzName, tiles)
}

// preindexFeatures builds one Feature per timezone owning at least one
// preindex tile, in dataset order.
func (f *fuzzyIndex) preindexFeatures() []*convert.FeatureItem {
	grouped := convert.PreindexTilesGrouped(f.single, f.multi)
	features := make([]*convert.FeatureItem, 0, len(grouped))
	for i, name := range f.names {
		tiles, ok := grouped[uint16(i)]
		if !ok {
			continue
		}
		features = append(features, convert.RevertItemFromTiles(name, tiles))
	}
	return features
}
