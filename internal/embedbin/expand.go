package embedbin

import (
	"encoding/binary"
	"fmt"

	"github.com/ringsaturn/tzf/internal/geom"
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false

	groups := make([][]geom.I32Point, r.groupCount)
	for i := uint32(0); i < r.groupCount; i++ {
		g, err := r.groupAt(i)
		if err != nil {
			return nil, err
		}
		// Cap the preallocation: pointCount is file-controlled, so a forged
		// header must not demand memory before decode proves the data exists.
		points := make([]geom.I32Point, 0, min(g.pointCount, 1<<16))
		for j := uint32(0); j < uint32(g.count); j++ {
			part, err := r.decodeChunkPointsAt(g.first + j)
			if err != nil {
				return nil, fmt.Errorf("expand group %d chunk %d: %w", i, j, err)
			}
			points = append(points, part...)
		}
		if uint32(len(points)) != g.pointCount ||
			!samePoint(points[0], g.entry) || !samePoint(points[len(points)-1], g.exit) {
			return nil, fmt.Errorf("expand group %d: %w: endpoints or count", i, ErrMalformed)
		}
		groups[i] = points
	}

	rings := make([][]geom.I32Point, r.ringCount)
	for i := uint32(0); i < r.ringCount; i++ {
		ring, err := r.expandRing(i, groups)
		if err != nil {
			return nil, err
		}
		rings[i] = ring
	}

	names := make([]string, r.tzCount)
	polygons := make([][]ExpandedPolygon, r.tzCount)
	for i := uint32(0); i < r.tzCount; i++ {
		name, err := r.nameBytesLocked(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
		t, err := r.tzAt(i)
		if err != nil {
			return nil, err
		}
		polys := make([]ExpandedPolygon, t.count)
		for j := uint32(0); j < uint32(t.count); j++ {
			p, err := r.polyAt(t.first + j)
			if err != nil {
				return nil, err
			}
			ep := ExpandedPolygon{Exterior: rings[p.first]}
			if p.count > 1 {
				ep.Holes = make([][]geom.I32Point, p.count-1)
				for h := uint32(1); h < uint32(p.count); h++ {
					ep.Holes[h-1] = rings[p.first+h]
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

// expandRing assembles one ring from its ops, skipping the duplicated
// junction vertex at each op boundary and the stored closing vertex.
func (r *Reader) expandRing(index uint32, groups [][]geom.I32Point) ([]geom.I32Point, error) {
	ring, err := r.ringAt(index)
	if err != nil {
		return nil, err
	}
	pts := make([]geom.I32Point, 0, min(uint64(ring.pointCount)+1, 1<<16))
	for k := uint32(0); k < uint32(ring.count); k++ {
		word, err := r.opAt(ring.first + k)
		if err != nil {
			return nil, err
		}
		g := groups[word&0x7fffffff]
		reversed := word>>31 != 0
		skip := k > 0
		if skip {
			entry := g[0]
			if reversed {
				entry = g[len(g)-1]
			}
			if !samePoint(entry, pts[len(pts)-1]) {
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
	if uint64(len(pts)) != uint64(ring.pointCount)+1 {
		return nil, fmt.Errorf("expand ring %d: %w: point count", index, ErrMalformed)
	}
	if !samePoint(pts[len(pts)-1], pts[0]) {
		return nil, fmt.Errorf("expand ring %d: %w: closing junction mismatch", index, ErrMalformed)
	}
	return pts[:len(pts)-1], nil
}

// decodeChunkPointsAt decodes one chunk's full point run.
func (r *Reader) decodeChunkPointsAt(index uint32) ([]geom.I32Point, error) {
	c, err := r.chunkAt(index)
	if err != nil {
		return nil, err
	}
	return r.decodeChunkPoints(index, c)
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
			raw, err := r.readSmall(uint64(s.off)+12+cell*4, 4)
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
