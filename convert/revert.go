package convert

import (
	"encoding/json"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
)

func FromPbPolygonToGeoMultipolygon(pbpoly []*pb.Polygon) MultiPolygonCoordinates {
	res := MultiPolygonCoordinates{}
	for _, poly := range pbpoly {
		newGeoPoly := make(PolygonCoordinates, 0)

		mainpoly := [][2]float64{}
		for _, point := range poly.Points {
			mainpoly = append(mainpoly, [2]float64{float64(point.Lng), float64(point.Lat)})
		}
		newGeoPoly = append(newGeoPoly, mainpoly)

		for _, holepoly := range poly.Holes {
			holepolyCoords := [][2]float64{}
			for _, point := range holepoly.Points {
				holepolyCoords = append(holepolyCoords, [2]float64{float64(point.Lng), float64(point.Lat)})
			}
			newGeoPoly = append(newGeoPoly, holepolyCoords)
		}
		res = append(res, newGeoPoly)
	}
	return res
}

func RevertItem(input *pb.Timezone) *FeatureItem {
	raw, err := json.Marshal(FromPbPolygonToGeoMultipolygon(input.Polygons))
	if err != nil {
		panic(err) // only fails if the coordinate data is not marshalable, which cannot happen
	}
	return &FeatureItem{
		Type: FeatureType,
		Properties: PropertiesDefine{
			Tzid: input.Name,
		},
		Geometry: GeometryDefine{
			Type:        MultiPolygonType,
			Coordinates: raw,
		},
	}
}

// Revert could convert pb define data to GeoJSON format.
func Revert(input *pb.Timezones) *BoundaryFile {
	output := &BoundaryFile{}
	for _, timezone := range input.Timezones {
		item := RevertItem(timezone)
		output.Features = append(output.Features, item)
	}
	output.Type = "FeatureCollection"
	return output
}

func fromGeomRingToCoords(r geom.Ring) [][2]float64 {
	coords := make([][2]float64, len(r)+1)
	for i, p := range r {
		coords[i] = [2]float64{p.X, p.Y}
	}
	coords[len(r)] = [2]float64{r[0].X, r[0].Y} // close the ring
	return coords
}

// FromGeomPolygonsToGeoMultipolygon converts a slice of geom.Polygon to
// MultiPolygonCoordinates suitable for GeoJSON serialisation.
func FromGeomPolygonsToGeoMultipolygon(polys []*geom.Polygon) MultiPolygonCoordinates {
	res := make(MultiPolygonCoordinates, 0, len(polys))
	for _, poly := range polys {
		ext := poly.Exterior()
		if len(ext) == 0 {
			continue
		}
		geoPoly := make(PolygonCoordinates, 0, 1+len(poly.Holes()))
		geoPoly = append(geoPoly, fromGeomRingToCoords(ext))
		for _, hole := range poly.Holes() {
			geoPoly = append(geoPoly, fromGeomRingToCoords(hole))
		}
		res = append(res, geoPoly)
	}
	return res
}

// RevertItemFromGeomPolygons builds a GeoJSON Feature from a timezone name and
// its already-decoded geom.Polygon slice. This avoids keeping the original
// protobuf Timezone in memory.
func RevertItemFromGeomPolygons(name string, polys []*geom.Polygon) *FeatureItem {
	raw, err := json.Marshal(FromGeomPolygonsToGeoMultipolygon(polys))
	if err != nil {
		panic(err) // unreachable: float64 coords are always marshalable
	}
	return &FeatureItem{
		Type:       FeatureType,
		Properties: PropertiesDefine{Tzid: name},
		Geometry: GeometryDefine{
			Type:        MultiPolygonType,
			Coordinates: raw,
		},
	}
}
