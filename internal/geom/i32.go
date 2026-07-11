package geom

// I32Scale is the fixed coordinate scale for integer polygons: 1e5, the
// Google Encoded Polyline grid used by the compressed tzf data.
const I32Scale = 1e5

// I32Point is a coordinate stored as a 1e5-scaled integer, halving the
// storage of a float64 Point.
type I32Point struct {
	X, Y int32
}

// I32Ring is an open polygon ring of scaled-integer points, with the same
// wrap-around segment convention as Ring.
type I32Ring []I32Point

// Poly is the query/export interface shared by Polygon and I32Polygon.
type Poly interface {
	// ContainsPoint reports whether the degree-space point p lies strictly
	// inside the polygon.
	ContainsPoint(p Point) bool
	// Rect returns the degree-space bounding box of the exterior ring.
	Rect() Rect
	// Exterior returns the exterior ring in degree space, open form.
	Exterior() Ring
	// Holes returns the interior hole rings in degree space, open form.
	Holes() []Ring
}

var (
	_ Poly = (*Polygon)(nil)
	_ Poly = (*I32Polygon)(nil)
)

// I32Polygon is a Polygon whose coordinates are stored as 1e5-scaled int32.
//
// Queries scale the point once and run the standard float64 raycast on
// in-register converted endpoints, so results match a float64 Polygon built
// from the same scaled-integer coordinates. YStripes indices are built and
// queried in the scaled space.
type I32Polygon struct {
	exterior I32Ring
	holes    []I32Ring
	min, max I32Point // scaled-space bounding box of the exterior
	extIdx   *yStripesIndex
	holeIdxs []*yStripesIndex
}

// NewI32Polygon creates an I32Polygon from a scaled-integer exterior ring and
// optional holes. Closing duplicate points are stripped, mirroring NewPolygon.
func NewI32Polygon(exterior []I32Point, holes [][]I32Point) *I32Polygon {
	ext := openI32Ring(exterior)
	hls := make([]I32Ring, len(holes))
	for i, h := range holes {
		hls[i] = openI32Ring(h)
	}

	poly := &I32Polygon{
		exterior: ext,
		holes:    hls,
	}
	if len(ext) > 0 {
		poly.min, poly.max = i32RingBounds(ext)
	}
	if len(ext) >= minIndexSegments {
		poly.extIdx = buildYStripesI32(ext)
	}
	poly.holeIdxs = make([]*yStripesIndex, len(hls))
	for i, h := range hls {
		if len(h) >= minIndexSegments {
			poly.holeIdxs[i] = buildYStripesI32(h)
		}
	}
	return poly
}

func openI32Ring(pts []I32Point) I32Ring {
	n := len(pts)
	if n >= 2 && pts[0] == pts[n-1] {
		return I32Ring(pts[:n-1])
	}
	return I32Ring(pts)
}

func i32RingBounds(r I32Ring) (min, max I32Point) {
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

// buildYStripesI32 builds the stripe index in scaled space via a transient
// float64 copy of the ring; only the compact index is retained.
func buildYStripesI32(r I32Ring) *yStripesIndex {
	tmp := make(Ring, len(r))
	for i, p := range r {
		tmp[i] = Point{X: float64(p.X), Y: float64(p.Y)}
	}
	return buildYStripes(tmp)
}

// ContainsPoint reports whether the degree-space point p lies strictly inside
// the polygon. Points on a ring boundary return false.
func (poly *I32Polygon) ContainsPoint(p Point) bool {
	sp := Point{X: p.X * I32Scale, Y: p.Y * I32Scale}
	if sp.X < float64(poly.min.X) || sp.X > float64(poly.max.X) ||
		sp.Y < float64(poly.min.Y) || sp.Y > float64(poly.max.Y) {
		return false
	}
	if !i32RingContainsPoint(poly.exterior, poly.extIdx, sp) {
		return false
	}
	for i, h := range poly.holes {
		if i32RingContainsPoint(h, poly.holeIdxs[i], sp) {
			return false
		}
	}
	return true
}

// Rect returns the degree-space bounding box of the polygon's exterior ring.
func (poly *I32Polygon) Rect() Rect {
	return Rect{
		Min: Point{X: float64(poly.min.X) / I32Scale, Y: float64(poly.min.Y) / I32Scale},
		Max: Point{X: float64(poly.max.X) / I32Scale, Y: float64(poly.max.Y) / I32Scale},
	}
}

// Exterior returns the exterior ring converted to degree space.
// Unlike Polygon.Exterior, the returned ring is a fresh allocation.
func (poly *I32Polygon) Exterior() Ring {
	return i32RingToRing(poly.exterior)
}

// Holes returns the interior hole rings converted to degree space.
// Unlike Polygon.Holes, the returned rings are fresh allocations.
func (poly *I32Polygon) Holes() []Ring {
	out := make([]Ring, len(poly.holes))
	for i, h := range poly.holes {
		out[i] = i32RingToRing(h)
	}
	return out
}

func i32RingToRing(r I32Ring) Ring {
	out := make(Ring, len(r))
	for i, p := range r {
		out[i] = Point{X: float64(p.X) / I32Scale, Y: float64(p.Y) / I32Scale}
	}
	return out
}

func i32pt(p I32Point) Point {
	return Point{X: float64(p.X), Y: float64(p.Y)}
}

// i32RingContainsPoint is ringContainsPoint over a scaled-integer ring; p must
// already be scaled by I32Scale. Segment endpoints are converted to float64 in
// registers, so the raycast itself is identical to the float path.
func i32RingContainsPoint(r I32Ring, idx *yStripesIndex, p Point) bool {
	n := len(r)
	if n < 3 {
		return false
	}

	inside := false

	if idx != nil {
		// Indexed path: iterate only the stripe containing p.Y.
		idx.forEachCandidateI32(r, p.Y, func(i int) bool {
			j := (i + 1) % n
			res := raycastSeg(i32pt(r[i]), i32pt(r[j]), p)
			if res.on {
				inside = false
				return false // stop
			}
			if res.inside {
				inside = !inside
			}
			return true
		})
		return inside
	}

	// Linear fallback for small rings.
	for i := range n {
		j := (i + 1) % n
		res := raycastSeg(i32pt(r[i]), i32pt(r[j]), p)
		if res.on {
			return false
		}
		if res.inside {
			inside = !inside
		}
	}
	return inside
}

// forEachCandidateI32 is forEachCandidate over a scaled-integer ring; y must
// already be scaled by I32Scale.
func (idx *yStripesIndex) forEachCandidateI32(r I32Ring, y float64, fn func(int) bool) {
	if y < idx.minY || y > idx.minY+idx.height {
		return
	}
	n := len(r)
	s := pointStripe(y, idx.minY, idx.height, len(idx.stripes))
	stripe := idx.stripes[s]
	start := int(stripe.start)
	for k := start; k < start+int(stripe.count); k++ {
		seg := int(idx.indexes[k])
		ay, by := float64(r[seg].Y), float64(r[(seg+1)%n].Y)
		if by < ay {
			ay, by = by, ay
		}
		if y >= ay && y <= by {
			if !fn(seg) {
				return
			}
		}
	}
}
