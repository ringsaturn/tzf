package embedbin

import (
	"fmt"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
)

// VerifyM performs host-side semantic validation of an M-profile file against
// the source topology, the M twin of Verify. The expected ring content is
// derived from the source independently of the encoder: each ring's segments
// are flattened, adjacent duplicates removed, and the closing duplicate
// dropped — exactly the §5.1 expansion the M builder must have applied.
func VerifyM(input *pb.CompressedTopoTimezones, r *Reader) error {
	if input == nil || r == nil {
		return fmt.Errorf("verify: %w: nil input", ErrMalformed)
	}
	if r.profile != profileM {
		return fmt.Errorf("verify: %w", ErrProfile)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false
	if len(input.Timezones) != int(r.tzCount) || input.Version != r.version {
		return fmt.Errorf("verify: %w: header/source mismatch", ErrMalformed)
	}

	points, err := r.flatPointsSlice()
	if err != nil {
		return err
	}
	edges := make(map[int32][]geom.I32Point, len(input.SharedEdges))
	for _, edge := range input.SharedEdges {
		if edge == nil {
			return fmt.Errorf("verify: %w: nil source edge", ErrMalformed)
		}
		pts, err := decodePolyline(edge.Points)
		if err != nil {
			return err
		}
		edges[edge.Id] = pts
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
			if poly == nil {
				return fmt.Errorf("verify: %w: source polygon", ErrMalformed)
			}
			pr, err := r.polyAt(polyIndex)
			if err != nil {
				return err
			}
			if pr.first != ringIndex || int(pr.count) != 1+len(poly.Holes) {
				return fmt.Errorf("verify: %w: polygon ring range", ErrMalformed)
			}
			ext, err := verifyFlatRing(r, ringIndex, points, poly.Exterior, edges)
			if err != nil {
				return err
			}
			if pr.box != ext.box || !tr.box.containsBBox(pr.box) {
				return fmt.Errorf("verify: %w: bbox containment chain", ErrMalformed)
			}
			ringIndex++
			for _, hole := range poly.Holes {
				if hole == nil || len(hole.Holes) != 0 {
					return fmt.Errorf("verify: %w: nested source hole", ErrMalformed)
				}
				hr, err := verifyFlatRing(r, ringIndex, points, hole.Exterior, edges)
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

// verifyFlatRing compares one stored flat ring against the source-derived
// expectation and checks its bbox is exact.
func verifyFlatRing(r *Reader, index uint32, points []geom.I32Point, source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point) (flatRingRecord, error) {
	rec, err := r.flatRingAt(index)
	if err != nil {
		return flatRingRecord{}, err
	}
	want, err := sourceOpenRing(source, edges)
	if err != nil {
		return flatRingRecord{}, err
	}
	got := points[rec.first : uint64(rec.first)+uint64(rec.count)]
	if len(got) != len(want) {
		return flatRingRecord{}, fmt.Errorf("verify: %w: ring %d point count %d != %d", ErrMalformed, index, len(got), len(want))
	}
	box := emptyBBox()
	for i, p := range got {
		if !samePoint(p, want[i]) {
			return flatRingRecord{}, fmt.Errorf("verify: %w: ring %d differs at point %d", ErrMalformed, index, i)
		}
		box.add(p)
	}
	if box != rec.box {
		return flatRingRecord{}, fmt.Errorf("verify: %w: ring %d bbox not exact", ErrMalformed, index)
	}
	return rec, nil
}

// sourceOpenRing flattens a source ring's segments and normalizes to the open
// form of spec rev 1 §5.1: adjacent duplicates removed, closing duplicate
// dropped.
func sourceOpenRing(source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point) ([]geom.I32Point, error) {
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
			pts, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return nil, err
			}
			flat = append(flat, pts...)
		case *pb.CompressedRingSegment_EdgeForward:
			pts, ok := edges[s.EdgeForward]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", ErrMalformed)
			}
			flat = append(flat, pts...)
		case *pb.CompressedRingSegment_EdgeReversed:
			pts, ok := edges[s.EdgeReversed]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", ErrMalformed)
			}
			for i := len(pts) - 1; i >= 0; i-- {
				flat = append(flat, pts[i])
			}
		default:
			return nil, fmt.Errorf("verify: %w: source segment content", ErrMalformed)
		}
	}
	flat = cleanPoints(flat)
	if len(flat) >= 2 && samePoint(flat[0], flat[len(flat)-1]) {
		flat = flat[:len(flat)-1]
	}
	if len(flat) < 3 {
		return nil, fmt.Errorf("verify: %w: source ring", ErrMalformed)
	}
	return flat, nil
}
