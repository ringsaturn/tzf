// Derived from github.com/tidwall/geojson; see LICENSE_GEOJSON.

package geom

// Coord is the coordinate storage type of a polygon: float64 for degree-space
// coordinates, int32 for 1e5-scaled polyline-grid coordinates. All geometric
// arithmetic runs in float64 regardless; the type parameter only governs how
// ring points are stored in memory.
type Coord interface {
	~int32 | ~float64
}

// I32Scale is the fixed coordinate scale for integer polygons: 1e5, the
// Google Encoded Polyline grid used by the compressed tzf data.
const I32Scale = 1e5

// PointOf is a 2-D coordinate. X represents longitude; Y represents latitude,
// in the storage space of T (degrees for float64, 1e5-scaled for int32).
type PointOf[T Coord] struct {
	X, Y T
}

// Point is a 2-D coordinate in degree space.
type Point = PointOf[float64]

// I32Point is a coordinate stored as a 1e5-scaled integer, halving the
// storage of a float64 Point.
type I32Point = PointOf[int32]

// Rect is an axis-aligned bounding box in degree space.
type Rect struct {
	Min, Max Point
}
