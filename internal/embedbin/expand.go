package embedbin

import (
	"encoding/binary"
	"fmt"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// ExpandedPolygon is one polygon's rings in open form (no closing vertex),
// ready for geom.NewI32Polygon.
type ExpandedPolygon struct {
	Exterior []geom.I32Point
	Holes    [][]geom.I32Point
}

// Expanded holds the decode-once outputs of an E-profile file: the exact
// inputs of the materialized int32 finder.
type Expanded struct {
	Version string
	Names   []string
	// Polygons is indexed by timezone, parallel to Names.
	Polygons [][]ExpandedPolygon
	// Grid maps (floor(lng), floor(lat)) → candidate timezone indices, with
	// the same content as gridindex.DecodeToMap over the source GridIndex.
	// Nil when the file has no GRID section.
	Grid map[[2]int16][]int32
}

// Expand decodes the file's geometry into open per-ring point slices in one
// sequential pass (spec rev 1 §5.1). Stored junction duplicates are removed:
// each op after the first drops its first streamed point, and the ring's
// final closing point is dropped, so ring length equals RINGDIR.point_count.
// The removed vertices are zero-length PIP no-ops, so queries over the result
// match the in-place reader; exported vertex lists simply omit duplicates.
func (r *Reader) Expand() (*Expanded, error) {
	if r.profile != profileE {
		return nil, ErrProfile
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false

	groups := make([][]geom.I32Point, r.groupCount)
	for i := uint32(0); i < r.groupCount; i++ {
		points, err := r.decodeGroupAt(i)
		if err != nil {
			return nil, err
		}
		groups[i] = points
	}
	group := func(i uint32) ([]geom.I32Point, error) { return groups[i], nil }

	rings := make([][]geom.I32Point, r.ringCount)
	for i := uint32(0); i < r.ringCount; i++ {
		ring, err := r.expandRing(i, group)
		if err != nil {
			return nil, err
		}
		rings[i] = ring
	}

	names := make([]string, r.tzCount)
	polygons := make([][]ExpandedPolygon, r.tzCount)
	for i := uint32(0); i < r.tzCount; i++ {
		name, err := r.NameBytesLocked(int32(i))
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
			ep := ExpandedPolygon{Exterior: rings[p.First]}
			if p.Count > 1 {
				ep.Holes = make([][]geom.I32Point, p.Count-1)
				for h := uint32(1); h < uint32(p.Count); h++ {
					ep.Holes[h-1] = rings[p.First+h]
				}
			}
			polys[j] = ep
		}
		polygons[i] = polys
	}

	grid, err := r.gridToMap()
	if err != nil {
		return nil, err
	}
	return &Expanded{Version: r.version, Names: names, Polygons: polygons, Grid: grid}, nil
}

// groupSource resolves one shared-edge group's decoded points. Expand hands
// it a pre-decoded slice; ExpandTimezone decodes on demand.
type groupSource func(index uint32) ([]geom.I32Point, error)

// decodeGroupAt decodes one GROUPDIR entry's chunks into its point run and
// checks the run against the record's stored endpoints and count.
func (r *Reader) decodeGroupAt(index uint32) ([]geom.I32Point, error) {
	g, err := r.GroupAt(index)
	if err != nil {
		return nil, err
	}
	// Cap the preallocation: pointCount is file-controlled, so a forged
	// header must not demand memory before decode proves the data exists.
	points := make([]geom.I32Point, 0, min(g.PointCount, 1<<16))
	for j := uint32(0); j < uint32(g.Count); j++ {
		part, err := r.decodeChunkPointsAt(g.First + j)
		if err != nil {
			return nil, fmt.Errorf("expand group %d chunk %d: %w", index, j, err)
		}
		points = append(points, part...)
	}
	if uint32(len(points)) != g.PointCount ||
		!SamePoint(points[0], g.Entry) || !SamePoint(points[len(points)-1], g.Exit) {
		return nil, fmt.Errorf("expand group %d: %w: endpoints or count", index, ErrMalformed)
	}
	return points, nil
}

// expandRing assembles one ring from its ops, skipping the duplicated
// junction vertex at each op boundary and the stored closing vertex.
func (r *Reader) expandRing(index uint32, group groupSource) ([]geom.I32Point, error) {
	ring, err := r.RingAt(index)
	if err != nil {
		return nil, err
	}
	pts := make([]geom.I32Point, 0, min(uint64(ring.PointCount)+1, 1<<16))
	for k := uint32(0); k < uint32(ring.Count); k++ {
		word, err := r.OpAt(ring.First + k)
		if err != nil {
			return nil, err
		}
		g, err := group(word & 0x7fffffff)
		if err != nil {
			return nil, err
		}
		reversed := word>>31 != 0
		skip := k > 0
		if skip {
			entry := g[0]
			if reversed {
				entry = g[len(g)-1]
			}
			if !SamePoint(entry, pts[len(pts)-1]) {
				return nil, fmt.Errorf("expand ring %d: %w: junction mismatch", index, ErrMalformed)
			}
		}
		if reversed {
			for i := len(g) - 1; i >= 0; i-- {
				if skip && i == len(g)-1 {
					continue
				}
				pts = append(pts, g[i])
			}
		} else {
			if skip {
				pts = append(pts, g[1:]...)
			} else {
				pts = append(pts, g...)
			}
		}
	}
	if uint64(len(pts)) != uint64(ring.PointCount)+1 {
		return nil, fmt.Errorf("expand ring %d: %w: point count", index, ErrMalformed)
	}
	if !SamePoint(pts[len(pts)-1], pts[0]) {
		return nil, fmt.Errorf("expand ring %d: %w: closing junction mismatch", index, ErrMalformed)
	}
	return pts[:len(pts)-1], nil
}

// decodeChunkPointsAt decodes one chunk's full point run.
func (r *Reader) decodeChunkPointsAt(index uint32) ([]geom.I32Point, error) {
	c, err := r.ChunkAt(index)
	if err != nil {
		return nil, err
	}
	return r.DecodeChunkPoints(index, c)
}

// ExpandTimezone decodes one timezone's polygons, with the same per-ring
// result Expand produces for that timezone. Only the shared-edge groups its
// rings reference are decoded — each at most once — so exporting a single
// timezone costs a fraction of a full Expand. Callers that need every
// timezone should use Expand, which decodes each group exactly once overall.
func (r *Reader) ExpandTimezone(index int32) ([]ExpandedPolygon, error) {
	if r.profile != profileE {
		return nil, ErrProfile
	}
	if index < 0 || uint32(index) >= r.tzCount {
		return nil, ErrIndex
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false

	decoded := make(map[uint32][]geom.I32Point)
	group := func(i uint32) ([]geom.I32Point, error) {
		if points, ok := decoded[i]; ok {
			return points, nil
		}
		points, err := r.decodeGroupAt(i)
		if err != nil {
			return nil, err
		}
		decoded[i] = points
		return points, nil
	}

	t, err := r.TZAt(uint32(index))
	if err != nil {
		return nil, err
	}
	polys := make([]ExpandedPolygon, t.Count)
	for j := uint32(0); j < uint32(t.Count); j++ {
		p, err := r.PolyAt(t.First + j)
		if err != nil {
			return nil, err
		}
		exterior, err := r.expandRing(p.First, group)
		if err != nil {
			return nil, err
		}
		ep := ExpandedPolygon{Exterior: exterior}
		if p.Count > 1 {
			ep.Holes = make([][]geom.I32Point, p.Count-1)
			for h := uint32(1); h < uint32(p.Count); h++ {
				hole, err := r.expandRing(p.First+h, group)
				if err != nil {
					return nil, err
				}
				ep.Holes[h-1] = hole
			}
		}
		polys[j] = ep
	}
	return polys, nil
}

// gridToMap materializes the dense GRID section into the candidate map used
// by the in-RAM finder, with the same result as gridindex.DecodeToMap over
// the source GridIndex (empty cells stay absent).
func (r *Reader) gridToMap() (map[[2]int16][]int32, error) {
	if !r.grid.present {
		return nil, nil
	}
	s := r.sections[sectionGrid]
	m := make(map[[2]int16][]int32, r.grid.cellCount)
	for cy := 0; cy < int(r.grid.latCells); cy++ {
		for cx := 0; cx < int(r.grid.lngCells); cx++ {
			cell := uint64(cy)*uint64(r.grid.lngCells) + uint64(cx)
			raw, err := r.readSmall(uint64(s.Off)+12+cell*4, 4)
			if err != nil {
				return nil, err
			}
			word := binary.LittleEndian.Uint32(raw)
			count, off := word>>28, word&0x0fffffff
			if count == 0 {
				continue
			}
			indices := make([]int32, count)
			for j := uint32(0); j < count; j++ {
				raw, err := r.readSmall(r.grid.candidates+uint64(off+j)*2, 2)
				if err != nil {
					return nil, err
				}
				indices[j] = int32(binary.LittleEndian.Uint16(raw))
			}
			m[[2]int16{int16(int(r.grid.lngMin) + cx), int16(int(r.grid.latMin) + cy)}] = indices
		}
	}
	return m, nil
}
