// Package reduce could reduce Polygon size both polygon lines and float precise.
package reduce

import (
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/simplify"
	"github.com/ringsaturn/tzf/pb"
)

func Do(input *pb.Timezones, skip int, precise float64, minist float64) *pb.Timezones {
	output := &pb.Timezones{}
	for _, timezone := range input.Timezones {
		reducedTimezone := &pb.Timezone{
			Name: timezone.Name,
		}
		for _, polygon := range timezone.Polygons {
			newPoly := &pb.Polygon{}

			original := orb.LineString{}
			for _, point := range polygon.Points {
				original = append(original, orb.Point{float64(point.Lng), float64(point.Lat)})
			}
			reduced := simplify.DouglasPeucker(0.001).Simplify(original.Clone()).(orb.LineString)
			for _, orbPoint := range reduced {
				newPoly.Points = append(newPoly.Points, &pb.Point{
					Lng: float32(orbPoint.Lon()),
					Lat: float32(orbPoint.Lat()),
				})
			}
			reducedTimezone.Polygons = append(reducedTimezone.Polygons, newPoly)
		}
		output.Timezones = append(output.Timezones, reducedTimezone)
	}
	return output
}
