package main

import (
	"encoding/json"
	"fmt"
	"os"

	gocitiesjson "github.com/ringsaturn/go-cities.json"
	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

var (
	defaultfinder tzf.F
	fullFinder    tzf.F
)

func init() {
	initLite()
	initFull()
}

func initLite() {
	_finder, _ := tzf.NewDefaultFinder()
	defaultfinder = _finder
}

func initFull() {
	input := &pb.Timezones{}
	if err := proto.Unmarshal(tzfrel.FullData, input); err != nil {
		panic(err)
	}
	_finder, _ := tzf.NewFinderFromPB(input)
	fullFinder = _finder
}

type FeatureCollection struct {
	Type     string     `json:"type"` // FeatureCollection
	Features []Features `json:"features"`
}

type Features struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Geometry   Geometry               `json:"geometry"`
}

type Geometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

func main() {
	notEqualData := &FeatureCollection{
		Type:     "FeatureCollection",
		Features: make([]Features, 0),
	}
	for lng := -180; lng <= 180; lng++ {
		for lat := -90; lat <= 90; lat++ {
			_lng := float64(lng)
			_lat := float64(lat)
			fullRes := fullFinder.GetTimezoneName(_lng, _lat)
			defaultRes := defaultfinder.GetTimezoneName(_lng, _lat)
			if fullRes == defaultRes {
				continue
			}
			notEqualData.Features = append(notEqualData.Features, Features{
				Type: "Feature",
				Properties: map[string]interface{}{
					"lite": defaultRes,
					"full": fullRes,
				},
				Geometry: Geometry{
					Type:        "Point",
					Coordinates: []float64{_lng, _lat},
				},
			})
		}
	}
	for _, city := range gocitiesjson.Cities {
		_lng := city.Lng
		_lat := city.Lat
		fullRes := fullFinder.GetTimezoneName(_lng, _lat)
		defaultRes := defaultfinder.GetTimezoneName(_lng, _lat)
		if fullRes == defaultRes {
			continue
		}
		notEqualData.Features = append(notEqualData.Features, Features{
			Type: "Feature",
			Properties: map[string]interface{}{
				"lite":      defaultRes,
				"full":      fullRes,
				"worldCity": fmt.Sprintf("%v-%v", city.Country, city.Name),
			},
			Geometry: Geometry{
				Type:        "Point",
				Coordinates: []float64{_lng, _lat},
			},
		})
	}

	file, _ := json.Marshal(notEqualData)

	_ = os.WriteFile("points_not_equal.geojson", file, 0644)
}
