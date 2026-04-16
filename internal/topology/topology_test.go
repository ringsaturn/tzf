package topology

import (
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

func TestDo_SharedBorderStaysIdentical(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 0.2}, {1.1, 0.4}, {0.9, 0.6}, {1, 0.8}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 0.8}, {0.9, 0.6}, {1.1, 0.4}, {1, 0.2}, {1, 0},
					})},
				},
			},
		},
	}

	output := Do(input, 0.15)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	left := output.Timezones[0].Polygons[0].Points
	right := output.Timezones[1].Polygons[0].Points

	leftShared := extractSubPath(left, [][2]float32{{1, 0}, {1, 1}})
	rightShared := extractSubPath(right, [][2]float32{{1, 1}, {1, 0}})

	if len(leftShared) == 0 || len(rightShared) == 0 {
		t.Fatalf("shared border lost after simplification")
	}
	if len(leftShared) != len(rightShared) {
		t.Fatalf("shared border mismatch: left=%d right=%d", len(leftShared), len(rightShared))
	}
	for idx := range leftShared {
		ridx := len(rightShared) - 1 - idx
		if !samePoint(leftShared[idx], rightShared[ridx]) {
			t.Fatalf("shared border diverged at %d: left=%+v right=%+v", idx, leftShared[idx], rightShared[ridx])
		}
	}
}

func TestDo_SharedHoleBorderStaysIdentical(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Outer",
				Polygons: []*pb.Polygon{
					{
						Points: lineToRing([][2]float32{
							{0, 0}, {3, 0}, {3, 3}, {0, 3}, {0, 0},
						}),
						Holes: []*pb.Polygon{
							{Points: lineToRing([][2]float32{
								{1, 1}, {1, 1.4}, {0.9, 1.7}, {1.1, 2}, {1, 2.8}, {2, 2.8}, {2, 1}, {1, 1},
							})},
						},
					},
				},
			},
			{
				Name: "Inner",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 1}, {2, 1}, {2, 2.8}, {1, 2.8}, {1.1, 2}, {0.9, 1.7}, {1, 1.4}, {1, 1},
					})},
				},
			},
		},
	}

	output := Do(input, 0.15)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	hole := output.Timezones[0].Polygons[0].Holes[0].Points
	inner := output.Timezones[1].Polygons[0].Points

	if len(hole) != len(inner) {
		t.Fatalf("hole border mismatch: hole=%d inner=%d", len(hole), len(inner))
	}
	for idx := range hole {
		ridx := len(inner) - 1 - idx
		if !samePoint(hole[idx], inner[ridx]) {
			t.Fatalf("hole border diverged at %d: hole=%+v inner=%+v", idx, hole[idx], inner[ridx])
		}
	}
}

func TestDo_SnapsSharedBorderVertices(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Sparse",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
					})},
				},
			},
			{
				Name: "Dense",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{2, 0}, {3, 0}, {3, 2}, {2, 2}, {2, 1.5}, {2, 1}, {2, 0.5}, {2, 0},
					})},
				},
			},
		},
	}

	output := Do(input, 0.1)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	sparse := output.Timezones[0].Polygons[0].Points
	dense := output.Timezones[1].Polygons[0].Points

	sparseShared := extractSubPath(sparse, [][2]float32{{2, 0}, {2, 2}})
	denseShared := extractSubPath(dense, [][2]float32{{2, 2}, {2, 0}})

	if len(sparseShared) != len(denseShared) {
		t.Fatalf("snapped shared border mismatch: sparse=%d dense=%d", len(sparseShared), len(denseShared))
	}
	for idx := range sparseShared {
		ridx := len(denseShared) - 1 - idx
		if !samePoint(sparseShared[idx], denseShared[ridx]) {
			t.Fatalf("snapped shared border diverged at %d: sparse=%+v dense=%+v", idx, sparseShared[idx], denseShared[ridx])
		}
	}
}

func TestDo_SnapsNearlyCollinearSharedVertex(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Sparse",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
					})},
				},
			},
			{
				Name: "Dense",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{2, 0}, {3, 0}, {3, 2}, {2, 2}, {2.0000005, 1}, {2, 0},
					})},
				},
			},
		},
	}

	output := Do(input, 0.1)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	sparse := output.Timezones[0].Polygons[0].Points
	dense := output.Timezones[1].Polygons[0].Points

	sparseShared := extractSubPath(sparse, [][2]float32{{2, 0}, {2, 2}})
	denseShared := extractSubPath(dense, [][2]float32{{2, 2}, {2, 0}})

	if len(sparseShared) != len(denseShared) {
		t.Fatalf("near-collinear shared border mismatch: sparse=%d dense=%d", len(sparseShared), len(denseShared))
	}
	for idx := range sparseShared {
		ridx := len(denseShared) - 1 - idx
		if !samePoint(sparseShared[idx], denseShared[ridx]) {
			t.Fatalf("near-collinear shared border diverged at %d: sparse=%+v dense=%+v", idx, sparseShared[idx], denseShared[ridx])
		}
	}
}

func TestSharedSegmentCacheKey_MatchesReverseDirection(t *testing.T) {
	segment := lineToRing([][2]float32{
		{1, 0}, {1, 0.5}, {1, 1},
	})
	edges := []edgeMeta{
		{Shared: true, PartnerRing: ringRef{TimezoneIdx: 1, PolygonIdx: 0, HoleIdx: -1}},
		{Shared: true, PartnerRing: ringRef{TimezoneIdx: 1, PolygonIdx: 0, HoleIdx: -1}},
	}

	keyA, reversedA, okA := sharedSegmentCacheKey(segment, edges)
	keyB, reversedB, okB := sharedSegmentCacheKey(reverseOpenPath(segment), edges)

	if !okA || !okB {
		t.Fatalf("expected shared segment cache keys to be generated")
	}
	if keyA != keyB {
		t.Fatalf("expected identical cache key for reversed segment: %v vs %v", keyA, keyB)
	}
	if reversedA == reversedB {
		t.Fatalf("expected reverse flag to differ for opposite directions")
	}
}

func TestDo_AntimeridianRingRemainsValid(t *testing.T) {
	// A ring whose western boundary sits exactly on the antimeridian (-180°).
	// The topology pass must NOT mix -180 and +180 coordinates within the same
	// ring, otherwise area / winding computations go wrong and the polygon
	// appears to span the entire globe.
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "AcrossLine",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{-180, 0}, {-179, 0}, {-179, 1}, {-180, 1}, {-180, 0.7}, {-180, 0.4}, {-180, 0},
					})},
				},
			},
		},
	}

	output := Do(input, 0.15)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	points := output.Timezones[0].Polygons[0].Points
	if len(points) < 4 {
		t.Fatalf("antimeridian ring collapsed: %d points", len(points))
	}
	// All longitude coordinates must be on the same side of the antimeridian
	// as the original input (negative). No mixing of -180 and +180.
	for _, p := range points {
		if p.Lng > 0 {
			t.Fatalf("antimeridian ring has unexpected positive longitude %+v", p)
		}
	}
}

func TestDo_PreservesTinyPolygonUnderLargeEpsilon(t *testing.T) {
	output := Do(&pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Tiny",
				Polygons: []*pb.Polygon{
					{Points: subdividedSquareRing(10, 10, 0.0001, 30)},
				},
			},
		},
	}, 1)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	points := output.Timezones[0].Polygons[0].Points
	if len(points) < 4 {
		t.Fatalf("tiny polygon collapsed to invalid ring: %d points", len(points))
	}
}

func TestDoWithStats_ReportsSegmentBuckets(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 0.2}, {1.1, 0.4}, {0.9, 0.6}, {1, 0.8}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 0.8}, {0.9, 0.6}, {1.1, 0.4}, {1, 0.2}, {1, 0},
					})},
				},
			},
		},
	}

	output, stats := DoWithStats(input, 0.15)
	if err := Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if stats.InputRings != 2 {
		t.Fatalf("unexpected input rings: %d", stats.InputRings)
	}
	if stats.Segments == 0 {
		t.Fatalf("expected segment stats to be populated")
	}
	if stats.SegmentInputPoints < stats.SegmentOutputPoints {
		t.Fatalf("unexpected segment point growth: input=%d output=%d", stats.SegmentInputPoints, stats.SegmentOutputPoints)
	}
	if stats.SegmentPointsLE10 == 0 {
		t.Fatalf("expected short segment bucket to be populated")
	}
}

func lineToRing(coords [][2]float32) []*pb.Point {
	points := make([]*pb.Point, 0, len(coords))
	for _, coord := range coords {
		points = append(points, &pb.Point{Lng: coord[0], Lat: coord[1]})
	}
	return points
}

func extractSubPath(points []*pb.Point, endpoints [][2]float32) []*pb.Point {
	start := -1
	end := -1
	for idx, point := range points {
		if start == -1 && almostEqual(point.Lng, endpoints[0][0]) && almostEqual(point.Lat, endpoints[0][1]) {
			start = idx
		}
		if almostEqual(point.Lng, endpoints[1][0]) && almostEqual(point.Lat, endpoints[1][1]) {
			end = idx
		}
	}
	if start == -1 || end == -1 {
		return nil
	}
	if start <= end {
		return points[start : end+1]
	}
	result := make([]*pb.Point, 0, len(points)-start+end+1)
	result = append(result, points[start:]...)
	result = append(result, points[:end+1]...)
	return result
}

func subdividedSquareRing(minLng, minLat, size float32, stepsPerEdge int) []*pb.Point {
	coords := make([][2]float32, 0, stepsPerEdge*4+1)
	for step := 0; step < stepsPerEdge; step++ {
		f := float32(step) / float32(stepsPerEdge)
		coords = append(coords, [2]float32{minLng + size*f, minLat})
	}
	for step := 0; step < stepsPerEdge; step++ {
		f := float32(step) / float32(stepsPerEdge)
		coords = append(coords, [2]float32{minLng + size, minLat + size*f})
	}
	for step := 0; step < stepsPerEdge; step++ {
		f := float32(step) / float32(stepsPerEdge)
		coords = append(coords, [2]float32{minLng + size - size*f, minLat + size})
	}
	for step := 0; step < stepsPerEdge; step++ {
		f := float32(step) / float32(stepsPerEdge)
		coords = append(coords, [2]float32{minLng, minLat + size - size*f})
	}
	coords = append(coords, coords[0])
	return lineToRing(coords)
}
