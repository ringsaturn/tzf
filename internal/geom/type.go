// Derived from github.com/tidwall/geojson; see LICENSE_GEOJSON.

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
