package tzf

import (
	"encoding/json"
	"slices"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/maps"
)

// tileID packs (x, y, z) into a single uint64 key.
// Layout: bits 56-63 = zoom (0-255), bits 28-55 = x (up to 2^28), bits 0-27 = y (up to 2^28).
// This covers all OSM zoom levels (0-28) without collision.
func tileID(x, y uint32, z maptile.Zoom) uint64 {
	return uint64(z)<<56 | uint64(x)<<28 | uint64(y)
}

func tileFromID(id uint64) maptile.Tile {
	return maptile.New(uint32(id>>28)&0x0FFFFFFF, uint32(id)&0x0FFFFFFF, maptile.Zoom(id>>56))
}

// FuzzyFinder use a tile map to store timezone name. Data are made by
// [github.com/ringsaturn/tzf/cmd/preindextzpb] which powerd by
// [github.com/ringsaturn/tzf/preindex.PreIndexTimezones].
type FuzzyFinder struct {
	idxZoom int
	aggZoom int
	m       map[uint64][]string // key = tileID(x,y,z); timezones may have common area
	version string
	names   []string
}

func NewFuzzyFinderFromPB(input *pb.PreindexTimezones) (F, error) {
	f := &FuzzyFinder{
		m:       make(map[uint64][]string),
		idxZoom: int(input.IdxZoom),
		aggZoom: int(input.AggZoom),
		version: input.Version,
	}
	namesMap := map[string]bool{}
	for _, item := range input.Keys {
		k := tileID(uint32(item.X), uint32(item.Y), maptile.Zoom(item.Z))
		f.m[k] = append(f.m[k], item.Name)
		namesMap[item.Name] = true
	}
	f.names = maps.Keys(namesMap)
	slices.Sort(f.names)
	return f, nil
}

func (f *FuzzyFinder) GetTimezoneName(lng float64, lat float64) string {
	names, err := f.GetTimezoneNames(lng, lat)
	if err != nil {
		return ""
	}
	return names[0]
}

func (f *FuzzyFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	p := orb.Point{lng, lat}
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		t := maptile.At(p, maptile.Zoom(z))
		v, ok := f.m[tileID(t.X, t.Y, t.Z)]
		if ok {
			return v, nil
		}
	}
	return nil, ErrNoTimezoneFound
}

func (f *FuzzyFinder) TimezoneNames() []string {
	return f.names
}

func (f *FuzzyFinder) DataVersion() string {
	return f.version
}

// tileToPolygon converts a map tile's bounding box to a closed GeoJSON ring.
func tileToPolygon(tile maptile.Tile) [][2]float64 {
	b := tile.Bound()
	lngMin, latMin := b.Min[0], b.Min[1]
	lngMax, latMax := b.Max[0], b.Max[1]
	return [][2]float64{
		{lngMin, latMin},
		{lngMax, latMin},
		{lngMax, latMax},
		{lngMin, latMax},
		{lngMin, latMin},
	}
}

func fuzzyFeatureItem(name string, tiles []maptile.Tile) *convert.FeatureItem {
	coords := make(convert.MultiPolygonCoordinates, 0, len(tiles))
	for _, tile := range tiles {
		coords = append(coords, convert.PolygonCoordinates{tileToPolygon(tile)})
	}
	raw, err := json.Marshal(coords)
	if err != nil {
		panic(err) // unreachable: float64 coords are always marshalable
	}
	return &convert.FeatureItem{
		Type:       convert.FeatureType,
		Properties: convert.PropertiesDefine{Tzid: name},
		Geometry: convert.GeometryDefine{
			Type:        convert.MultiPolygonType,
			Coordinates: raw,
		},
	}
}

// GetTZGeoJSON returns a GeoJSON Feature for the named timezone, where each
// polygon is the bounding box of one preindex tile.
func (f *FuzzyFinder) GetTZGeoJSON(tzName string) (*convert.FeatureItem, error) {
	var tiles []maptile.Tile
	for id, names := range f.m {
		if slices.Contains(names, tzName) {
			tiles = append(tiles, tileFromID(id))
		}
	}
	if len(tiles) == 0 {
		return nil, ErrNoTimezoneFound
	}
	return fuzzyFeatureItem(tzName, tiles), nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones, where
// each polygon is the bounding box of one preindex tile.
func (f *FuzzyFinder) GetGeoJSON() *convert.BoundaryFile {
	nameToTiles := make(map[string][]maptile.Tile, len(f.names))
	for id, names := range f.m {
		tile := tileFromID(id)
		for _, n := range names {
			nameToTiles[n] = append(nameToTiles[n], tile)
		}
	}
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for _, name := range f.names { // sorted, deterministic order
		output.Features = append(output.Features, fuzzyFeatureItem(name, nameToTiles[name]))
	}
	return output
}
