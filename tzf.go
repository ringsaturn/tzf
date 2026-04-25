// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"errors"
	"slices"

	tzfdist "github.com/ringsaturn/tzf-dist"
	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
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

type tzitem struct {
	pbtz  *pb.Timezone
	name  string
	polys []*geom.Polygon
	min   [2]float64
	max   [2]float64
}

func (i *tzitem) ContainsPoint(p geom.Point) bool {
	for _, poly := range i.polys {
		if poly.ContainsPoint(p) {
			return true
		}
	}
	return false
}

func (i *tzitem) getMinMax() ([2]float64, [2]float64) {
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

// Finder is based on point-in-polygon search algo.
//
// Memory will use about 100MB if lite data and 1G if full data.
// Performance is very stable and very accuate.
type Finder struct {
	items   []*tzitem
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
	items := make([]*tzitem, 0)
	names := make([]string, 0)

	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}

	for _, timezone := range input.Timezones {
		names = append(names, timezone.Name)

		newItem := &tzitem{
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
	finder := &Finder{}
	finder.items = items
	finder.names = names
	finder.reduced = input.Reduced
	finder.opt = opt
	finder.version = input.Version
	return finder, nil
}

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

	// Decompress shared edges once into geom.Point slices indexed by edge ID.
	// This replaces the two intermediate proto objects (TopoTimezones + pb.Timezones).
	edges := make([][]geom.Point, len(input.SharedEdges))
	for _, e := range input.SharedEdges {
		raw := reduce.DecompressedPolylineBytesToPoints(e.Points)
		pts := make([]geom.Point, len(raw))
		for j, p := range raw {
			pts[j] = geom.Point{X: float64(p.Lng), Y: float64(p.Lat)}
		}
		edges[e.Id] = pts
	}

	items := make([]*tzitem, 0, len(input.Timezones))
	names := make([]string, 0, len(input.Timezones))

	for _, tz := range input.Timezones {
		names = append(names, tz.Name)
		newItem := &tzitem{name: tz.Name}

		for _, poly := range tz.Polygons {
			exterior := expandCompressedRing(poly.Exterior, edges)
			holes := make([][]geom.Point, 0, len(poly.Holes))
			for _, hole := range poly.Holes {
				holes = append(holes, expandCompressedRing(hole.Exterior, edges))
			}
			newItem.polys = append(newItem.polys, geom.NewPolygon(exterior, holes))
		}

		minp, maxp := newItem.getMinMax()
		newItem.min = minp
		newItem.max = maxp
		items = append(items, newItem)
	}

	return &Finder{
		items:   items,
		names:   names,
		opt:     opt,
		version: input.Version,
	}, nil
}

// expandCompressedRing expands a compressed ring's segments into a flat geom.Point
// slice, resolving edge-forward/reversed references against the pre-decoded edge table.
func expandCompressedRing(segs []*pb.CompressedRingSegment, edges [][]geom.Point) []geom.Point {
	var pts []geom.Point
	for _, seg := range segs {
		switch s := seg.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			for _, p := range reduce.DecompressedPolylineBytesToPoints(s.Inline.Points) {
				pts = append(pts, geom.Point{X: float64(p.Lng), Y: float64(p.Lat)})
			}
		case *pb.CompressedRingSegment_EdgeForward:
			pts = append(pts, edges[s.EdgeForward]...)
		case *pb.CompressedRingSegment_EdgeReversed:
			edge := edges[s.EdgeReversed]
			for i := len(edge) - 1; i >= 0; i-- {
				pts = append(pts, edge[i])
			}
		}
	}
	return pts
}

// GetTimezoneName will use alphabet order and return first matched result.
func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	p := geom.Point{X: lng, Y: lat}
	for _, item := range f.items {
		if item.ContainsPoint(p) {
			return item.name
		}
	}
	return ""
}

func (f *Finder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	p := geom.Point{X: lng, Y: lat}
	res := []string{}
	for i := range f.items {
		if f.items[i].ContainsPoint(p) {
			res = append(res, f.items[i].name)
		}
	}
	slices.Sort(res)
	return res, nil
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
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for _, item := range f.items {
		if item.name == tzName {
			output.Features = append(output.Features, convert.RevertItemFromGeomPolygons(item.name, item.polys))
		}
	}
	if len(output.Features) == 0 {
		return nil, ErrNoTimezoneFound
	}
	return output, nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones.
func (f *Finder) GetGeoJSON() *convert.BoundaryFile {
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for _, item := range f.items {
		output.Features = append(output.Features, convert.RevertItemFromGeomPolygons(item.name, item.polys))
	}
	return output
}
