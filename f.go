package tzf

import "github.com/ringsaturn/tzf/convert"

type F interface {
	GetTimezoneName(lng float64, lat float64) string
	GetTimezoneNames(lng float64, lat float64) ([]string, error)
	TimezoneNames() []string
	DataVersion() string
}

// GeoJSONer is implemented by finders that can export the boundary geometry
// they answer from. Every finder this package constructs satisfies it, but it
// is deliberately kept out of [F] so that test doubles and third-party F
// implementations need not produce geometry.
//
// Constructors return F, so reach the export methods by asserting the
// behavior rather than a concrete type:
//
//	finder, err := tzf.NewDefaultFinder()
//	// ...
//	boundaries := finder.(tzf.GeoJSONer).GetGeoJSON()
type GeoJSONer interface {
	// GetTZGeoJSON returns a FeatureCollection for one timezone name. A name
	// the dataset does not contain returns [ErrNoTimezoneFound]. The same
	// name may map to more than one item, so the collection may hold several
	// Features.
	GetTZGeoJSON(tzName string) (*convert.BoundaryFile, error)
	// GetGeoJSON returns a FeatureCollection covering all timezones.
	GetGeoJSON() *convert.BoundaryFile
}
