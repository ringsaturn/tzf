// Package geom provides zero-dependency 2-D geometry primitives and a
// YStripes-indexed point-in-polygon query optimised for timezone lookup.
//
// The YStripes index divides each polygon ring's Y-axis span into horizontal
// stripes. A PIP query needs only to scan the segments stored in the single
// stripe that contains the query latitude, reducing work from O(n) to roughly
// O(n/stripes) in the common case.
//
// The ray-casting algorithm is a direct Go port of github.com/tidwall/geojson
// (MIT), which is itself the canonical implementation used throughout this
// project.
package geom

// Point is a 2-D coordinate. X represents longitude; Y represents latitude.
type Point struct {
	X, Y float64
}

// Rect is an axis-aligned bounding box.
type Rect struct {
	Min, Max Point
}

func (r Rect) containsPoint(p Point) bool {
	return p.X >= r.Min.X && p.X <= r.Max.X &&
		p.Y >= r.Min.Y && p.Y <= r.Max.Y
}
