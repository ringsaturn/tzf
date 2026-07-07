package preindex

import "github.com/ringsaturn/tzf/internal/preindexexclude"

// Preindex works for most places, except too small regions that current smallest
// tile couldn't detect.
//
// https://github.com/ringsaturn/tzf/issues/76
func excludePreIndex(lng float64, lat float64) bool {
	return preindexexclude.Match(lng, lat)
}
