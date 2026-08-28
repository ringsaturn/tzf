// Package convert holds the GeoJSON boundary-file types and the pipeline's
// GeoJSON ↔ model conversions. The public GeoJSONer surface serializes
// these types to JSON at the boundary; they are not exported API.
package convert

import "encoding/json"

const (
	MultiPolygonType = "MultiPolygon"
	PolygonType      = "Polygon"
	FeatureType      = "Feature"
)

type PolygonCoordinates [][][2]float64
type MultiPolygonCoordinates []PolygonCoordinates

type GeometryDefine struct {
	Coordinates json.RawMessage `json:"coordinates"`
	Type        string          `json:"type"`
}

type PropertiesDefine struct {
	Tzid string `json:"tzid"`
}

type FeatureItem struct {
	Geometry   GeometryDefine   `json:"geometry"`
	Properties PropertiesDefine `json:"properties"`
	Type       string           `json:"type"`
}

type BoundaryFile struct {
	Type     string         `json:"type"`
	Features []*FeatureItem `json:"features"`
}

// MustMarshal serializes a boundary file for the public GeoJSONer surface.
func MustMarshal(b *BoundaryFile) []byte {
	raw, err := json.Marshal(b)
	if err != nil {
		panic(err) // unreachable: float64 coords and raw JSON always marshal
	}
	return raw
}
