// Derived from github.com/tidwall/geojson; see LICENSE_GEOJSON.

package geom

import "math"

// Ring is an open polygon ring: n points implying n segments, where segment i
// runs from Ring[i] to Ring[(i+1)%n]. The ring is treated as implicitly closed
// (the last point connects back to the first).
type Ring []Point

func ringBounds(r Ring) Rect {
	if len(r) == 0 {
		return Rect{}
	}
	rect := Rect{Min: r[0], Max: r[0]}
	for _, p := range r[1:] {
		if p.X < rect.Min.X {
			rect.Min.X = p.X
		}
		if p.Y < rect.Min.Y {
			rect.Min.Y = p.Y
		}
		if p.X > rect.Max.X {
			rect.Max.X = p.X
		}
		if p.Y > rect.Max.Y {
			rect.Max.Y = p.Y
		}
	}
	return rect
}

// ringAreaAndPerimeter returns the unsigned shoelace area and perimeter of an
// open ring (all n wrap-around segments included).
func ringAreaAndPerimeter(r Ring) (area, perim float64) {
	n := len(r)
	for i := range n {
		j := (i + 1) % n
		a, b := r[i], r[j]
		area += a.X*b.Y - b.X*a.Y
		dx := b.X - a.X
		dy := b.Y - a.Y
		perim += math.Sqrt(dx*dx + dy*dy)
	}
	area = math.Abs(area) * 0.5
	return
}

// openRing normalises pts to an open ring by stripping the closing duplicate
// when pts[0] == pts[n-1]. Callers may pass either open or closed point
// slices; the result is always an open ring suitable for wrap-around indexing.
func openRing(pts []Point) Ring {
	n := len(pts)
	if n >= 2 && pts[0].X == pts[n-1].X && pts[0].Y == pts[n-1].Y {
		return Ring(pts[:n-1])
	}
	return Ring(pts)
}
