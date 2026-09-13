package embedbin

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// hostLittleEndian reports whether native multi-byte loads match the file's
// little-endian layout, enabling the zero-copy aliasing paths of spec rev 1
// §6.2. Explicit little-endian decoding remains the fallback everywhere.
var hostLittleEndian = binary.NativeEndian.Uint16([]byte{0x01, 0x02}) == 0x0201

// FlatView exposes an M-profile file as the exact inputs of the materialized
// int32 finder. On little-endian hosts with a byte-backed reader the ring
// slices alias the file's FLATPOINTS section without copying, so the view is
// only valid while the source bytes stay live and unmodified.
type FlatView struct {
	Version string
	Names   []string
	// Polygons is indexed by timezone, parallel to Names.
	Polygons [][]ExpandedPolygon
	// Grid is the dense GRID section, queried in place. Nil when the file
	// has no GRID section.
	Grid *DenseGrid
}

// Flat materializes an M-profile file's directory structure over its flat
// point array (spec rev 1 §6.6). Ring point data is aliased when the host is
// little-endian and the section is correctly aligned in memory, and decoded
// into a heap copy otherwise; results are identical.
func (r *Reader) Flat() (*FlatView, error) {
	if r.profile != profileM {
		return nil, ErrProfile
	}
	v := r.view()
	defer r.release(v)
	r = v

	points, err := r.FlatPointsSlice()
	if err != nil {
		return nil, err
	}
	names := make([]string, r.tzCount)
	polygons := make([][]ExpandedPolygon, r.tzCount)
	for i := uint32(0); i < r.tzCount; i++ {
		name, err := r.NameCopy(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
		t, err := r.TZAt(i)
		if err != nil {
			return nil, err
		}
		polys := make([]ExpandedPolygon, t.Count)
		for j := uint32(0); j < uint32(t.Count); j++ {
			p, err := r.PolyAt(t.First + j)
			if err != nil {
				return nil, err
			}
			rings := make([][]geom.I32Point, p.Count)
			for k := uint32(0); k < uint32(p.Count); k++ {
				rec, err := r.FlatRingAt(p.First + k)
				if err != nil {
					return nil, err
				}
				rings[k] = points[rec.First : uint64(rec.First)+uint64(rec.Count) : uint64(rec.First)+uint64(rec.Count)]
			}
			polys[j] = ExpandedPolygon{Exterior: rings[0], Holes: rings[1:]}
		}
		polygons[i] = polys
	}
	grid, err := r.denseGrid()
	if err != nil {
		return nil, err
	}
	return &FlatView{Version: r.version, Names: names, Polygons: polygons, Grid: grid}, nil
}

// sectionBytes returns one section's raw bytes: a subslice for byte-backed
// readers, a heap copy for io.ReaderAt sources.
func (r *Reader) sectionBytes(typ uint32) ([]byte, error) {
	s := r.sections[typ]
	if r.data != nil {
		return r.data[s.Off : uint64(s.Off)+uint64(s.Len)], nil
	}
	out := make([]byte, s.Len)
	if err := r.readRaw(out, uint64(s.Off)); err != nil {
		return nil, err
	}
	return out, nil
}

// flatPointsSlice returns the FLATPOINTS pairs as a point slice, aliasing the
// section bytes when the sanctioned §6.2 conditions hold.
func (r *Reader) FlatPointsSlice() ([]geom.I32Point, error) {
	b, err := r.sectionBytes(sectionFlatPoints)
	if err != nil {
		return nil, err
	}
	if pts, ok := aliasSlice[geom.I32Point](b); ok {
		return pts, nil
	}
	out := make([]geom.I32Point, r.flatPairCount)
	for i := range out {
		out[i] = geom.I32Point{
			X: int32(binary.LittleEndian.Uint32(b[i*8:])),
			Y: int32(binary.LittleEndian.Uint32(b[i*8+4:])),
		}
	}
	return out, nil
}

// aliasSlice reinterprets b as a []T without copying when the host is
// little-endian and b is aligned for T. The bool result reports success.
func aliasSlice[T geom.I32Point | uint32 | uint16](b []byte) ([]T, bool) {
	var zero T
	size := int(unsafe.Sizeof(zero))
	if len(b) == 0 {
		return nil, true
	}
	if !hostLittleEndian || uintptr(unsafe.Pointer(&b[0]))%unsafe.Alignof(zero) != 0 {
		return nil, false
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&b[0])), len(b)/size), true
}

// DenseGrid queries the dense GRID section in place (spec rev 1 §6.5),
// avoiding the materialized candidate map's heap. Content and results match
// gridindex.DecodeToMap over the source GridIndex.
type DenseGrid struct {
	lngMin, latMin     int
	lngCells, latCells int
	words              []uint32
	cands              []uint16
}

// denseGrid builds the in-place GRID accessor; nil when the section is absent.
func (r *Reader) denseGrid() (*DenseGrid, error) {
	if !r.grid.present {
		return nil, nil
	}
	b, err := r.sectionBytes(sectionGrid)
	if err != nil {
		return nil, err
	}
	g := &DenseGrid{
		lngMin:   int(r.grid.lngMin),
		latMin:   int(r.grid.latMin),
		lngCells: int(r.grid.lngCells),
		latCells: int(r.grid.latCells),
	}
	cellBytes := b[12 : 12+4*g.lngCells*g.latCells]
	candBytes := b[12+4*g.lngCells*g.latCells:]
	if words, ok := aliasSlice[uint32](cellBytes); ok {
		g.words = words
	} else {
		g.words = make([]uint32, g.lngCells*g.latCells)
		for i := range g.words {
			g.words[i] = binary.LittleEndian.Uint32(cellBytes[i*4:])
		}
	}
	if cands, ok := aliasSlice[uint16](candBytes); ok {
		g.cands = cands
	} else {
		g.cands = make([]uint16, r.grid.candCount)
		for i := range g.cands {
			g.cands[i] = binary.LittleEndian.Uint16(candBytes[i*2:])
		}
	}
	return g, nil
}

// CellRange returns the candidate range for (lng, lat): count 0 means no
// candidate covers the point. Offsets were bounds-checked at open, so
// Candidate(off+i) is safe for i < count.
func (g *DenseGrid) CellRange(lng, lat float64) (off, count uint32) {
	if math.IsNaN(lng) || math.IsNaN(lat) || math.IsInf(lng, 0) || math.IsInf(lat, 0) ||
		lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return 0, 0
	}
	cx := int(math.Floor(lng)) - g.lngMin
	cy := int(math.Floor(lat)) - g.latMin
	if cx < 0 || cy < 0 || cx >= g.lngCells || cy >= g.latCells {
		return 0, 0
	}
	word := g.words[cy*g.lngCells+cx]
	return word & 0x0fffffff, word >> 28
}

// Candidate resolves one candidate slot to its timezone index.
func (g *DenseGrid) Candidate(off uint32) int32 {
	return int32(g.cands[off])
}
