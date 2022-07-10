// Package tzkey
package tzkey

import (
	"github.com/ringsaturn/tzf/pb"
	"github.com/uber/h3-go"
	"golang.org/x/exp/maps"
)

func Do(input *pb.Timezones, resolutin int) map[string]string {
	res := make(map[string]string, 0)
	for _, pbtz := range input.Timezones {
		newRes := DoItem(pbtz, resolutin)
		maps.Copy(res, newRes)
	}
	return res
}

func DoItem(input *pb.Timezone, resolution int) map[string]string {
	h3geopoly := &h3.GeoPolygon{
		Geofence: make([]h3.GeoCoord, 0),
		Holes:    make([][]h3.GeoCoord, 0),
	}

	for polygonIdx, polygon := range input.Polygons {
		// Geofence
		if polygonIdx == 0 {
			for _, point := range polygon.Points {
				h3coord := h3.GeoCoord{Latitude: float64(point.Lat), Longitude: float64(point.Lng)}
				h3geopoly.Geofence = append(h3geopoly.Geofence, h3coord)
			}
			continue
		}

		// Holes
		hole := make([]h3.GeoCoord, 0)
		for _, point := range polygon.Points {
			h3coord := h3.GeoCoord{Latitude: float64(point.Lat), Longitude: float64(point.Lng)}
			hole = append(hole, h3coord)
		}
		h3geopoly.Holes = append(h3geopoly.Holes, hole)
	}

	h3idxs := h3.Polyfill(*h3geopoly, resolution)
	res := make(map[string]string, 0)
	for _, h3idx := range h3idxs {
		res[h3.ToString(h3idx)] = input.Name
	}
	return res
}
