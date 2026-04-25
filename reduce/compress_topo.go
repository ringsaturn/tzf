package reduce

import (
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

// CompressTopoTimezones converts a TopoTimezones into CompressedTopoTimezones
// by polyline-encoding all point sequences (shared edges and inline segments).
// Edge ID references (edge_forward / edge_reversed) are stored as-is.
func CompressTopoTimezones(input *pb.TopoTimezones) *pb.CompressedTopoTimezones {
	output := &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: input.Version,
	}

	for _, edge := range input.SharedEdges {
		output.SharedEdges = append(output.SharedEdges, &pb.CompressedSharedEdge{
			Id:     edge.Id,
			Points: CompressedPointsToPolylineBytes(edge.Points),
		})
	}

	for _, tz := range input.Timezones {
		outTz := &pb.CompressedTopoTimezone{Name: tz.Name}
		for _, poly := range tz.Polygons {
			outTz.Polygons = append(outTz.Polygons, compressTopoPolygon(poly))
		}
		output.Timezones = append(output.Timezones, outTz)
	}

	return output
}

func compressTopoPolygon(poly *pb.TopoPolygon) *pb.CompressedTopoPolygon {
	out := &pb.CompressedTopoPolygon{}
	for _, seg := range poly.Exterior {
		out.Exterior = append(out.Exterior, compressRingSegment(seg))
	}
	for _, hole := range poly.Holes {
		out.Holes = append(out.Holes, compressTopoPolygon(hole))
	}
	return out
}

func compressRingSegment(seg *pb.RingSegment) *pb.CompressedRingSegment {
	switch c := seg.Content.(type) {
	case *pb.RingSegment_Inline:
		return &pb.CompressedRingSegment{
			Content: &pb.CompressedRingSegment_Inline{
				Inline: &pb.CompressedInlinePoints{
					Points: CompressedPointsToPolylineBytes(c.Inline.Points),
				},
			},
		}
	case *pb.RingSegment_EdgeForward:
		return &pb.CompressedRingSegment{
			Content: &pb.CompressedRingSegment_EdgeForward{EdgeForward: c.EdgeForward},
		}
	case *pb.RingSegment_EdgeReversed:
		return &pb.CompressedRingSegment{
			Content: &pb.CompressedRingSegment_EdgeReversed{EdgeReversed: c.EdgeReversed},
		}
	}
	return &pb.CompressedRingSegment{}
}

// DecompressTopoTimezones converts a CompressedTopoTimezones back to TopoTimezones
// by decoding all polyline-encoded point sequences.
func DecompressTopoTimezones(input *pb.CompressedTopoTimezones) *pb.TopoTimezones {
	output := &pb.TopoTimezones{Version: input.Version}

	for _, edge := range input.SharedEdges {
		output.SharedEdges = append(output.SharedEdges, &pb.SharedEdge{
			Id:     edge.Id,
			Points: DecompressedPolylineBytesToPoints(edge.Points),
		})
	}

	for _, tz := range input.Timezones {
		outTz := &pb.TopoTimezone{Name: tz.Name}
		for _, poly := range tz.Polygons {
			outTz.Polygons = append(outTz.Polygons, decompressTopoPolygon(poly))
		}
		output.Timezones = append(output.Timezones, outTz)
	}

	return output
}

func decompressTopoPolygon(poly *pb.CompressedTopoPolygon) *pb.TopoPolygon {
	out := &pb.TopoPolygon{}
	for _, seg := range poly.Exterior {
		out.Exterior = append(out.Exterior, decompressRingSegment(seg))
	}
	for _, hole := range poly.Holes {
		out.Holes = append(out.Holes, decompressTopoPolygon(hole))
	}
	return out
}

func decompressRingSegment(seg *pb.CompressedRingSegment) *pb.RingSegment {
	switch c := seg.Content.(type) {
	case *pb.CompressedRingSegment_Inline:
		return &pb.RingSegment{
			Content: &pb.RingSegment_Inline{
				Inline: &pb.InlinePoints{
					Points: DecompressedPolylineBytesToPoints(c.Inline.Points),
				},
			},
		}
	case *pb.CompressedRingSegment_EdgeForward:
		return &pb.RingSegment{
			Content: &pb.RingSegment_EdgeForward{EdgeForward: c.EdgeForward},
		}
	case *pb.CompressedRingSegment_EdgeReversed:
		return &pb.RingSegment{
			Content: &pb.RingSegment_EdgeReversed{EdgeReversed: c.EdgeReversed},
		}
	}
	return &pb.RingSegment{}
}
