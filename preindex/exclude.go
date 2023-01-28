package preindex

import (
	_ "embed"
	"encoding/json"

	"github.com/ringsaturn/polyf"
	"github.com/ringsaturn/polyf/integration/featurecollection"
)

var (
	excludesFinder *polyf.F[any]

	//go:embed exclude.geojson
	exludeGeoJSONBytes []byte
)

func init() {
	boundaryFile := &featurecollection.BoundaryFile[any]{}
	err := json.Unmarshal(exludeGeoJSONBytes, boundaryFile)
	if err != nil {
		panic(err)
	}
	excludesFinder, err = featurecollection.Do(boundaryFile)
	if err != nil {
		panic(err)
	}
}

// Preindex works for most places, except too small regions that current smallest
// tile couldn't detect.
//
// https://github.com/ringsaturn/tzf/issues/76
func excludePreIndex(lng float64, lat float64) bool {
	_, err := excludesFinder.FindOne(lng, lat)
	return err == nil
}
