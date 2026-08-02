package embedbin

import (
	"fmt"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
)

type segmentKey struct {
	ax int32
	ay int32
	bx int32
	by int32
}

// Verify performs host-side semantic validation against the source topology.
// It scans all point streams and is intended for build pipelines.
func Verify(input *pb.CompressedTopoTimezones, r *Reader) error {
	if input == nil || r == nil {
		return fmt.Errorf("verify: %w: nil input", ErrMalformed)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false
	if len(input.Timezones) != int(r.tzCount) || input.Version != r.version {
		return fmt.Errorf("verify: %w: header/source mismatch", ErrMalformed)
	}

	groups := make([][]geom.I32Point, r.groupCount)
	for i := uint32(0); i < r.groupCount; i++ {
		g, err := r.groupAt(i)
		if err != nil {
			return err
		}
		var points []geom.I32Point
		for j := uint32(0); j < uint32(g.count); j++ {
			idx := g.first + j
			c, err := r.chunkAt(idx)
			if err != nil {
				return err
			}
			part, err := r.decodeChunkPoints(idx, c)
			if err != nil {
				return fmt.Errorf("verify group %d chunk %d: %w", i, j, err)
			}
			check := slicesWithNext(part, nil)
			if j+1 < uint32(g.count) {
				next, err := r.chunkAt(idx + 1)
				if err != nil {
					return err
				}
				first, err := r.firstChunkPoint(idx+1, next)
				if err != nil {
					return err
				}
				check = slicesWithNext(part, &first)
			}
			for _, p := range check {
				if !c.box.contains(float64(p.X), float64(p.Y)) {
					return fmt.Errorf("verify: %w: chunk bbox misses point", ErrMalformed)
				}
			}
			if !g.box.containsBBox(c.box) {
				return fmt.Errorf("verify: %w: group bbox misses chunk", ErrMalformed)
			}
			points = append(points, part...)
		}
		if uint32(len(points)) != g.pointCount || !samePoint(points[0], g.entry) ||
			!samePoint(points[len(points)-1], g.exit) {
			return fmt.Errorf("verify: %w: group endpoints or count", ErrMalformed)
		}
		for j := 1; j < len(points); j++ {
			if samePoint(points[j-1], points[j]) {
				return fmt.Errorf("verify: %w: duplicate group point", ErrMalformed)
			}
		}
		groups[i] = points
	}

	edges := make(map[int32][]geom.I32Point, len(input.SharedEdges))
	for _, edge := range input.SharedEdges {
		if edge == nil {
			return fmt.Errorf("verify: %w: nil source edge", ErrMalformed)
		}
		points, err := decodePolyline(edge.Points)
		if err != nil {
			return err
		}
		edges[edge.Id] = cleanPoints(points)
	}

	ringIndex := uint32(0)
	polyIndex := uint32(0)
	for ti, tz := range input.Timezones {
		name, err := r.nameBytesLocked(int32(ti))
		if err != nil {
			return err
		}
		if tz == nil || string(name) != tz.Name {
			return fmt.Errorf("verify: %w: timezone name", ErrMalformed)
		}
		tr, err := r.tzAt(uint32(ti))
		if err != nil {
			return err
		}
		if tr.first != polyIndex || int(tr.count) != len(tz.Polygons) {
			return fmt.Errorf("verify: %w: timezone polygon range", ErrMalformed)
		}
		for _, poly := range tz.Polygons {
			if poly == nil || len(poly.Holes) > int(^uint16(0)) {
				return fmt.Errorf("verify: %w: source polygon", ErrMalformed)
			}
			pr, err := r.polyAt(polyIndex)
			if err != nil {
				return err
			}
			if pr.first != ringIndex || int(pr.count) != 1+len(poly.Holes) {
				return fmt.Errorf("verify: %w: polygon ring range", ErrMalformed)
			}
			if err := verifyRingSegments(r, ringIndex, poly.Exterior, edges, groups); err != nil {
				return err
			}
			ext, err := r.ringAt(ringIndex)
			if err != nil {
				return err
			}
			if !pr.box.containsBBox(ext.box) || !tr.box.containsBBox(pr.box) {
				return fmt.Errorf("verify: %w: bbox containment chain", ErrMalformed)
			}
			ringIndex++
			for _, hole := range poly.Holes {
				if hole == nil || len(hole.Holes) != 0 {
					return fmt.Errorf("verify: %w: nested source hole", ErrMalformed)
				}
				if err := verifyRingSegments(r, ringIndex, hole.Exterior, edges, groups); err != nil {
					return err
				}
				hr, err := r.ringAt(ringIndex)
				if err != nil {
					return err
				}
				if !pr.box.containsBBox(hr.box) {
					return fmt.Errorf("verify: %w: exterior bbox misses hole", ErrMalformed)
				}
				ringIndex++
			}
			polyIndex++
		}
	}
	if ringIndex != r.ringCount || polyIndex != r.polyCount {
		return fmt.Errorf("verify: %w: unused directory records", ErrMalformed)
	}
	return nil
}

func (r *Reader) decodeChunkPoints(index uint32, chunk chunkRecord) ([]geom.I32Point, error) {
	start, end, err := r.chunkRange(index, chunk)
	if err != nil {
		return nil, err
	}
	cursor := streamCursor{r: r, pos: start, end: end}
	x, err := cursor.varint()
	if err != nil {
		return nil, err
	}
	y, err := cursor.varint()
	if err != nil {
		return nil, err
	}
	points := make([]geom.I32Point, 0, chunk.count)
	prev := geom.I32Point{X: x, Y: y}
	points = append(points, prev)
	for i := uint16(1); i < chunk.count; i++ {
		dx, err := cursor.varint()
		if err != nil {
			return nil, err
		}
		dy, err := cursor.varint()
		if err != nil {
			return nil, err
		}
		nx, err := addDelta(prev.X, dx)
		if err != nil {
			return nil, err
		}
		ny, err := addDelta(prev.Y, dy)
		if err != nil {
			return nil, err
		}
		prev = geom.I32Point{X: nx, Y: ny}
		if !pointInDomain(prev) {
			return nil, fmt.Errorf("%w: point domain", ErrMalformed)
		}
		points = append(points, prev)
	}
	if cursor.pos != end {
		return nil, fmt.Errorf("%w: chunk termination", ErrMalformed)
	}
	return points, nil
}

func slicesWithNext(points []geom.I32Point, next *geom.I32Point) []geom.I32Point {
	if next == nil {
		return points
	}
	out := make([]geom.I32Point, len(points)+1)
	copy(out, points)
	out[len(points)] = *next
	return out
}

func cleanPoints(points []geom.I32Point) []geom.I32Point {
	out := make([]geom.I32Point, 0, len(points))
	for _, p := range points {
		if len(out) == 0 || !samePoint(out[len(out)-1], p) {
			out = append(out, p)
		}
	}
	return out
}

func verifyRingSegments(r *Reader, index uint32, source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point, groups [][]geom.I32Point) error {
	record, err := r.ringAt(index)
	if err != nil {
		return err
	}
	got := make(map[segmentKey]int)
	var first, previous geom.I32Point
	var sum uint64
	for i := uint32(0); i < uint32(record.count); i++ {
		word, err := r.opAt(record.first + i)
		if err != nil {
			return err
		}
		points := groups[word&0x7fffffff]
		group, err := r.groupAt(word & 0x7fffffff)
		if err != nil {
			return err
		}
		if !record.box.containsBBox(group.box) {
			return fmt.Errorf("verify: %w: ring bbox misses group", ErrMalformed)
		}
		sum += uint64(len(points))
		entry, exit := points[0], points[len(points)-1]
		if word>>31 != 0 {
			entry, exit = exit, entry
		}
		if i == 0 {
			first = entry
		} else {
			if !samePoint(previous, entry) {
				return fmt.Errorf("verify: %w: disconnected ring junction", ErrMalformed)
			}
			addSegment(got, previous, entry)
		}
		previous = exit
		for j := 1; j < len(points); j++ {
			addSegment(got, points[j-1], points[j])
		}
	}
	if !samePoint(previous, first) {
		return fmt.Errorf("verify: %w: disconnected cyclic ring junction", ErrMalformed)
	}
	addSegment(got, previous, first)
	if sum-uint64(record.count) != uint64(record.pointCount) {
		return fmt.Errorf("verify: %w: ring point formula", ErrMalformed)
	}
	want, err := sourceSegments(source, edges)
	if err != nil {
		return err
	}
	if !equalSegmentMaps(got, want) {
		return fmt.Errorf("verify: %w: ring segment multiset differs at ring %d", ErrMalformed, index)
	}
	return nil
}

func sourceSegments(source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point) (map[segmentKey]int, error) {
	var flat []geom.I32Point
	for _, segment := range source {
		if segment == nil {
			return nil, fmt.Errorf("verify: %w: nil source segment", ErrMalformed)
		}
		switch s := segment.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			if s.Inline == nil {
				return nil, fmt.Errorf("verify: %w: nil inline", ErrMalformed)
			}
			points, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return nil, err
			}
			flat = append(flat, points...)
		case *pb.CompressedRingSegment_EdgeForward:
			points, ok := edges[s.EdgeForward]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", ErrMalformed)
			}
			flat = append(flat, points...)
		case *pb.CompressedRingSegment_EdgeReversed:
			points, ok := edges[s.EdgeReversed]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", ErrMalformed)
			}
			for i := len(points) - 1; i >= 0; i-- {
				flat = append(flat, points[i])
			}
		default:
			return nil, fmt.Errorf("verify: %w: source segment content", ErrMalformed)
		}
	}
	if len(flat) < 3 {
		return nil, fmt.Errorf("verify: %w: source ring", ErrMalformed)
	}
	out := make(map[segmentKey]int)
	for i := range flat {
		addSegment(out, flat[i], flat[(i+1)%len(flat)])
	}
	return out, nil
}

func addSegment(dst map[segmentKey]int, a, b geom.I32Point) {
	if samePoint(a, b) {
		return
	}
	if b.X < a.X || b.X == a.X && b.Y < a.Y {
		a, b = b, a
	}
	dst[segmentKey{ax: a.X, ay: a.Y, bx: b.X, by: b.Y}]++
}

func equalSegmentMaps(a, b map[segmentKey]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, count := range a {
		if b[key] != count {
			return false
		}
	}
	return true
}

func (r *Reader) nameBytesLocked(idx int32) ([]byte, error) {
	start, end, err := r.nameBounds(idx)
	if err != nil {
		return nil, err
	}
	out := make([]byte, int(end-start))
	if r.data != nil {
		copy(out, r.data[start:end])
		return out, nil
	}
	if err := r.readRaw(out, start); err != nil {
		return nil, err
	}
	return out, nil
}
