// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import "time"

func init() {
	_, _ = time.LoadLocation("Asia/Shanghai")
}

type BoundaryFile struct {
	Features []struct {
		Geometry struct {
			Coordinates [][][2]float64 `json:"coordinates"`
			Type        string         `json:"type"`
		} `json:"geometry"`
		Properties struct {
			Tzid string `json:"tzid"`
		} `json:"properties"`
		Type string `json:"type"`
	} `json:"features"`
}

func GetTimezoneName(lng float64, lat float64) string {
	return ""
}

func GetTimezone(lng float64, lat float64) (*time.Location, error) {
	return time.LoadLocation(GetTimezoneName(lng, lat))
}
