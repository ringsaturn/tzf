package tzf

import (
	tzfdist "github.com/ringsaturn/tzf-dist"

	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/inplace"
)

// defaultFinder composes the FUZZY tile fast path with a polygon finder:
// GetTimezoneName answers from the preindex tile when it is unambiguous and
// falls back to point-in-polygon otherwise; GetTimezoneNames always uses the
// polygon scan. Both views come from one file, so they share a data version.
type defaultFinder struct {
	fuzzy  *fuzzyIndex
	finder *finder
}

var (
	_ F         = (*defaultFinder)(nil)
	_ GeoJSONer = (*defaultFinder)(nil)
)

// NewDefaultFinder builds the recommended general-purpose finder from the
// tzf-dist embedded lite memory image (.tzm): the FUZZY tile fast path over
// an in-place polygon view whose ring storage aliases the embedded bytes.
func NewDefaultFinder() (F, error) {
	return NewFinderFromTZM(tzfdist.LiteTZM)
}

// NewEmbeddedFinder builds the finder for embedded and memory-constrained
// targets from the tzf-dist lite .tzb file, queried in place: ~3 MB total
// (the embedded bytes plus <1 KB of heap), FUZZY-first lookups with the
// compressed-geometry scan as fallback. It carries the same dataset as the
// other pre-defined finders — only the mechanism differs.
func NewEmbeddedFinder() (F, error) {
	reader, err := embedbin.Open(tzfdist.LiteTZB)
	if err != nil {
		return nil, err
	}
	return inplace.New(reader)
}

// NewFullFinder builds the highest-fidelity finder from the tzf-dist full
// .tzb file, expanded at load time: full-precision geometry, FUZZY-first
// lookups with exact polygon fallback.
func NewFullFinder() (F, error) {
	return NewFinderFromTZB(tzfdist.FullTZB)
}

func (f *defaultFinder) GetTimezoneName(lng float64, lat float64) string {
	if res := f.fuzzy.getTimezoneName(lng, lat); res != "" {
		return res
	}
	return f.finder.GetTimezoneName(lng, lat)
}

func (f *defaultFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	return f.finder.GetTimezoneNames(lng, lat)
}

func (f *defaultFinder) TimezoneNames() []string {
	return f.finder.TimezoneNames()
}

func (f *defaultFinder) DataVersion() string {
	return f.finder.DataVersion()
}

// GetTZGeoJSON returns a GeoJSON FeatureCollection for the named timezone.
func (f *defaultFinder) GetTZGeoJSON(tzName string) ([]byte, error) {
	return f.finder.GetTZGeoJSON(tzName)
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones.
func (f *defaultFinder) GetGeoJSON() []byte {
	return f.finder.GetGeoJSON()
}
