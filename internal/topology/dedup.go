package topology

import (
	"slices"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

// BuildTopoTimezones converts a Timezones protobuf into the topology-aware
// distribution format where shared boundary edges are stored only once in a
// global edge library. Rings are decomposed into segments that reference
// shared edges by ID instead of repeating the point sequences.
//
// This is intended for full-precision source data (≈96 MB) where ≈43% of
// boundary edges are shared between adjacent timezone polygons. Deduplication
// reduces the file size by approximately 30–35 MB while preserving full
// geometric accuracy.
func BuildTopoTimezones(input *pb.Timezones) *pb.TopoTimezones {
	if input == nil {
		return nil
	}

	// Normalize coordinates then fix winding order before topology analysis.
	// Same rationale as DoWithStats: hole rings in the source data are sometimes
	// stored CCW (exterior winding) instead of CW, causing their shared edges to
	// appear same-direction as the adjacent land polygon exterior and be skipped.
	normalized := normalizeTimezones(input)
	normalizeWindings(normalized)
	snapVertices(normalized, nil)

	// Build topology structures: shared edge detection and fixed-vertex marking.
	rings, edgeIndex, vertexIndex := collectRings(normalized)
	markSharedEdges(rings, edgeIndex)
	markFixedVerticesForDedup(rings, vertexIndex)

	// Decompose every ring into segments, building the shared edge library.
	sharedEdges := make([]*pb.SharedEdge, 0)
	edgeLibrary := make(map[sharedSegmentKey]int32)

	output := &pb.TopoTimezones{
		Version: input.Version,
	}
	for timezoneIdx, timezone := range normalized.Timezones {
		topoTz := &pb.TopoTimezone{Name: timezone.Name}
		for polygonIdx, polygon := range timezone.Polygons {
			extRef := ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: -1}
			extSegs := segmentizeRing(rings[extRef], edgeLibrary, &sharedEdges)
			topoPoly := &pb.TopoPolygon{Exterior: extSegs}
			for holeIdx := range polygon.Holes {
				holeRef := ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: holeIdx}
				holeSegs := segmentizeRing(rings[holeRef], edgeLibrary, &sharedEdges)
				topoPoly.Holes = append(topoPoly.Holes, &pb.TopoPolygon{Exterior: holeSegs})
			}
			topoTz.Polygons = append(topoTz.Polygons, topoPoly)
		}
		output.Timezones = append(output.Timezones, topoTz)
	}
	output.SharedEdges = sharedEdges
	return output
}

// DecodeTopoTimezones converts a TopoTimezones back to a flat Timezones
// protobuf by expanding all edge references into their point sequences.
func DecodeTopoTimezones(input *pb.TopoTimezones) *pb.Timezones {
	if input == nil {
		return nil
	}
	output := &pb.Timezones{
		Version: input.Version,
	}
	for _, tz := range input.Timezones {
		outTz := &pb.Timezone{Name: tz.Name}
		for _, poly := range tz.Polygons {
			outPoly := &pb.Polygon{
				Points: decodeRing(poly.Exterior, input.SharedEdges),
			}
			for _, hole := range poly.Holes {
				outPoly.Holes = append(outPoly.Holes, &pb.Polygon{
					Points: decodeRing(hole.Exterior, input.SharedEdges),
				})
			}
			outTz.Polygons = append(outTz.Polygons, outPoly)
		}
		output.Timezones = append(output.Timezones, outTz)
	}
	return output
}

// segmentizeRing splits a ring into an ordered slice of RingSegments.
// Shared boundary chains are referenced by ID in the global edge library;
// non-shared boundary sections are stored inline.
func segmentizeRing(
	ring *ringData,
	edgeLibrary map[sharedSegmentKey]int32,
	sharedEdges *[]*pb.SharedEdge,
) []*pb.RingSegment {
	if ring == nil || len(ring.Points) == 0 {
		return nil
	}
	unique := ringUniquePoints(ring.Points)
	if len(unique) < 3 {
		return []*pb.RingSegment{topoInlineSegment(ring.Points)}
	}

	fixedIndices := topoSortedFixedIndices(ring.Fixed, len(unique))

	// No fixed vertices: the ring is either entirely non-shared (coastline /
	// island) or a complete circular enclave whose entire boundary is shared
	// with exactly one partner ring. Handle both via buildSegmentRef.
	if len(fixedIndices) == 0 {
		seg := buildSegmentRef(ring.Points, ring.Edges, edgeLibrary, sharedEdges)
		return []*pb.RingSegment{seg}
	}

	// Rotate so that the first fixed vertex is the origin.
	start := fixedIndices[0]
	rotated := rotatePoints(unique, start)
	rotatedEdges := rotateEdges(ring.Edges, start)

	// Re-express fixed indices relative to the rotated origin.
	adjusted := make([]int, 0, len(fixedIndices))
	for _, idx := range fixedIndices {
		if idx < start {
			adjusted = append(adjusted, idx+len(unique)-start)
		} else {
			adjusted = append(adjusted, idx-start)
		}
	}
	slices.Sort(adjusted)
	// adjusted[0] == 0 after rotation

	segments := make([]*pb.RingSegment, 0, len(adjusted))
	for i := range adjusted {
		segStart := adjusted[i]
		var segEnd int
		if i == len(adjusted)-1 {
			segEnd = adjusted[0] + len(unique) // wrap around to first fixed vertex
		} else {
			segEnd = adjusted[i+1]
		}

		segPoints := make([]*pb.Point, 0, segEnd-segStart+1)
		for cursor := segStart; cursor <= segEnd; cursor++ {
			segPoints = append(segPoints, clonePoint(rotated[cursor%len(rotated)]))
		}
		segEdges := make([]edgeMeta, 0, segEnd-segStart)
		for cursor := segStart; cursor < segEnd; cursor++ {
			segEdges = append(segEdges, rotatedEdges[cursor%len(rotatedEdges)])
		}

		seg := buildSegmentRef(segPoints, segEdges, edgeLibrary, sharedEdges)
		segments = append(segments, seg)
	}
	return segments
}

// buildSegmentRef returns a RingSegment for the given point sequence.
// If the segment is a shared boundary chain (determined by sharedSegmentCacheKey),
// it is stored once in the edge library and subsequent occurrences return a
// reference. Non-shared segments are returned as inline points.
func buildSegmentRef(
	segPoints []*pb.Point,
	segEdges []edgeMeta,
	edgeLibrary map[sharedSegmentKey]int32,
	sharedEdges *[]*pb.SharedEdge,
) *pb.RingSegment {
	key, reversed, ok := sharedSegmentCacheKey(segPoints, segEdges)
	if !ok {
		return topoInlineSegment(segPoints)
	}

	if existingID, found := edgeLibrary[key]; found {
		if reversed {
			return &pb.RingSegment{Content: &pb.RingSegment_EdgeReversed{EdgeReversed: existingID}}
		}
		return &pb.RingSegment{Content: &pb.RingSegment_EdgeForward{EdgeForward: existingID}}
	}

	// First occurrence: add to library using the canonical direction.
	newID := int32(len(*sharedEdges))
	var storedPoints []*pb.Point
	if reversed {
		// canonical direction = reverse of input
		storedPoints = reverseOpenPath(segPoints)
	} else {
		storedPoints = clonePoints(segPoints)
	}
	*sharedEdges = append(*sharedEdges, &pb.SharedEdge{
		Id:     newID,
		Points: storedPoints,
	})
	edgeLibrary[key] = newID

	if reversed {
		return &pb.RingSegment{Content: &pb.RingSegment_EdgeReversed{EdgeReversed: newID}}
	}
	return &pb.RingSegment{Content: &pb.RingSegment_EdgeForward{EdgeForward: newID}}
}

func topoInlineSegment(points []*pb.Point) *pb.RingSegment {
	return &pb.RingSegment{
		Content: &pb.RingSegment_Inline{
			Inline: &pb.InlinePoints{Points: clonePoints(points)},
		},
	}
}

// topoSortedFixedIndices returns the fixed-vertex indices in ascending order,
// filtering out any out-of-range indices.
func topoSortedFixedIndices(fixed map[int]struct{}, n int) []int {
	indices := make([]int, 0, len(fixed))
	for idx := range fixed {
		if idx >= 0 && idx < n {
			indices = append(indices, idx)
		}
	}
	slices.Sort(indices)
	return indices
}

// decodeRing reconstructs a flat point sequence from a slice of RingSegments.
// Adjacent segments share their junction point; the function deduplicates it
// so the resulting ring has no repeated consecutive points. The ring is always
// closed (first point == last point).
func decodeRing(segments []*pb.RingSegment, sharedEdges []*pb.SharedEdge) []*pb.Point {
	if len(segments) == 0 {
		return nil
	}
	var points []*pb.Point
	for i, seg := range segments {
		segPoints := decodeSegmentPoints(seg, sharedEdges)
		if len(segPoints) == 0 {
			continue
		}
		if i == 0 {
			points = append(points, clonePoints(segPoints)...)
		} else {
			// First point of each subsequent segment equals the last point of
			// the previous segment; skip it to avoid duplication.
			points = append(points, clonePoints(segPoints[1:])...)
		}
	}
	if len(points) == 0 {
		return nil
	}
	// Ensure the ring is closed.
	if !samePoint(points[0], points[len(points)-1]) {
		points = append(points, clonePoint(points[0]))
	}
	return points
}

// decodeSegmentPoints returns the point sequence for a single RingSegment.
func decodeSegmentPoints(seg *pb.RingSegment, sharedEdges []*pb.SharedEdge) []*pb.Point {
	if seg == nil {
		return nil
	}
	switch c := seg.Content.(type) {
	case *pb.RingSegment_Inline:
		if c.Inline == nil {
			return nil
		}
		return c.Inline.Points
	case *pb.RingSegment_EdgeForward:
		id := int(c.EdgeForward)
		if id < 0 || id >= len(sharedEdges) {
			return nil
		}
		return sharedEdges[id].Points
	case *pb.RingSegment_EdgeReversed:
		id := int(c.EdgeReversed)
		if id < 0 || id >= len(sharedEdges) {
			return nil
		}
		return reverseOpenPath(sharedEdges[id].Points)
	}
	return nil
}
