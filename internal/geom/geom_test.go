package geom

import (
	"math"
	"testing"
)

// square returns a closed ring for a square with the given min/max coordinates.
func square(min, max float64) []Point {
	return []Point{
		{min, min},
		{min, max},
		{max, max},
		{max, min},
		{min, min}, // closing duplicate — stripped by openRing
	}
}

func TestPolygonContainsPoint_SimpleSquare(t *testing.T) {
	poly := NewPolygon(square(0, 10), nil)

	inside := []Point{{5, 5}, {0.1, 0.1}, {9.9, 9.9}}
	for _, p := range inside {
		if !poly.ContainsPoint(p) {
			t.Errorf("expected %v to be inside square [0,10]", p)
		}
	}

	outside := []Point{{-1, 5}, {5, -1}, {11, 5}, {5, 11}, {-1, -1}}
	for _, p := range outside {
		if poly.ContainsPoint(p) {
			t.Errorf("expected %v to be outside square [0,10]", p)
		}
	}
}

func TestPolygonContainsPoint_OnEdge(t *testing.T) {
	poly := NewPolygon(square(0, 10), nil)
	// Points exactly on edges should NOT be considered inside (strict containment).
	edges := []Point{{0, 5}, {10, 5}, {5, 0}, {5, 10}}
	for _, p := range edges {
		if poly.ContainsPoint(p) {
			t.Errorf("expected edge point %v to be outside (strict containment)", p)
		}
		if !poly.ContainsPointAllowOnEdge(p) {
			t.Errorf("expected edge point %v to be inside under the allow-on-edge rule", p)
		}
	}
}

// shiftRing returns ring translated by dx along the x axis.
func shiftRing(ring []Point, dx float64) []Point {
	out := make([]Point, len(ring))
	for i, p := range ring {
		out[i] = Point{p.X + dx, p.Y}
	}
	return out
}

// TestPolygonContainsPoint_SharedBorder covers two squares sharing the x == 10
// border: under the strict rule a query on that border belongs to neither,
// which is how a tiled polygon set grows gaps along its internal edges.
func TestPolygonContainsPoint_SharedBorder(t *testing.T) {
	// The dense rings cross minIndexSegments, so both the linear and the
	// YStripes-indexed code path get exercised.
	for _, tc := range []struct {
		name      string
		left      []Point
		right     []Point
		wantIndex bool
	}{
		{"linear", square(0, 10), shiftRing(square(0, 10), 10), false},
		{"indexed", denseSquare(0, 10, 200), shiftRing(denseSquare(0, 10, 200), 10), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left := NewPolygon(tc.left, nil)
			right := NewPolygon(tc.right, nil)

			if got := left.extIdx != nil; got != tc.wantIndex {
				t.Fatalf("YStripes index built = %v, want %v", got, tc.wantIndex)
			}

			for _, p := range []Point{{10, 5}, {10, 10}, {10, 0}} {
				if left.ContainsPoint(p) || right.ContainsPoint(p) {
					t.Errorf("border point %v: expected strict containment to reject both sides", p)
				}
				if !left.ContainsPointAllowOnEdge(p) {
					t.Errorf("border point %v: expected left polygon to claim it", p)
				}
				if !right.ContainsPointAllowOnEdge(p) {
					t.Errorf("border point %v: expected right polygon to claim it", p)
				}
			}

			// Points off the border are unaffected by the edge rule.
			for _, p := range []Point{{5, 5}, {15, 5}, {25, 5}, {10, 25}} {
				if left.ContainsPoint(p) != left.ContainsPointAllowOnEdge(p) {
					t.Errorf("point %v: left polygon changed under the edge rule", p)
				}
				if right.ContainsPoint(p) != right.ContainsPointAllowOnEdge(p) {
					t.Errorf("point %v: right polygon changed under the edge rule", p)
				}
			}
		})
	}
}

func TestRaycastSeg(t *testing.T) {
	tests := []struct {
		name       string
		a, b, p    Point
		inside, on bool
	}{
		{"crossing", Point{10, -1}, Point{10, 1}, Point{0, 0}, true, false},
		{"right of segment", Point{10, -1}, Point{10, 1}, Point{20, 0}, false, false},
		{"horizontal boundary", Point{0, 0}, Point{10, 0}, Point{5, 0}, false, true},
		{"vertical boundary", Point{0, 0}, Point{0, 10}, Point{0, 5}, false, true},
		{"diagonal boundary", Point{0, 0}, Point{10, 10}, Point{5, 5}, false, true},
		{"degenerate boundary", Point{2, 3}, Point{2, 3}, Point{2, 3}, false, true},
		{"degenerate miss", Point{2, 3}, Point{2, 3}, Point{3, 3}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inside, on := RaycastSeg(tc.a, tc.b, tc.p)
			if inside != tc.inside || on != tc.on {
				t.Fatalf("RaycastSeg = %v,%v, want %v,%v", inside, on, tc.inside, tc.on)
			}
		})
	}
}

// TestPolygonContainsPoint_AllowOnEdgeHoleBoundary pins the rule to the
// exterior: a hole's boundary counts as polygon interior either way, so the
// ring filling the hole and the ring around it both claim it rather than both
// dropping it.
func TestPolygonContainsPoint_AllowOnEdgeHoleBoundary(t *testing.T) {
	poly := NewPolygon(square(0, 10), [][]Point{square(3, 7)})
	fill := NewPolygon(square(3, 7), nil)

	for _, p := range []Point{{3, 5}, {3, 3}} {
		if !poly.ContainsPoint(p) || !poly.ContainsPointAllowOnEdge(p) {
			t.Errorf("hole boundary %v: expected the surrounding polygon to claim it under both rules", p)
		}
		if fill.ContainsPoint(p) {
			t.Errorf("hole boundary %v: expected strict containment to reject the filling polygon", p)
		}
		if !fill.ContainsPointAllowOnEdge(p) {
			t.Errorf("hole boundary %v: expected the filling polygon to claim it under the allow-on-edge rule", p)
		}
	}

	// Inside the hole proper stays outside under both rules.
	if poly.ContainsPoint(Point{5, 5}) || poly.ContainsPointAllowOnEdge(Point{5, 5}) {
		t.Error("expected a point inside the hole to stay outside under both rules")
	}
}

// denseSquare returns a square ring with n extra vertices per side, enough to
// cross the minimum segment count for building a YStripes index.
func denseSquare(min, max float64, n int) []Point {
	step := (max - min) / float64(n)
	ring := make([]Point, 0, 4*n+1)
	for i := range n { // left edge, going up
		ring = append(ring, Point{min, min + step*float64(i)})
	}
	for i := range n { // top edge
		ring = append(ring, Point{min + step*float64(i), max})
	}
	for i := range n { // right edge, going down
		ring = append(ring, Point{max, max - step*float64(i)})
	}
	for i := range n { // bottom edge
		ring = append(ring, Point{max - step*float64(i), min})
	}
	return append(ring, ring[0])
}

func TestPolygonContainsPoint_WithHole(t *testing.T) {
	// Outer: [0,10] square; hole: [3,7] square.
	poly := NewPolygon(square(0, 10), [][]Point{square(3, 7)})

	if !poly.ContainsPoint(Point{1, 1}) {
		t.Error("expected point in outer ring but outside hole to be inside")
	}
	if poly.ContainsPoint(Point{5, 5}) {
		t.Error("expected point in hole to be outside")
	}
	if poly.ContainsPoint(Point{-1, -1}) {
		t.Error("expected point outside outer ring to be outside")
	}
}

func TestPolygonBoundingBox(t *testing.T) {
	poly := NewPolygon(square(2, 8), nil)
	r := poly.Rect()
	if r.Min.X != 2 || r.Min.Y != 2 || r.Max.X != 8 || r.Max.Y != 8 {
		t.Errorf("unexpected rect %+v", r)
	}
}

func TestOpenRing_StripsClosingDuplicate(t *testing.T) {
	pts := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 0}}
	r := openRing(pts)
	if len(r) != 3 {
		t.Errorf("expected 3 points, got %d", len(r))
	}
}

func TestOpenRing_NoClosingDuplicate(t *testing.T) {
	pts := []Point{{0, 0}, {1, 0}, {1, 1}}
	r := openRing(pts)
	if len(r) != 3 {
		t.Errorf("expected 3 points, got %d", len(r))
	}
}

func TestYStripes_LargePolygon(t *testing.T) {
	// Build a regular polygon with many points to exercise the YStripes index.
	const n = 200
	pts := make([]Point, n+1)
	for i := range n {
		angle := 2 * math.Pi * float64(i) / n
		pts[i] = Point{math.Cos(angle) * 10, math.Sin(angle) * 10}
	}
	pts[n] = pts[0] // closing duplicate

	poly := NewPolygon(pts, nil)
	if poly.extIdx == nil {
		t.Fatal("expected YStripes index to be built for large polygon")
	}

	if !poly.ContainsPoint(Point{0, 0}) {
		t.Error("expected centre to be inside circle polygon")
	}
	if poly.ContainsPoint(Point{11, 0}) {
		t.Error("expected far point to be outside circle polygon")
	}
}

func TestCalcStripeCount_MinBound(t *testing.T) {
	// A degenerate ring (no area) should still produce at least yStripesMin stripes.
	r := Ring{{0, 0}, {1, 0}, {0, 0}} // collinear, zero area
	n := calcStripeCount(r)
	if n < yStripesMin {
		t.Errorf("expected at least %d stripes, got %d", yStripesMin, n)
	}
}

func TestContainsPoly(t *testing.T) {
	outer := NewPolygon(square(0, 10), nil)
	inner := NewPolygon(square(2, 8), nil)
	smaller := NewPolygon(square(11, 15), nil)

	if !outer.ContainsPoly(inner) {
		t.Error("outer should contain inner")
	}
	if outer.ContainsPoly(smaller) {
		t.Error("outer should not contain smaller (outside)")
	}
}

func circleRing(n int, radius float64) []Point {
	pts := make([]Point, n+1)
	for i := range n {
		angle := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = Point{math.Cos(angle) * radius, math.Sin(angle) * radius}
	}
	pts[n] = pts[0]
	return pts
}

// BenchmarkContainsPoint_WithIndex measures PIP performance with YStripes index.
func BenchmarkContainsPoint_WithIndex(b *testing.B) {
	poly := NewPolygon(circleRing(500, 10), nil)
	p := Point{1, 1}
	b.ResetTimer()
	for range b.N {
		poly.ContainsPoint(p)
	}
}

// BenchmarkContainsPoint_LinearScan measures PIP performance via direct linear
// scan on the same 500-point ring (bypasses the index).
func BenchmarkContainsPoint_LinearScan(b *testing.B) {
	r := openRing(circleRing(500, 10))
	p := Point{1, 1}
	b.ResetTimer()
	for range b.N {
		ringContainsPoint(r, nil, p, false) // nil index → linear scan
	}
}

// BenchmarkContainsPoint_NoIndex measures PIP performance via linear scan on a
// small ring (no index built).
func BenchmarkContainsPoint_NoIndex(b *testing.B) {
	poly := NewPolygon(square(0, 10), nil)
	p := Point{5, 5}
	b.ResetTimer()
	for range b.N {
		poly.ContainsPoint(p)
	}
}
