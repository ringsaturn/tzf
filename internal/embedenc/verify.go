package embedenc

import (
	"fmt"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

type segmentKey struct {
	ax int32
	ay int32
	bx int32
	by int32
}

// Verify performs host-side semantic validation against the source topology.
// It scans all point streams and is intended for build pipelines.
func Verify(input *pb.CompressedTopoTimezones, r *embedbin.Reader) error {
	if input == nil || r == nil {
		return fmt.Errorf("verify: %w: nil input", embedbin.ErrMalformed)
	}
	r.LockDecode()
	defer r.UnlockDecode()
	if len(input.Timezones) != int(r.TZCount()) || input.Version != r.DataVersion() {
		return fmt.Errorf("verify: %w: header/source mismatch", embedbin.ErrMalformed)
	}

	groups := make([][]geom.I32Point, r.GroupCount())
	for i := uint32(0); i < r.GroupCount(); i++ {
		g, err := r.GroupAt(i)
		if err != nil {
			return err
		}
		var points []geom.I32Point
		for j := uint32(0); j < uint32(g.Count); j++ {
			idx := g.First + j
			c, err := r.ChunkAt(idx)
			if err != nil {
				return err
			}
			part, err := r.DecodeChunkPoints(idx, c)
			if err != nil {
				return fmt.Errorf("verify group %d chunk %d: %w", i, j, err)
			}
			check := slicesWithNext(part, nil)
			if j+1 < uint32(g.Count) {
				next, err := r.ChunkAt(idx + 1)
				if err != nil {
					return err
				}
				first, err := r.FirstChunkPoint(idx+1, next)
				if err != nil {
					return err
				}
				check = slicesWithNext(part, &first)
			}
			for _, p := range check {
				if !c.Box.Contains(float64(p.X), float64(p.Y)) {
					return fmt.Errorf("verify: %w: chunk embedbin.BBox misses point", embedbin.ErrMalformed)
				}
			}
			if !g.Box.ContainsBBox(c.Box) {
				return fmt.Errorf("verify: %w: group embedbin.BBox misses chunk", embedbin.ErrMalformed)
			}
			points = append(points, part...)
		}
		if uint32(len(points)) != g.PointCount || !embedbin.SamePoint(points[0], g.Entry) ||
			!embedbin.SamePoint(points[len(points)-1], g.Exit) {
			return fmt.Errorf("verify: %w: group endpoints or count", embedbin.ErrMalformed)
		}
		for j := 1; j < len(points); j++ {
			if embedbin.SamePoint(points[j-1], points[j]) {
				return fmt.Errorf("verify: %w: duplicate group point", embedbin.ErrMalformed)
			}
		}
		groups[i] = points
	}

	edges := make(map[int32][]geom.I32Point, len(input.SharedEdges))
	for _, edge := range input.SharedEdges {
		if edge == nil {
			return fmt.Errorf("verify: %w: nil source edge", embedbin.ErrMalformed)
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
		name, err := r.NameBytesLocked(int32(ti))
		if err != nil {
			return err
		}
		if tz == nil || string(name) != tz.Name {
			return fmt.Errorf("verify: %w: timezone name", embedbin.ErrMalformed)
		}
		tr, err := r.TZAt(uint32(ti))
		if err != nil {
			return err
		}
		if tr.First != polyIndex || int(tr.Count) != len(tz.Polygons) {
			return fmt.Errorf("verify: %w: timezone polygon range", embedbin.ErrMalformed)
		}
		for _, poly := range tz.Polygons {
			if poly == nil || len(poly.Holes) > int(^uint16(0)) {
				return fmt.Errorf("verify: %w: source polygon", embedbin.ErrMalformed)
			}
			pr, err := r.PolyAt(polyIndex)
			if err != nil {
				return err
			}
			if pr.First != ringIndex || int(pr.Count) != 1+len(poly.Holes) {
				return fmt.Errorf("verify: %w: polygon ring range", embedbin.ErrMalformed)
			}
			if err := verifyRingSegments(r, ringIndex, poly.Exterior, edges, groups); err != nil {
				return err
			}
			ext, err := r.RingAt(ringIndex)
			if err != nil {
				return err
			}
			if !pr.Box.ContainsBBox(ext.Box) || !tr.Box.ContainsBBox(pr.Box) {
				return fmt.Errorf("verify: %w: embedbin.BBox containment chain", embedbin.ErrMalformed)
			}
			ringIndex++
			for _, hole := range poly.Holes {
				if hole == nil || len(hole.Holes) != 0 {
					return fmt.Errorf("verify: %w: nested source hole", embedbin.ErrMalformed)
				}
				if err := verifyRingSegments(r, ringIndex, hole.Exterior, edges, groups); err != nil {
					return err
				}
				hr, err := r.RingAt(ringIndex)
				if err != nil {
					return err
				}
				if !pr.Box.ContainsBBox(hr.Box) {
					return fmt.Errorf("verify: %w: exterior embedbin.BBox misses hole", embedbin.ErrMalformed)
				}
				ringIndex++
			}
			polyIndex++
		}
	}
	if ringIndex != r.RingCount() || polyIndex != r.PolyCount() {
		return fmt.Errorf("verify: %w: unused directory records", embedbin.ErrMalformed)
	}
	return nil
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
		if len(out) == 0 || !embedbin.SamePoint(out[len(out)-1], p) {
			out = append(out, p)
		}
	}
	return out
}

func verifyRingSegments(r *embedbin.Reader, index uint32, source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point, groups [][]geom.I32Point) error {
	record, err := r.RingAt(index)
	if err != nil {
		return err
	}
	got := make(map[segmentKey]int)
	var first, previous geom.I32Point
	var sum uint64
	for i := uint32(0); i < uint32(record.Count); i++ {
		word, err := r.OpAt(record.First + i)
		if err != nil {
			return err
		}
		points := groups[word&0x7fffffff]
		group, err := r.GroupAt(word & 0x7fffffff)
		if err != nil {
			return err
		}
		if !record.Box.ContainsBBox(group.Box) {
			return fmt.Errorf("verify: %w: ring embedbin.BBox misses group", embedbin.ErrMalformed)
		}
		sum += uint64(len(points))
		entry, exit := points[0], points[len(points)-1]
		if word>>31 != 0 {
			entry, exit = exit, entry
		}
		if i == 0 {
			first = entry
		} else {
			if !embedbin.SamePoint(previous, entry) {
				return fmt.Errorf("verify: %w: disconnected ring junction", embedbin.ErrMalformed)
			}
			addSegment(got, previous, entry)
		}
		previous = exit
		for j := 1; j < len(points); j++ {
			addSegment(got, points[j-1], points[j])
		}
	}
	if !embedbin.SamePoint(previous, first) {
		return fmt.Errorf("verify: %w: disconnected cyclic ring junction", embedbin.ErrMalformed)
	}
	addSegment(got, previous, first)
	if sum-uint64(record.Count) != uint64(record.PointCount) {
		return fmt.Errorf("verify: %w: ring point formula", embedbin.ErrMalformed)
	}
	want, err := sourceSegments(source, edges)
	if err != nil {
		return err
	}
	if !equalSegmentMaps(got, want) {
		return fmt.Errorf("verify: %w: ring segment multiset differs at ring %d", embedbin.ErrMalformed, index)
	}
	return nil
}

func sourceSegments(source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point) (map[segmentKey]int, error) {
	var flat []geom.I32Point
	for _, segment := range source {
		if segment == nil {
			return nil, fmt.Errorf("verify: %w: nil source segment", embedbin.ErrMalformed)
		}
		switch s := segment.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			if s.Inline == nil {
				return nil, fmt.Errorf("verify: %w: nil inline", embedbin.ErrMalformed)
			}
			points, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return nil, err
			}
			flat = append(flat, points...)
		case *pb.CompressedRingSegment_EdgeForward:
			points, ok := edges[s.EdgeForward]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", embedbin.ErrMalformed)
			}
			flat = append(flat, points...)
		case *pb.CompressedRingSegment_EdgeReversed:
			points, ok := edges[s.EdgeReversed]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", embedbin.ErrMalformed)
			}
			for i := len(points) - 1; i >= 0; i-- {
				flat = append(flat, points[i])
			}
		default:
			return nil, fmt.Errorf("verify: %w: source segment content", embedbin.ErrMalformed)
		}
	}
	if len(flat) < 3 {
		return nil, fmt.Errorf("verify: %w: source ring", embedbin.ErrMalformed)
	}
	out := make(map[segmentKey]int)
	for i := range flat {
		addSegment(out, flat[i], flat[(i+1)%len(flat)])
	}
	return out, nil
}

func addSegment(dst map[segmentKey]int, a, b geom.I32Point) {
	if embedbin.SamePoint(a, b) {
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
