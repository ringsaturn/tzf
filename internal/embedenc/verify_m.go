package embedenc

import (
	"fmt"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

// VerifyM performs host-side semantic validation of an M-profile file against
// the source topology, the M twin of Verify. The expected ring content is
// derived from the source independently of the encoder: each ring's segments
// are flattened, adjacent duplicates removed, and the closing duplicate
// dropped — exactly the §5.1 expansion the M builder must have applied.
func VerifyM(input *pb.CompressedTopoTimezones, r *embedbin.Reader) error {
	if input == nil || r == nil {
		return fmt.Errorf("verify: %w: nil input", embedbin.ErrMalformed)
	}
	if r.Profile() != embedbin.ProfileM {
		return fmt.Errorf("verify: %w", embedbin.ErrProfile)
	}
	if len(input.Timezones) != int(r.TZCount()) || input.Version != r.DataVersion() {
		return fmt.Errorf("verify: %w: header/source mismatch", embedbin.ErrMalformed)
	}

	points, err := r.FlatPointsSlice()
	if err != nil {
		return err
	}
	edges := make(map[int32][]geom.I32Point, len(input.SharedEdges))
	for _, edge := range input.SharedEdges {
		if edge == nil {
			return fmt.Errorf("verify: %w: nil source edge", embedbin.ErrMalformed)
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
		name, err := r.NameCopy(int32(ti))
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
			if poly == nil {
				return fmt.Errorf("verify: %w: source polygon", embedbin.ErrMalformed)
			}
			pr, err := r.PolyAt(polyIndex)
			if err != nil {
				return err
			}
			if pr.First != ringIndex || int(pr.Count) != 1+len(poly.Holes) {
				return fmt.Errorf("verify: %w: polygon ring range", embedbin.ErrMalformed)
			}
			ext, err := verifyFlatRing(r, ringIndex, points, poly.Exterior, edges)
			if err != nil {
				return err
			}
			if pr.Box != ext.Box || !tr.Box.ContainsBBox(pr.Box) {
				return fmt.Errorf("verify: %w: embedbin.BBox containment chain", embedbin.ErrMalformed)
			}
			ringIndex++
			for _, hole := range poly.Holes {
				if hole == nil || len(hole.Holes) != 0 {
					return fmt.Errorf("verify: %w: nested source hole", embedbin.ErrMalformed)
				}
				hr, err := verifyFlatRing(r, ringIndex, points, hole.Exterior, edges)
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

// verifyFlatRing compares one stored flat ring against the source-derived
// expectation and checks its embedbin.BBox is exact.
func verifyFlatRing(r *embedbin.Reader, index uint32, points []geom.I32Point, source []*pb.CompressedRingSegment, edges map[int32][]geom.I32Point) (embedbin.FlatRingRecord, error) {
	rec, err := r.FlatRingAt(index)
	if err != nil {
		return embedbin.FlatRingRecord{}, err
	}
	want, err := sourceOpenRing(source, edges)
	if err != nil {
		return embedbin.FlatRingRecord{}, err
	}
	got := points[rec.First : uint64(rec.First)+uint64(rec.Count)]
	if len(got) != len(want) {
		return embedbin.FlatRingRecord{}, fmt.Errorf("verify: %w: ring %d point count %d != %d", embedbin.ErrMalformed, index, len(got), len(want))
	}
	box := embedbin.EmptyBBox()
	for i, p := range got {
		if !embedbin.SamePoint(p, want[i]) {
			return embedbin.FlatRingRecord{}, fmt.Errorf("verify: %w: ring %d differs at point %d", embedbin.ErrMalformed, index, i)
		}
		box.Add(p)
	}
	if box != rec.Box {
		return embedbin.FlatRingRecord{}, fmt.Errorf("verify: %w: ring %d embedbin.BBox not exact", embedbin.ErrMalformed, index)
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
			return nil, fmt.Errorf("verify: %w: nil source segment", embedbin.ErrMalformed)
		}
		switch s := segment.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			if s.Inline == nil {
				return nil, fmt.Errorf("verify: %w: nil inline", embedbin.ErrMalformed)
			}
			pts, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return nil, err
			}
			flat = append(flat, pts...)
		case *pb.CompressedRingSegment_EdgeForward:
			pts, ok := edges[s.EdgeForward]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", embedbin.ErrMalformed)
			}
			flat = append(flat, pts...)
		case *pb.CompressedRingSegment_EdgeReversed:
			pts, ok := edges[s.EdgeReversed]
			if !ok {
				return nil, fmt.Errorf("verify: %w: edge reference", embedbin.ErrMalformed)
			}
			for i := len(pts) - 1; i >= 0; i-- {
				flat = append(flat, pts[i])
			}
		default:
			return nil, fmt.Errorf("verify: %w: source segment content", embedbin.ErrMalformed)
		}
	}
	flat = cleanPoints(flat)
	if len(flat) >= 2 && embedbin.SamePoint(flat[0], flat[len(flat)-1]) {
		flat = flat[:len(flat)-1]
	}
	if len(flat) < 3 {
		return nil, fmt.Errorf("verify: %w: source ring", embedbin.ErrMalformed)
	}
	return flat, nil
}
