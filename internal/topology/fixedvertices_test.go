package topology

import (
	"testing"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

// markRings runs the topology preparation stages up to and including
// markFixedVertices, mirroring DoWithStatsAndBaseline so a test can inspect
// the fixed-vertex set the simplifier actually uses.
func markRings(t *testing.T, input *pb.Timezones) map[ringRef]*ringData {
	t.Helper()
	output := prepareBaseline(input, nil)
	rings, edgeIndex, vertexIndex := collectRings(output)
	markSharedEdges(rings, edgeIndex)
	markFixedVertices(rings, vertexIndex, nil)
	return rings
}

// ringSegmentEdges splits a ring into segments exactly as walkRingSegments
// does and returns the edge metadata belonging to each segment.
func ringSegmentEdges(ring *ringData) [][]edgeMeta {
	unique := ringUniquePoints(ring.Points)
	if len(unique) <= 3 {
		return nil
	}
	fixed := topoSortedFixedIndices(ring.Fixed, len(unique))
	switch len(fixed) {
	case 0, 1:
		// Zero or one fixed vertex: the whole ring is a single open path.
		return [][]edgeMeta{ring.Edges}
	}
	segments := make([][]edgeMeta, 0, len(fixed))
	for i := range fixed {
		start := fixed[i]
		end := fixed[(i+1)%len(fixed)]
		if i == len(fixed)-1 {
			end += len(unique)
		}
		segment := make([]edgeMeta, 0, end-start)
		for cursor := start; cursor < end; cursor++ {
			segment = append(segment, ring.Edges[cursor%len(ring.Edges)])
		}
		segments = append(segments, segment)
	}
	return segments
}

// TestMarkFixedVertices_SegmentsAreHomogeneous pins the invariant the shared
// segment cache depends on: after markFixedVertices, every segment produced by
// walkRingSegments is either entirely local or entirely shared with a single
// partner ring. A segment mixing the two cannot be keyed in the cache (see
// sharedSegmentCacheKey), which makes the two sides of a border simplify
// independently and open coverage gaps.
func TestMarkFixedVertices_SegmentsAreHomogeneous(t *testing.T) {
	// A wavy border used by several layouts below.
	wavy := [][2]float32{
		{0, 0}, {0.5, 0.02}, {1, -0.02}, {1.5, 0.02}, {2, 0},
	}
	reverse := func(in [][2]float32) [][2]float32 {
		out := make([][2]float32, 0, len(in))
		for i := len(in) - 1; i >= 0; i-- {
			out = append(out, in[i])
		}
		return out
	}

	tests := []struct {
		name  string
		input *pb.Timezones
	}{
		{
			// Two rings, no three-ring junction: the previous bug.
			name: "two_ring_no_junction",
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Left", Polygons: []*pb.Polygon{{Points: lineToRing(append(append([][2]float32{}, wavy...),
					[2]float32{2, -1}, [2]float32{0, -1}, [2]float32{0, 0}))}}},
				{Name: "Right", Polygons: []*pb.Polygon{{Points: lineToRing(append(append([][2]float32{},
					reverse(wavy)...), [2]float32{0, 1}, [2]float32{2, 1}, [2]float32{2, 0}))}}},
			}},
		},
		{
			// One ring sharing consecutive borders with two different partners,
			// so a shared edge meets a shared edge with a different partner.
			name: "partner_change",
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Center", Polygons: []*pb.Polygon{{Points: lineToRing([][2]float32{
					{0, 0}, {0.5, 0.02}, {1, -0.02}, {1.5, 0.02}, {2, 0},
					{2, 0.5}, {2, 1.5}, {2, 2}, {0, 2}, {0, 0},
				})}}},
				{Name: "Bottom", Polygons: []*pb.Polygon{{Points: lineToRing([][2]float32{
					{2, 0}, {1.5, 0.02}, {1, -0.02}, {0.5, 0.02}, {0, 0},
					{0, -1}, {2, -1}, {2, 0},
				})}}},
				{Name: "Right", Polygons: []*pb.Polygon{{Points: lineToRing([][2]float32{
					{2, 2}, {2, 1.5}, {2, 0.5}, {2, 0}, {3, 0}, {3, 2}, {2, 2},
				})}}},
			}},
		},
		{
			// Enclave: a hole whose shape is another timezone's exterior.
			name: "enclave",
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Outer", Polygons: []*pb.Polygon{{
					Points: lineToRing([][2]float32{{0, 0}, {3, 0}, {3, 3}, {0, 3}, {0, 0}}),
					Holes: []*pb.Polygon{{Points: lineToRing([][2]float32{
						{1, 1}, {1, 1.4}, {0.9, 1.7}, {1.1, 2}, {1, 2.8}, {2, 2.8}, {2, 1}, {1, 1},
					})}},
				}}},
				{Name: "Inner", Polygons: []*pb.Polygon{{Points: lineToRing([][2]float32{
					{1, 1}, {2, 1}, {2, 2.8}, {1, 2.8}, {1.1, 2}, {0.9, 1.7}, {1, 1.4}, {1, 1},
				})}}},
			}},
		},
		{
			// Purely local ring: every segment is local.
			name: "local_only",
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Island", Polygons: []*pb.Polygon{{Points: lineToRing([][2]float32{
					{0, 0}, {1, 0.2}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
				})}}},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rings := markRings(t, tc.input)
			segments := 0
			for ref, ring := range rings {
				for _, edgeSegment := range ringSegmentEdges(ring) {
					if len(edgeSegment) == 0 {
						continue
					}
					segments++
					partner := edgeSegment[0].PartnerRing
					shared := edgeSegment[0].Shared
					for _, e := range edgeSegment[1:] {
						if e.Shared != shared {
							t.Fatalf("%+v: segment mixes shared and local edges", ref)
						}
						if shared && e.PartnerRing != partner {
							t.Fatalf("%+v: segment mixes partners %+v and %+v", ref, partner, e.PartnerRing)
						}
					}
				}
			}
			if segments == 0 {
				t.Fatalf("no segments produced")
			}
		})
	}
}
