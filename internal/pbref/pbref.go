// Package pbref builds protobuf-backed reference finders for the parity
// harnesses. It reproduces the v1 query semantics (post-#216 boundary rules,
// grid candidates with the single-candidate shortcut, alphabet-order first
// match) over pb.CompressedTopoTimezones and pb.PreindexTimezones, so the
// pb-free runtime can be validated against the pipeline's source data
// without the runtime module carrying any pb code.
package pbref

import (
	"math"
	"slices"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	"github.com/ringsaturn/tzf/v2/internal/gridindex"
	"github.com/ringsaturn/tzf/v2/internal/maps"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/polyline"
	"github.com/ringsaturn/tzf/v2/internal/tzerr"
)

type item struct {
	name  string
	polys []*geom.I32Polygon
}

func (i *item) containsPoint(p geom.Point) bool {
	for _, poly := range i.polys {
		if poly.ContainsPointAllowOnEdge(p) {
			return true
		}
	}
	return false
}

// Finder is the polygon reference: the deleted v1
// NewFinderFromCompressedTopo semantics, including the embedded GridIndex
// (with its single-candidate shortcut) when the input carries one.
type Finder struct {
	items   []*item
	grid    map[[2]int16][]int32
	names   []string
	version string
}

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

// New builds the reference finder from a compressed-topology dataset.
func New(input *pb.CompressedTopoTimezones) (*Finder, error) {
	edges := make([][]geom.I32Point, len(input.SharedEdges))
	for _, e := range input.SharedEdges {
		pts, err := decodePolylineToI32Points(e.Points)
		if err != nil {
			return nil, err
		}
		edges[e.Id] = pts
	}

	f := &Finder{version: input.Version}
	for _, tz := range input.Timezones {
		f.names = append(f.names, tz.Name)
		it := &item{name: tz.Name}
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
			it.polys = append(it.polys, geom.NewI32Polygon(exterior, holes))
		}
		f.items = append(f.items, it)
	}
	if input.GridIndex != nil {
		f.grid = gridindex.DecodeToMap(input.GridIndex)
	}
	return f, nil
}

func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	if f.grid != nil {
		key := [2]int16{int16(math.Floor(lng)), int16(math.Floor(lat))}
		candidates := f.grid[key]
		if len(candidates) == 1 && lng > -179 && lng < 179 && lat > -89 && lat < 89 {
			return f.items[candidates[0]].name
		}
		p := geom.Point{X: lng, Y: lat}
		for _, idx := range candidates {
			if f.items[idx].containsPoint(p) {
				return f.items[idx].name
			}
		}
		return ""
	}
	p := geom.Point{X: lng, Y: lat}
	for _, it := range f.items {
		if it.containsPoint(p) {
			return it.name
		}
	}
	return ""
}

func (f *Finder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	p := geom.Point{X: lng, Y: lat}
	var res []string
	if f.grid != nil {
		key := [2]int16{int16(math.Floor(lng)), int16(math.Floor(lat))}
		for _, idx := range f.grid[key] {
			if f.items[idx].containsPoint(p) {
				res = append(res, f.items[idx].name)
			}
		}
	} else {
		for _, it := range f.items {
			if it.containsPoint(p) {
				res = append(res, it.name)
			}
		}
	}
	slices.Sort(res)
	return res, nil
}

func (f *Finder) TimezoneNames() []string { return f.names }
func (f *Finder) DataVersion() string     { return f.version }

// Fuzzy is the tile reference: the deleted v1 NewFuzzyFinderFromPB
// semantics over a source preindex.
type Fuzzy struct {
	idxZoom int
	aggZoom int
	single  map[geom.TileID]uint16
	multi   map[geom.TileID][]uint16
	version string
	names   []string
}

// NewFuzzy builds the tile reference from a source preindex.
func NewFuzzy(input *pb.PreindexTimezones) (*Fuzzy, error) {
	f := &Fuzzy{
		single:  make(map[geom.TileID]uint16),
		multi:   make(map[geom.TileID][]uint16),
		idxZoom: int(input.IdxZoom),
		aggZoom: int(input.AggZoom),
		version: input.Version,
	}
	namesMap := map[string]bool{}
	for _, k := range input.Keys {
		namesMap[k.Name] = true
	}
	f.names = maps.Keys(namesMap)
	slices.Sort(f.names)
	nameIdx := make(map[string]uint16, len(f.names))
	for i, n := range f.names {
		nameIdx[n] = uint16(i)
	}
	tmp := make(map[geom.TileID][]uint16)
	for _, k := range input.Keys {
		key := geom.NewTileIDFromXYZ(uint32(k.X), uint32(k.Y), uint8(k.Z))
		tmp[key] = append(tmp[key], nameIdx[k.Name])
	}
	for k, idxs := range tmp {
		if len(idxs) == 1 {
			f.single[k] = idxs[0]
		} else {
			f.multi[k] = idxs
		}
	}
	return f, nil
}

func (f *Fuzzy) GetTimezoneName(lng float64, lat float64) string {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		t := tile.Shift(uint8(f.idxZoom - z))
		if idx, ok := f.single[t]; ok {
			return f.names[idx]
		}
		if idxs, ok := f.multi[t]; ok {
			return f.names[idxs[0]]
		}
	}
	return ""
}

func (f *Fuzzy) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		t := tile.Shift(uint8(f.idxZoom - z))
		if idx, ok := f.single[t]; ok {
			return []string{f.names[idx]}, nil
		}
		if idxs, ok := f.multi[t]; ok {
			names := make([]string, len(idxs))
			for i, idx := range idxs {
				names[i] = f.names[idx]
			}
			return names, nil
		}
	}
	return nil, tzerr.ErrNoTimezoneFound
}

func (f *Fuzzy) TimezoneNames() []string { return f.names }
func (f *Fuzzy) DataVersion() string     { return f.version }
