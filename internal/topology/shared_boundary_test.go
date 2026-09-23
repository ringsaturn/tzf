package topology

import (
	"testing"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

// TestDo_SharedBordersFormPartition pins the strongest form of the invariant:
// after simplification the rings sharing a border must still tile the plane
// along it, i.e. sample points just off the source border belong to exactly
// one of the two rings. A gap means both rings retreated from the border; an
// overlap means both claimed it. Either way the two sides were simplified
// independently instead of through the shared-segment cache.
//
// Regression for the shipped lite gap at 16.06440,-82.33148.
func TestDo_SharedBordersFormPartition(t *testing.T) {
	reverse := func(in [][2]float32) [][2]float32 {
		out := make([][2]float32, 0, len(in))
		for i := len(in) - 1; i >= 0; i-- {
			out = append(out, in[i])
		}
		return out
	}
	cat := func(parts ...[][2]float32) [][2]float32 {
		var out [][2]float32
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	// Case 1: a wavy border between two rings that meet nowhere else, with the
	// left ring rotated so its start vertex sits inside the shared chain. This
	// is the layout that used to diverge.
	border1 := [][2]float32{
		{0, 4}, {0.0020, 3.6}, {-0.0015, 3.2}, {0.0025, 2.8}, {-0.0020, 2.4},
		{0.0010, 2.0}, {-0.0025, 1.6}, {0.0015, 1.2}, {0, 1.0},
	}
	leftSide := [][2]float32{
		{-0.5, 0.8}, {-1.2, 0.6}, {-2.0, 1.0}, {-2.6, 2.0},
		{-2.8, 3.0}, {-2.5, 3.8}, {-1.8, 4.4}, {-0.8, 4.5},
	}
	rightSide := [][2]float32{
		{0.8, 4.5}, {1.8, 4.4}, {2.5, 3.8}, {2.8, 3.0},
		{2.6, 2.0}, {2.0, 1.0}, {1.2, 0.6}, {0.5, 0.8},
	}
	leftRotated := cat(border1, leftSide)
	leftRotated = append(leftRotated[4:], leftRotated[:4]...)

	// Case 2: one ring sharing consecutive borders with two different partners,
	// exercising the partner-change vertex.
	bottomChain := [][2]float32{{0, 0}, {0.5, 0.02}, {1, -0.02}, {1.5, 0.02}, {2, 0}}
	rightChain := [][2]float32{{2, 0}, {2.01, 0.7}, {2.02, 1.3}, {2.01, 1.7}, {2, 2}}
	centerRing := cat(bottomChain, rightChain[1:], [][2]float32{[2]float32{0, 2}})
	bottomRing := cat(reverse(bottomChain), [][2]float32{[2]float32{0, -1}, [2]float32{2, -1}})
	rightRing := cat(reverse(rightChain), [][2]float32{[2]float32{3, 0}, [2]float32{3, 2}})

	tests := []struct {
		name         string
		input        *pb.Timezones
		epsilon      float64
		wantCacheHit bool
		checks       []partitionCheck
	}{
		{
			name:         "two_ring_rotated",
			epsilon:      0.005,
			wantCacheHit: true,
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Left", Polygons: []*pb.Polygon{{Points: lineToRing(leftRotated)}}},
				{Name: "Right", Polygons: []*pb.Polygon{{Points: lineToRing(cat(rightSide, reverse(border1)))}}},
			}},
			checks: []partitionCheck{{a: "Left", b: "Right", chain: border1, delta: 0.0005}},
		},
		{
			name:         "partner_change",
			epsilon:      0.03,
			wantCacheHit: true,
			input: &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
				{Name: "Center", Polygons: []*pb.Polygon{{Points: lineToRing(centerRing)}}},
				{Name: "Bottom", Polygons: []*pb.Polygon{{Points: lineToRing(bottomRing)}}},
				{Name: "Right", Polygons: []*pb.Polygon{{Points: lineToRing(rightRing)}}},
			}},
			checks: []partitionCheck{
				{a: "Center", b: "Bottom", chain: bottomChain, delta: 0.002},
				{a: "Center", b: "Right", chain: rightChain, delta: 0.002},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, stats := DoWithStats(tc.input, tc.epsilon)
			if err := Validate(out); err != nil {
				t.Fatalf("Validate returned error: %v", err)
			}
			if tc.wantCacheHit && stats.SharedCacheHits == 0 {
				t.Fatalf("shared-segment cache never hit (shared=%d misses=%d); borders simplified independently",
					stats.SharedSegments, stats.SharedCacheMisses)
			}
			for _, check := range tc.checks {
				assertPartition(t, out, check)
			}
		})
	}
}

type partitionCheck struct {
	a, b  string
	chain [][2]float32
	delta float64
}

// assertPartition samples just off a source shared chain and requires exactly
// one of rings a and b to cover each sample.
func assertPartition(t *testing.T, out *pb.Timezones, check partitionCheck) {
	t.Helper()
	ra, rb := ringByName(out, check.a), ringByName(out, check.b)
	if ra == nil || rb == nil {
		t.Fatalf("missing ring: %s=%v %s=%v", check.a, ra != nil, check.b, rb != nil)
	}
	for i := 0; i+1 < len(check.chain); i++ {
		ax, ay := check.chain[i][0], check.chain[i][1]
		bx, by := check.chain[i+1][0], check.chain[i+1][1]
		dx, dy := bx-ax, by-ay
		l := dx*dx + dy*dy
		if l == 0 {
			continue
		}
		nx, ny := -dy/l, dx/l
		for s := float32(0.05); s < 1; s += 0.05 {
			mx, my := ax+dx*s, ay+dy*s
			for _, side := range []float64{-1, 1} {
				px, py := float64(mx)+side*float64(nx)*check.delta, float64(my)+side*float64(ny)*check.delta
				inA := pointInRing(px, py, ra)
				inB := pointInRing(px, py, rb)
				if !inA && !inB {
					t.Fatalf("coverage gap at (%.5f,%.5f) on %s/%s border segment %d", px, py, check.a, check.b, i)
				}
				if inA && inB {
					t.Fatalf("coverage overlap at (%.5f,%.5f) on %s/%s border segment %d", px, py, check.a, check.b, i)
				}
			}
		}
	}
}

func ringByName(out *pb.Timezones, name string) []*pb.Point {
	for _, tz := range out.Timezones {
		if tz.Name == name {
			return tz.Polygons[0].Points
		}
	}
	return nil
}

// pointInRing is a plain even-odd ray cast used by the shared-boundary tests.
func pointInRing(lng, lat float64, ring []*pb.Point) bool {
	inside := false
	n := len(ring)
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := float64(ring[i].Lng), float64(ring[i].Lat)
		xj, yj := float64(ring[j].Lng), float64(ring[j].Lat)
		if (yi > lat) != (yj > lat) && lng < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
