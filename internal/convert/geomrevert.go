package convert

import (
	"encoding/json"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

func fromGeomRingToCoords(r geom.Ring) [][2]float64 {
	coords := make([][2]float64, len(r)+1)
	for i, p := range r {
		coords[i] = [2]float64{p.X, p.Y}
	}
	coords[len(r)] = [2]float64{r[0].X, r[0].Y} // close the ring
	return coords
}

// fromGeomPolygonsToGeoMultipolygon converts a slice of geom.Polygon to
// MultiPolygonCoordinates suitable for GeoJSON serialisation.
func fromGeomPolygonsToGeoMultipolygon(polys []geom.Poly) MultiPolygonCoordinates {
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
// its already-decoded geom.Polygon slice.
func RevertItemFromGeomPolygons(name string, polys []geom.Poly) *FeatureItem {
	raw, err := json.Marshal(fromGeomPolygonsToGeoMultipolygon(polys))
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
