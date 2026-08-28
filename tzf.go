// Package tzf converts (lng, lat) coordinates to timezone names.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"math"
	"slices"

	"github.com/ringsaturn/tzf/v2/internal/convert"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
	"github.com/ringsaturn/tzf/v2/internal/tzerr"
)

// ErrNoTimezoneFound is returned when no timezone covers the query point, or
// when a GeoJSON export names a timezone the dataset does not contain.
var ErrNoTimezoneFound = tzerr.ErrNoTimezoneFound

// tzitem holds one timezone's polygons. T is the coordinate storage type;
// every loader in v2 produces 1e5-scaled int32 storage.
type tzitem[T geom.Coord] struct {
	name  string
	polys []*geom.PolygonOf[T]
	min   [2]float64
	max   [2]float64
}

func (i *tzitem[T]) ContainsPoint(p geom.Point) bool {
	for _, poly := range i.polys {
		// Timezone polygons tile the globe, so a query that lands exactly on a
		// shared border must belong to both neighbours rather than to neither.
		// The nautical zones make this easy to hit: their borders sit on whole
		// meridians (7.5, 22.5, ...), which is exactly the kind of coordinate
		// people type by hand.
		if poly.ContainsPointAllowOnEdge(p) {
			return true
		}
	}
	return false
}

func (i *tzitem[T]) getMinMax() ([2]float64, [2]float64) {
	r0 := i.polys[0].Rect()
	retmin := [2]float64{r0.Min.X, r0.Min.Y}
	retmax := [2]float64{r0.Max.X, r0.Max.Y}

	for _, poly := range i.polys[1:] {
		r := poly.Rect()
		if r.Min.X < retmin[0] {
			retmin[0] = r.Min.X
		}
		if r.Min.Y < retmin[1] {
			retmin[1] = r.Min.Y
		}
		if r.Max.X > retmax[0] {
			retmax[0] = r.Max.X
		}
		if r.Max.Y > retmax[1] {
			retmax[1] = r.Max.Y
		}
	}
	return retmin, retmax
}

// finderCore is the coordinate-storage-generic part of finder. Keeping the
// type parameter behind this unexported interface lets finder stay a plain
// struct: queries pay one interface dispatch here, and every call below it
// is concrete.
type finderCore interface {
	getTimezoneName(lng float64, lat float64) string
	getTimezoneNames(lng float64, lat float64) []string
	revertFeatures(tzName string, all bool) []*convert.FeatureItem
}

// finderImpl carries the polygon items of one storage type.
type finderImpl[T geom.Coord] struct {
	items []*tzitem[T]
	// grid maps (floor(lng), floor(lat)) → candidate item indices,
	// materialized by the .tzb expansion loader from the GRID section.
	grid map[[2]int16][]int32
	// dense queries a .tzm file's GRID section in place instead of a
	// materialized map, keeping open-time heap low (spec rev 1 §6.5). At
	// most one of grid and dense is set; both carry the same content and
	// produce identical results.
	dense *embedbin.DenseGrid
}

// finder is the point-in-polygon mechanism behind the public constructors.
type finder struct {
	core    finderCore
	names   []string
	version string
}

var (
	_ F         = (*finder)(nil)
	_ GeoJSONer = (*finder)(nil)
)

// gridCandidates returns the candidate timezone indices for a given coordinate.
// The second return value reports whether the grid is loaded; when false the
// caller should fall back to a linear scan. When true but the slice is empty,
// no timezone covers the point and the caller should return early.
func (c *finderImpl[T]) gridCandidates(lng float64, lat float64) ([]int32, bool) {
	if c.grid == nil {
		return nil, false
	}
	key := [2]int16{int16(math.Floor(lng)), int16(math.Floor(lat))}
	return c.grid[key], true
}

func (c *finderImpl[T]) getTimezoneName(lng float64, lat float64) string {
	if c.dense != nil {
		off, count := c.dense.CellRange(lng, lat)
		if count == 0 {
			return ""
		}
		// Same single-candidate short-circuit as the map-backed grid below.
		if count == 1 && lng > -179 && lng < 179 && lat > -89 && lat < 89 {
			return c.items[c.dense.Candidate(off)].name
		}
		p := geom.Point{X: lng, Y: lat}
		for i := range count {
			idx := c.dense.Candidate(off + i)
			if c.items[idx].ContainsPoint(p) {
				return c.items[idx].name
			}
		}
		return ""
	}
	if candidates, ok := c.gridCandidates(lng, lat); ok {
		// Single-candidate short-circuit: skip PIP when there is only one
		// candidate and we are away from the antimeridian / pole edges.
		if len(candidates) == 1 && lng > -179 && lng < 179 && lat > -89 && lat < 89 {
			return c.items[candidates[0]].name
		}
		p := geom.Point{X: lng, Y: lat}
		for _, idx := range candidates {
			if c.items[idx].ContainsPoint(p) {
				return c.items[idx].name
			}
		}
		return ""
	}
	p := geom.Point{X: lng, Y: lat}
	for _, item := range c.items {
		if item.ContainsPoint(p) {
			return item.name
		}
	}
	return ""
}

func (c *finderImpl[T]) getTimezoneNames(lng float64, lat float64) []string {
	p := geom.Point{X: lng, Y: lat}
	var res []string

	if c.dense != nil {
		off, count := c.dense.CellRange(lng, lat)
		for i := range count {
			idx := c.dense.Candidate(off + i)
			if c.items[idx].ContainsPoint(p) {
				res = append(res, c.items[idx].name)
			}
		}
		slices.Sort(res)
		return res
	}
	if candidates, ok := c.gridCandidates(lng, lat); ok {
		for _, idx := range candidates {
			if c.items[idx].ContainsPoint(p) {
				res = append(res, c.items[idx].name)
			}
		}
	} else {
		for _, item := range c.items {
			if item.ContainsPoint(p) {
				res = append(res, item.name)
			}
		}
	}

	slices.Sort(res)
	return res
}

// revertFeatures builds GeoJSON Features for items matching tzName, or for
// all items when all is true.
func (c *finderImpl[T]) revertFeatures(tzName string, all bool) []*convert.FeatureItem {
	var out []*convert.FeatureItem
	for _, item := range c.items {
		if all || item.name == tzName {
			polys := make([]geom.Poly, len(item.polys))
			for i, p := range item.polys {
				polys[i] = p
			}
			out = append(out, convert.RevertItemFromGeomPolygons(item.name, polys))
		}
	}
	return out
}

// GetTimezoneName will use alphabet order and return first matched result.
func (f *finder) GetTimezoneName(lng float64, lat float64) string {
	return f.core.getTimezoneName(lng, lat)
}

func (f *finder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	return f.core.getTimezoneNames(lng, lat), nil
}

func (f *finder) TimezoneNames() []string {
	return f.names
}

func (f *finder) DataVersion() string {
	return f.version
}

// GetTZGeoJSON returns a GeoJSON FeatureCollection for the named timezone.
// The same timezone name may map to more than one item in the dataset, so the
// result is a FeatureCollection that may contain multiple Features.
func (f *finder) GetTZGeoJSON(tzName string) ([]byte, error) {
	features := f.core.revertFeatures(tzName, false)
	if len(features) == 0 {
		return nil, ErrNoTimezoneFound
	}
	return convert.MustMarshal(&convert.BoundaryFile{Type: "FeatureCollection", Features: features}), nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones.
func (f *finder) GetGeoJSON() []byte {
	return convert.MustMarshal(&convert.BoundaryFile{Type: "FeatureCollection", Features: f.core.revertFeatures("", true)})
}

// GetTZPreindexGeoJSON implements [GeoJSONer]. The plain expanded finder is
// only constructed for files without a FUZZY section — [NewFinderFromTZB]
// wraps it in the preindex composition otherwise — so there is nothing to
// export.
func (f *finder) GetTZPreindexGeoJSON(string) ([]byte, error) {
	return nil, ErrNoFuzzySection
}

// GetPreindexGeoJSON implements [GeoJSONer]; see [finder.GetTZPreindexGeoJSON].
func (f *finder) GetPreindexGeoJSON() ([]byte, error) {
	return nil, ErrNoFuzzySection
}
