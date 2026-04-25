package convert

import (
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
)

func FromTimezonePBToGeometryPoly(timezone *pb.Timezone) []*geom.Polygon {
	ret := make([]*geom.Polygon, 0, len(timezone.Polygons))
	for _, polygon := range timezone.Polygons {
		newPoints := make([]geom.Point, 0, len(polygon.Points))
		for _, point := range polygon.Points {
			newPoints = append(newPoints, geom.Point{X: float64(point.Lng), Y: float64(point.Lat)})
		}

		holes := make([][]geom.Point, 0, len(polygon.Holes))
		for _, holePoly := range polygon.Holes {
			newHolePoints := make([]geom.Point, 0, len(holePoly.Points))
			for _, point := range holePoly.Points {
				newHolePoints = append(newHolePoints, geom.Point{X: float64(point.Lng), Y: float64(point.Lat)})
			}
			holes = append(holes, newHolePoints)
		}

		ret = append(ret, geom.NewPolygon(newPoints, holes))
	}
	return ret
}
