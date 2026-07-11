// Derived from github.com/tidwall/geojson; see LICENSE_GEOJSON.

package geom

// minIndexSegments is the minimum ring length (in unique points) required to
// build a YStripes index. Smaller rings are scanned linearly.
const minIndexSegments = 32

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

// PolygonOf is a 2-D polygon with an exterior ring and optional interior
// holes, stored in the space of T: degrees for float64, 1e5-scaled integers
// for int32 (halving per-point storage). scale converts degree-space query
// points into storage space (1 for float64, I32Scale for int32).
//
// Queries scale the point once and run the standard float64 raycast on
// in-register converted endpoints, so results are identical across storage
// types for coordinates representable in both. YStripes indices are built
// automatically for rings with at least minIndexSegments unique points,
// reducing PIP queries from O(n) to O(n/k); they live in storage space.
type PolygonOf[T Coord] struct {
	exterior RingOf[T]
	holes    []RingOf[T]
	min, max PointOf[T] // storage-space bounding box of the exterior
	scale    float64
	extIdx   *yStripesIndex
	holeIdxs []*yStripesIndex
}

// Polygon is a polygon stored in degree space (float64).
type Polygon = PolygonOf[float64]

// I32Polygon is a polygon stored as 1e5-scaled int32 coordinates.
type I32Polygon = PolygonOf[int32]

// NewPolygon creates a degree-space Polygon from an exterior ring and
// optional holes.
func NewPolygon(exterior []Point, holes [][]Point) *Polygon {
	return newPolygonOf(exterior, holes, 1)
}

// NewI32Polygon creates an I32Polygon from a 1e5-scaled integer exterior ring
// and optional holes.
func NewI32Polygon(exterior []I32Point, holes [][]I32Point) *I32Polygon {
	return newPolygonOf(exterior, holes, I32Scale)
}

// newPolygonOf creates a PolygonOf from an exterior ring and optional holes
// in storage space. Closing duplicate points (exterior[0] == exterior[n-1])
// are stripped so that rings are stored in open form.
func newPolygonOf[T Coord](exterior []PointOf[T], holes [][]PointOf[T], scale float64) *PolygonOf[T] {
	ext := openRing(exterior)
	hls := make([]RingOf[T], len(holes))
	for i, h := range holes {
		hls[i] = openRing(h)
	}

	poly := &PolygonOf[T]{
		exterior: ext,
		holes:    hls,
		scale:    scale,
	}
	if len(ext) > 0 {
		poly.min, poly.max = ringBounds(ext)
	}
	if len(ext) >= minIndexSegments {
		poly.extIdx = buildYStripes(ext)
	}
	poly.holeIdxs = make([]*yStripesIndex, len(hls))
	for i, h := range hls {
		if len(h) >= minIndexSegments {
			poly.holeIdxs[i] = buildYStripes(h)
		}
	}
	return poly
}

// ContainsPoint reports whether the degree-space point p lies strictly inside
// the polygon: inside the exterior ring and outside all hole rings. Points on
// a ring boundary return false.
func (poly *PolygonOf[T]) ContainsPoint(p Point) bool {
	sp := Point{X: p.X * poly.scale, Y: p.Y * poly.scale}
	if sp.X < float64(poly.min.X) || sp.X > float64(poly.max.X) ||
		sp.Y < float64(poly.min.Y) || sp.Y > float64(poly.max.Y) {
		return false
	}
	if !ringContainsPoint(poly.exterior, poly.extIdx, sp) {
		return false
	}
	for i, h := range poly.holes {
		if ringContainsPoint(h, poly.holeIdxs[i], sp) {
			return false
		}
	}
	return true
}

// ContainsPoly reports whether every point of other's exterior ring is inside
// this polygon. This is a point-sample approximation of polygon containment.
func (poly *PolygonOf[T]) ContainsPoly(other *PolygonOf[T]) bool {
	for _, pt := range other.exterior {
		p := Point{X: float64(pt.X) / other.scale, Y: float64(pt.Y) / other.scale}
		if !poly.ContainsPoint(p) {
			return false
		}
	}
	return true
}

// Rect returns the degree-space bounding box of the polygon's exterior ring.
func (poly *PolygonOf[T]) Rect() Rect {
	return Rect{
		Min: Point{X: float64(poly.min.X) / poly.scale, Y: float64(poly.min.Y) / poly.scale},
		Max: Point{X: float64(poly.max.X) / poly.scale, Y: float64(poly.max.Y) / poly.scale},
	}
}

// Exterior returns the exterior ring in degree space, open form (the closing
// point is not repeated). Degree-space polygons return the stored ring
// without copying; scaled polygons return a fresh allocation.
func (poly *PolygonOf[T]) Exterior() Ring {
	return toDegreeRing(poly.exterior, poly.scale)
}

// Holes returns the interior hole rings in degree space, open form. The same
// copying rules as Exterior apply.
func (poly *PolygonOf[T]) Holes() []Ring {
	out := make([]Ring, len(poly.holes))
	for i, h := range poly.holes {
		out[i] = toDegreeRing(h, poly.scale)
	}
	return out
}

func toDegreeRing[T Coord](r RingOf[T], scale float64) Ring {
	if fr, ok := any(r).(Ring); ok && scale == 1 {
		return fr
	}
	out := make(Ring, len(r))
	for i, p := range r {
		out[i] = Point{X: float64(p.X) / scale, Y: float64(p.Y) / scale}
	}
	return out
}
