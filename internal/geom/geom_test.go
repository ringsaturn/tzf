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
	}
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
		ringContainsPoint(r, nil, p) // nil index → linear scan
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
