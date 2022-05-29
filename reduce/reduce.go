// Package reduce could reduce Polygon size both polygon lines and float precise.
package reduce

import (
	"math"

	"github.com/ringsaturn/tzf/pb"
)

const RADIUES = 6371 // km, for distance computing

func radians(degree float64) float64 {
	return degree * math.Pi / 180
}

func geoDistance(lat1, long1, lat2, long2 float64) float64 {
	lat1 = radians(lat1)
	long1 = radians(long1)
	lat2 = radians(lat2)
	long2 = radians(long2)

	hav_lat := 0.5 * (1 - math.Cos(lat1-lat2))
	hav_long := 0.5 * (1 - math.Cos(long1-long2))
	radical := math.Sqrt(hav_lat + math.Cos(lat1)*math.Cos(lat2)*hav_long)
	return 2 * RADIUES * math.Asin(radical) * 1000
}

func Do(input *pb.Timezones, skip int, precise float64, minist float64) *pb.Timezones {
	output := &pb.Timezones{}
	for _, timezone := range input.Timezones {
		reducedTimezone := &pb.Timezone{
			Name: timezone.Name,
		}
		for _, polygon := range timezone.Polygons {
			newPoly := &pb.Polygon{}
			maxIndex := len(polygon.Points) - 1
			for index, point := range polygon.Points {
				if index == maxIndex || index == 0 {
					newPoly.Points = append(newPoly.Points, &pb.Point{
						Lng: float32(math.Round(float64(point.Lng)*precise) / precise),
						Lat: float32(math.Round(float64(point.Lat)*precise) / precise),
					})
					continue
				}
				if index%skip != 0 {
					continue
				}
				previousPoint := newPoly.Points[len(newPoly.Points)-1]
				dist := geoDistance(float64(point.Lat), float64(point.Lng), float64(previousPoint.Lat), float64(previousPoint.Lng))
				if dist < minist {
					continue
				}
				newPoly.Points = append(newPoly.Points, &pb.Point{
					Lng: float32(math.Round(float64(point.Lng)*precise) / precise),
					Lat: float32(math.Round(float64(point.Lat)*precise) / precise),
				})
			}
			reducedTimezone.Polygons = append(reducedTimezone.Polygons, newPoly)
		}
		output.Timezones = append(output.Timezones, reducedTimezone)
	}
	return output
}
