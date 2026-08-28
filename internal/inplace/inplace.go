// Package inplace implements the finder that queries a TZF embedded binary
// (.tzb, E profile) where it lies, without expanding its geometry into Go
// polygon objects. It backs both tzf.NewFinderFromTZB, which reads from a
// byte slice, and x.NewFinderFromTZBReaderAt, which reads through an
// io.ReaderAt; the two differ only in how the embedbin.Reader was opened.
package inplace

import (
	"github.com/ringsaturn/tzf/v2/internal/convert"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
	"github.com/ringsaturn/tzf/v2/internal/tzerr"
)

// Finder answers queries directly over a validated embedbin.Reader. It
// satisfies tzf.F and tzf.GeoJSONer.
type Finder struct {
	reader         *embedbin.Reader
	names          []string
	lookupCapacity int
	fuzzy          bool
}

// New builds a Finder over an opened E-profile reader. M-profile files carry
// no chunked point streams, so they are rejected here: use tzf.NewFinderFromTZM.
func New(reader *embedbin.Reader) (*Finder, error) {
	if reader.ProfileM() {
		return nil, embedbin.ErrProfile
	}
	names := make([]string, reader.TimezoneCount())
	for i := range names {
		name, err := reader.Name(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
	}
	return &Finder{
		reader: reader, names: names, lookupCapacity: reader.LookupBufferSize(),
		fuzzy: reader.HasFuzzy(),
	}, nil
}

// GetTimezoneName returns the first matching timezone in source order.
//
// The tzf.F interface cannot expose a lazy structural read error. Such an
// error is treated as no match. Open-time validation and the CRC catch
// ordinary file corruption before queries begin.
func (f *Finder) GetTimezoneName(lng, lat float64) string {
	if f.fuzzy {
		if idx, ok, err := f.reader.FuzzyLookup(lng, lat); err == nil && ok {
			return f.names[idx]
		}
	}
	idx, ok, err := f.reader.Lookup(lng, lat)
	if err != nil || !ok {
		return ""
	}
	return f.names[idx]
}

func (f *Finder) GetTimezoneNames(lng, lat float64) ([]string, error) {
	indices, err := f.reader.LookupInto(lng, lat, make([]int32, 0, f.lookupCapacity))
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		return nil, nil
	}
	names := make([]string, len(indices))
	for i, idx := range indices {
		names[i] = f.names[idx]
	}
	return names, nil
}

func (f *Finder) TimezoneNames() []string {
	return f.names
}

func (f *Finder) DataVersion() string {
	return f.reader.DataVersion()
}

// GetTZGeoJSON returns a GeoJSON FeatureCollection for the named timezone.
// Unlike the expanded finders, which export polygons they already hold, this
// one decodes the timezone's rings from the file on each call — allocating,
// but only for the requested timezone.
func (f *Finder) GetTZGeoJSON(tzName string) ([]byte, error) {
	var features []*convert.FeatureItem
	for i, name := range f.names {
		if name != tzName {
			continue
		}
		item, err := f.feature(int32(i))
		if err != nil {
			return nil, err
		}
		features = append(features, item)
	}
	if len(features) == 0 {
		return nil, tzerr.ErrNoTimezoneFound
	}
	return convert.MustMarshal(&convert.BoundaryFile{Type: "FeatureCollection", Features: features}), nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones,
// decoding the whole file's geometry — roughly the cost of loading an
// expanded finder, and not a query-path operation.
//
// The signature cannot report a lazy structural read error, so a timezone
// that fails to decode is omitted, the same way GetTimezoneName treats such
// an error as no match. Use GetTZGeoJSON when the error matters.
func (f *Finder) GetGeoJSON() []byte {
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for i := range f.names {
		item, err := f.feature(int32(i))
		if err != nil {
			continue
		}
		output.Features = append(output.Features, item)
	}
	return convert.MustMarshal(output)
}

// feature decodes one timezone's geometry into a GeoJSON Feature. Ring vertex
// lists match the expanded loader's, junction duplicates included: both go
// through embedbin ring expansion.
func (f *Finder) feature(idx int32) (*convert.FeatureItem, error) {
	expanded, err := f.reader.ExpandTimezone(idx)
	if err != nil {
		return nil, err
	}
	polys := make([]geom.Poly, len(expanded))
	for i, poly := range expanded {
		polys[i] = geom.NewI32Polygon(poly.Exterior, poly.Holes)
	}
	return convert.RevertItemFromGeomPolygons(f.names[idx], polys), nil
}
