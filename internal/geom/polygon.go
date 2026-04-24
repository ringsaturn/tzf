package geom

// minIndexSegments is the minimum ring length (in unique points) required to
// build a YStripes index. Smaller rings are scanned linearly.
const minIndexSegments = 32

// Polygon is a 2-D polygon with an exterior ring and optional interior holes.
//
// YStripes indices are built automatically for rings with at least
// minIndexSegments unique points, reducing PIP queries from O(n) to O(n/k).
type Polygon struct {
	exterior Ring
	holes    []Ring
	rect     Rect
	extIdx   *yStripesIndex
	holeIdxs []*yStripesIndex
}

// NewPolygon creates a Polygon from an exterior ring and optional holes.
//
// Closing duplicate points (exterior[0] == exterior[n-1]) are stripped so that
// the ring is stored in open form. YStripes indices are built for rings with at
// least minIndexSegments unique points.
func NewPolygon(exterior []Point, holes [][]Point) *Polygon {
	ext := openRing(exterior)
	hls := make([]Ring, len(holes))
	for i, h := range holes {
		hls[i] = openRing(h)
	}

	poly := &Polygon{
		exterior: ext,
		holes:    hls,
	}
	if len(ext) > 0 {
		poly.rect = ringBounds(ext)
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

// Rect returns the axis-aligned bounding box of the polygon's exterior ring.
func (poly *Polygon) Rect() Rect {
	return poly.rect
}

// ContainsPoint reports whether p lies strictly inside the polygon: inside the
// exterior ring and outside all hole rings.  Points on a ring boundary return
// false.
func (poly *Polygon) ContainsPoint(p Point) bool {
	if !poly.rect.containsPoint(p) {
		return false
	}
	if !ringContainsPoint(poly.exterior, poly.extIdx, p) {
		return false
	}
	for i, h := range poly.holes {
		if ringContainsPoint(h, poly.holeIdxs[i], p) {
			return false
		}
	}
	return true
}

// ContainsPoly reports whether every point of other's exterior ring is inside
// this polygon. This is a point-sample approximation of polygon containment.
func (poly *Polygon) ContainsPoly(other *Polygon) bool {
	for _, pt := range other.exterior {
		if !poly.ContainsPoint(pt) {
			return false
		}
	}
	return true
}
