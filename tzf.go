// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"errors"
	"math"
	"slices"

	tzfdist "github.com/ringsaturn/tzf-dist"
	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
	"github.com/ringsaturn/tzf/internal/gridindex"
	"github.com/ringsaturn/tzf/internal/polyline"
	"github.com/ringsaturn/tzf/reduce"
	"google.golang.org/protobuf/proto"
)

var ErrNoTimezoneFound = errors.New("tzf: no timezone found")

type Option struct {
	DropPBTZ bool
}

type OptionFunc = func(opt *Option)

// SetDropPBTZ will make Finder not save [github.com/ringsaturn/tzf/pb.Timezone] in memory
func SetDropPBTZ(opt *Option) {
	opt.DropPBTZ = true
}

// tzitem holds one timezone's polygons. T is the coordinate storage type:
// float64 for degree-space data (NewFinderFromPB), int32 for 1e5-scaled
// compressed topology data (NewFinderFromCompressedTopo).
type tzitem[T geom.Coord] struct {
	pbtz  *pb.Timezone
	name  string
	polys []*geom.PolygonOf[T]
	min   [2]float64
	max   [2]float64
}

func (i *tzitem[T]) ContainsPoint(p geom.Point) bool {
	for _, poly := range i.polys {
		if poly.ContainsPoint(p) {
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

// finderCore is the coordinate-storage-generic part of Finder. Keeping the
// type parameter behind this unexported interface lets Finder stay a plain
// exported struct with an unchanged API: queries pay one interface dispatch
// here, and every call below it is concrete.
type finderCore interface {
	getTimezoneName(lng float64, lat float64) string
	getTimezoneNames(lng float64, lat float64) []string
	revertFeatures(tzName string, all bool) []*convert.FeatureItem
}

// finderImpl carries the polygon items of one storage type.
type finderImpl[T geom.Coord] struct {
	items []*tzitem[T]
	// grid maps (floor(lng), floor(lat)) → candidate item indices.
	// Populated automatically when loading CompressedTopoTimezones that
	// contains an embedded GridIndex.
	grid map[[2]int16][]int32
}

// Finder is based on point-in-polygon search algo.
//
// Memory will use about 100MB if lite data and 1G if full data.
// Performance is very stable and very accuate.
type Finder struct {
	core    finderCore
	names   []string
	reduced bool
	opt     *Option
	version string
}

func NewFinderFromRawJSON(input *convert.BoundaryFile, opts ...OptionFunc) (F, error) {
	timezones, err := convert.Do(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(timezones, opts...)
}

func NewFinderFromPB(input *pb.Timezones, opts ...OptionFunc) (F, error) {
	items := make([]*tzitem[float64], 0)
	names := make([]string, 0)

	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}

	for _, timezone := range input.Timezones {
		names = append(names, timezone.Name)

		newItem := &tzitem[float64]{
			name: timezone.Name,
		}
		if !opt.DropPBTZ {
			newItem.pbtz = timezone
		}
		for _, polygon := range timezone.Polygons {
			newPoints := make([]geom.Point, 0, len(polygon.Points))
			for _, point := range polygon.Points {
				newPoints = append(newPoints, geom.Point{X: float64(point.Lng), Y: float64(point.Lat)})
			}

			holes := make([][]geom.Point, 0, len(polygon.Holes))
			for _, holePoly := range polygon.Holes {
				newHolePoints := make([]geom.Point, 0, len(holePoly.Points))
				for _, point := range holePoly.Points {
					newHolePoints = append(newHolePoints, geom.Point{X: float64(point.Lng), Y: float64(point.Lat)})
				}
				holes = append(holes, newHolePoints)
			}

			newItem.polys = append(newItem.polys, geom.NewPolygon(newPoints, holes))
		}
		minp, maxp := newItem.getMinMax()

		newItem.min = minp
		newItem.max = maxp

		items = append(items, newItem)
	}
	return &Finder{
		core:    &finderImpl[float64]{items: items},
		names:   names,
		reduced: input.Reduced,
		opt:     opt,
		version: input.Version,
	}, nil
}

// Deprecated: use NewFinderFromCompressedTopo instead and update data source.
func NewFinderFromCompressed(input *pb.CompressedTimezones, opts ...OptionFunc) (F, error) {
	tzs, err := reduce.Decompress(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(tzs, opts...)
}

// NewFullFinder builds a [DefaultFinder] from the tzf-dist embedded data with no
// parameters required. It combines a [FuzzyFinder] (topology.preindex) with a
// [Finder] (topology.compress.topo) for fast lookups with accurate fallback.
func NewFullFinder() (F, error) {
	preindex := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfdist.PreindexData, preindex); err != nil {
		return nil, err
	}
	topo := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.CompressTopoData, topo); err != nil {
		return nil, err
	}
	return newDefaultFinderFromCompressedTopo(preindex, topo)
}

// NewFinderFromCompressedTopo builds a [Finder] directly from a [pb.CompressedTopoTimezones]
// input without materialising an intermediate [pb.Timezones], reducing peak memory usage.
//
// It accepts both the full-precision combined-with-oceans.compress.topo.bin and the
// topology-aware simplified combined-with-oceans.topology.compress.topo.bin from tzf-dist.
func NewFinderFromCompressedTopo(input *pb.CompressedTopoTimezones, opts ...OptionFunc) (F, error) {
	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}

	// Decompress shared edges once into scaled-integer point slices indexed by
	// edge ID. Coordinates stay on the 1e5 polyline grid (geom.I32Point), which
	// halves ring storage compared to float64 points.
	edges := make([][]geom.I32Point, len(input.SharedEdges))
	for _, e := range input.SharedEdges {
		pts, err := decodePolylineToI32Points(e.Points)
		if err != nil {
			return nil, err
		}
		edges[e.Id] = pts
	}

	items := make([]*tzitem[int32], 0, len(input.Timezones))
	names := make([]string, 0, len(input.Timezones))

	for _, tz := range input.Timezones {
		names = append(names, tz.Name)
		newItem := &tzitem[int32]{name: tz.Name}

		for _, poly := range tz.Polygons {
			exterior, err := expandCompressedRing(poly.Exterior, edges)
			if err != nil {
				return nil, err
			}
			holes := make([][]geom.I32Point, 0, len(poly.Holes))
			for _, hole := range poly.Holes {
				h, err := expandCompressedRing(hole.Exterior, edges)
				if err != nil {
					return nil, err
				}
				holes = append(holes, h)
			}
			newItem.polys = append(newItem.polys, geom.NewI32Polygon(exterior, holes))
		}

		minp, maxp := newItem.getMinMax()
		newItem.min = minp
		newItem.max = maxp
		items = append(items, newItem)
	}

	core := &finderImpl[int32]{items: items}
	if input.GridIndex != nil {
		core.grid = gridindex.DecodeToMap(input.GridIndex)
	}
	return &Finder{
		core:    core,
		names:   names,
		opt:     opt,
		version: input.Version,
	}, nil
}

// decodePolylineToI32Points decodes polyline bytes into 1e5-scaled integer
// points, skipping the float32 protobuf representation entirely.
func decodePolylineToI32Points(b []byte) ([]geom.I32Point, error) {
	coords, err := polyline.DecodeCoordsInt32(b)
	if err != nil {
		return nil, err
	}
	pts := make([]geom.I32Point, len(coords))
	for i, c := range coords {
		pts[i] = geom.I32Point{X: c[0], Y: c[1]}
	}
	return pts, nil
}

// expandCompressedRing expands a compressed ring's segments into a flat
// scaled-integer point slice, resolving edge-forward/reversed references
// against the pre-decoded edge table.
func expandCompressedRing(segs []*pb.CompressedRingSegment, edges [][]geom.I32Point) ([]geom.I32Point, error) {
	var pts []geom.I32Point
	for _, seg := range segs {
		switch s := seg.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			inline, err := decodePolylineToI32Points(s.Inline.Points)
			if err != nil {
				return nil, err
			}
			pts = append(pts, inline...)
		case *pb.CompressedRingSegment_EdgeForward:
			pts = append(pts, edges[s.EdgeForward]...)
		case *pb.CompressedRingSegment_EdgeReversed:
			edge := edges[s.EdgeReversed]
			for i := len(edge) - 1; i >= 0; i-- {
				pts = append(pts, edge[i])
			}
		}
	}
	return pts, nil
}

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
func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	return f.core.getTimezoneName(lng, lat)
}

func (f *Finder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	return f.core.getTimezoneNames(lng, lat), nil
}

func (f *Finder) TimezoneNames() []string {
	return f.names
}

func (f *Finder) DataVersion() string {
	return f.version
}

// GetTZGeoJSON returns a GeoJSON FeatureCollection for the named timezone.
// The same timezone name may map to more than one item in the dataset, so the
// result is a FeatureCollection that may contain multiple Features.
func (f *Finder) GetTZGeoJSON(tzName string) (*convert.BoundaryFile, error) {
	features := f.core.revertFeatures(tzName, false)
	if len(features) == 0 {
		return nil, ErrNoTimezoneFound
	}
	return &convert.BoundaryFile{Type: "FeatureCollection", Features: features}, nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones.
func (f *Finder) GetGeoJSON() *convert.BoundaryFile {
	return &convert.BoundaryFile{Type: "FeatureCollection", Features: f.core.revertFeatures("", true)}
}
