package polyf

import (
	"encoding/json"
	"fmt"

	"github.com/ringsaturn/tzf/internal/geom"
)

// GeoJSON geometry type constants.
const (
	multiPolygonType = "MultiPolygon"
	polygonType      = "Polygon"
	featureType      = "Feature"
)

type polygonCoords [][][2]float64
type multiPolygonCoords []polygonCoords

// geoFeature is a GeoJSON Feature with generic properties.
type geoFeature[T any] struct {
	Geometry struct {
		// Coordinates is kept as raw JSON so we can decode it into either
		// polygonCoords or multiPolygonCoords based on Type, without
		// a mapstructure intermediate step.
		Coordinates json.RawMessage `json:"coordinates"`
		Type        string          `json:"type"`
	} `json:"geometry"`
	Properties T      `json:"properties"`
	Type       string `json:"type"`
}

// BoundaryFile is a GeoJSON FeatureCollection with typed properties.
type BoundaryFile[T any] struct {
	Features []*geoFeature[T] `json:"features"`
}

// Do parses a BoundaryFile into an F[T] ready for point-in-polygon queries.
// It is a port of github.com/ringsaturn/polyf/integration/featurecollection.Do
// using encoding/json instead of mitchellh/mapstructure.
func Do[T any](input *BoundaryFile[T]) (*F[T], error) {
	f := &F[T]{}

	for _, item := range input.Features {
		var multi multiPolygonCoords

		decodeMulti := func() error {
			return json.Unmarshal(item.Geometry.Coordinates, &multi)
		}
		decodePoly := func() error {
			var poly polygonCoords
			if err := json.Unmarshal(item.Geometry.Coordinates, &poly); err != nil {
				return err
			}
			multi = multiPolygonCoords{poly}
			return nil
		}

		switch item.Type {
		case multiPolygonType:
			if err := decodeMulti(); err != nil {
				return nil, err
			}
		case polygonType:
			if err := decodePoly(); err != nil {
				return nil, err
			}
		case featureType:
			switch item.Geometry.Type {
			case multiPolygonType:
				if err := decodeMulti(); err != nil {
					return nil, err
				}
			case polygonType:
				if err := decodePoly(); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("polyf: unknown geometry type %q", item.Geometry.Type)
			}
		default:
			return nil, fmt.Errorf("polyf: unknown feature type %q", item.Type)
		}

		for _, subcoords := range multi {
			if len(subcoords) == 0 {
				continue
			}
			exterior := make([]geom.Point, 0, len(subcoords[0]))
			for _, c := range subcoords[0] {
				exterior = append(exterior, geom.Point{X: c[0], Y: c[1]})
			}
			holes := make([][]geom.Point, 0, len(subcoords)-1)
			for _, ring := range subcoords[1:] {
				hole := make([]geom.Point, 0, len(ring))
				for _, c := range ring {
					hole = append(hole, geom.Point{X: c[0], Y: c[1]})
				}
				holes = append(holes, hole)
			}
			f.Insert(geom.NewPolygon(exterior, holes), item.Properties)
		}
	}
	return f, nil
}
