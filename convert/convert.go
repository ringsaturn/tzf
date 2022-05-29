package convert

import (
	"log"

	"github.com/mitchellh/mapstructure"
	"github.com/ringsaturn/tzf/pb"
)

const (
	MultiPolygonType = "MultiPolygon"
	PolygonType      = "Polygon"
	FeatureType      = "Feature"
)

type PolygonCoordinates [][][2]float64
type MultiPolygonCoordinates []PolygonCoordinates

type FeatureItem struct {
	Geometry struct {
		Coordinates interface{} `json:"coordinates"`
		Type        string      `json:"type"`
	} `json:"geometry"`
	Properties struct {
		Tzid string `json:"tzid"`
	} `json:"properties"`
	Type string `json:"type"` // Polygon
}

type BoundaryFile struct {
	Features []*FeatureItem `json:"features"`
}

func Do(input *BoundaryFile) []*pb.Timezone {
	output := make([]*pb.Timezone, 0)

	for _, item := range input.Features {
		pbtzItem := &pb.Timezone{
			Name: item.Properties.Tzid,
		}

		var coordinates MultiPolygonCoordinates

		MultiPolygonTypeHandler := func() {
			if err := mapstructure.Decode(item.Geometry.Coordinates, &coordinates); err != nil {
				panic(err)
			}
		}
		PolygonTypeHandler := func() {
			var polygonCoordinates PolygonCoordinates
			if err := mapstructure.Decode(item.Geometry.Coordinates, &polygonCoordinates); err != nil {
				panic(err)
			}
			coordinates = append(coordinates, polygonCoordinates)
		}

		switch item.Type {
		case MultiPolygonType:
			MultiPolygonTypeHandler()
		case PolygonType:
			PolygonTypeHandler()
		case FeatureType:
			switch item.Geometry.Type {
			case MultiPolygonType:
				MultiPolygonTypeHandler()
			case PolygonType:
				PolygonTypeHandler()
			default:
				log.Panicf("unknown type %v", item.Type)
			}
		default:
			log.Panicf("unknown type %v", item.Type)
		}

		polygons := make([]*pb.Polygon, 0)

		for _, subcoordinates := range coordinates {
			for _, geoPoly := range subcoordinates {
				newpbPoly := &pb.Polygon{
					Points: make([]*pb.Point, 0),
				}
				for _, rawCoords := range geoPoly {
					newpbPoly.Points = append(newpbPoly.Points, &pb.Point{
						Lng: float32(rawCoords[0]),
						Lat: float32(rawCoords[1]),
					})
				}
				polygons = append(polygons, newpbPoly)
			}
		}

		pbtzItem.Polygons = polygons
		output = append(output, pbtzItem)
	}

	return output
}
