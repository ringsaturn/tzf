package preindexexclude

import (
	_ "embed"
	"encoding/json"

	"github.com/ringsaturn/tzf/internal/polyf"
)

var (
	excludesFinder *polyf.F[any]

	//go:embed exclude.geojson
	excludeGeoJSONBytes []byte
)

func init() {
	boundaryFile := &polyf.BoundaryFile[any]{}
	if err := json.Unmarshal(excludeGeoJSONBytes, boundaryFile); err != nil {
		panic(err)
	}
	var err error
	excludesFinder, err = polyf.Do(boundaryFile)
	if err != nil {
		panic(err)
	}
}

// Match reports whether a coordinate should bypass preindex results.
//
// Preindex works for most places, except too small regions that current smallest
// tile couldn't detect.
//
// https://github.com/ringsaturn/tzf/issues/76
func Match(lng float64, lat float64) bool {
	_, err := excludesFinder.FindOne(lng, lat)
	return err == nil
}
