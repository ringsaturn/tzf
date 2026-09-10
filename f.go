package tzf

type F interface {
	GetTimezoneName(lng float64, lat float64) string
	GetTimezoneNames(lng float64, lat float64) ([]string, error)
	TimezoneNames() []string
	DataVersion() string
}

// GeoJSONer is implemented by finders that can export the geometry they
// answer from: the boundary polygons and the FUZZY preindex tiles the fast
// path uses. Every finder this package constructs satisfies it, but it is
// deliberately kept out of [F] so that test doubles and third-party F
// implementations need not produce geometry.
//
// Constructors return F, so reach the export methods by asserting the
// behavior rather than a concrete type:
//
//	finder, err := tzf.NewDefaultFinder()
//	// ...
//	boundaries := finder.(tzf.GeoJSONer).GetGeoJSON()
type GeoJSONer interface {
	// GetTZGeoJSON returns a serialized GeoJSON FeatureCollection for one
	// timezone name. A name the dataset does not contain returns
	// [ErrNoTimezoneFound]. The same name may map to more than one item, so
	// the collection may hold several Features.
	GetTZGeoJSON(tzName string) ([]byte, error)
	// GetGeoJSON returns a serialized GeoJSON FeatureCollection covering
	// all timezones.
	GetGeoJSON() []byte
	// GetTZPreindexGeoJSON returns a serialized GeoJSON FeatureCollection
	// holding one Feature whose MultiPolygon carries the bounding rectangle
	// of every FUZZY preindex tile naming the timezone — the area where
	// GetTimezoneName answers without falling back to exact
	// point-in-polygon — coarsest zoom first. A name the dataset does not
	// contain, or one no tile names, returns [ErrNoTimezoneFound]; a file
	// without a FUZZY section returns [ErrNoFuzzySection].
	GetTZPreindexGeoJSON(tzName string) ([]byte, error)
	// GetPreindexGeoJSON returns a serialized GeoJSON FeatureCollection
	// covering the whole FUZZY preindex: one Feature per timezone that owns
	// at least one tile, in dataset order; a boundary tile appears in every
	// timezone it names. A file without a FUZZY section returns
	// [ErrNoFuzzySection].
	GetPreindexGeoJSON() ([]byte, error)
}
