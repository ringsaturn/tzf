// Derived from github.com/tidwall/geojson; see LICENSE_GEOJSON.

package geom

import "math"

// RingOf is an open polygon ring: n points implying n segments, where segment
// i runs from RingOf[i] to RingOf[(i+1)%n]. The ring is treated as implicitly
// closed (the last point connects back to the first).
type RingOf[T Coord] []PointOf[T]

// Ring is an open ring in degree space.
type Ring = RingOf[float64]

// I32Ring is an open ring of 1e5-scaled integer points.
type I32Ring = RingOf[int32]

// ringBounds returns the storage-space bounding box of r; r must be non-empty.
func ringBounds[T Coord](r RingOf[T]) (min, max PointOf[T]) {
	min, max = r[0], r[0]
	for _, p := range r[1:] {
		if p.X < min.X {
			min.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	return
}

// ringAreaAndPerimeter returns the unsigned shoelace area and perimeter of an
// open ring (all n wrap-around segments included), in storage space.
func ringAreaAndPerimeter[T Coord](r RingOf[T]) (area, perim float64) {
	n := len(r)
	for i := range n {
		j := (i + 1) % n
		ax, ay := float64(r[i].X), float64(r[i].Y)
		bx, by := float64(r[j].X), float64(r[j].Y)
		area += ax*by - bx*ay
		dx := bx - ax
		dy := by - ay
		perim += math.Sqrt(dx*dx + dy*dy)
	}
	area = math.Abs(area) * 0.5
	return
}

// openRing normalises pts to an open ring by stripping the closing duplicate
// when pts[0] == pts[n-1]. Callers may pass either open or closed point
// slices; the result is always an open ring suitable for wrap-around indexing.
func openRing[T Coord](pts []PointOf[T]) RingOf[T] {
	n := len(pts)
	if n >= 2 && pts[0] == pts[n-1] {
		return RingOf[T](pts[:n-1])
	}
	return RingOf[T](pts)
}
