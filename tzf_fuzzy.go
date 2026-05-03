package tzf

import (
	"encoding/json"
	"slices"

	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
	"github.com/ringsaturn/tzf/internal/maps"
)

// FuzzyFinder use a tile map to store timezone name. Data are made by
// [github.com/ringsaturn/tzf/cmd/preindextzpb] which powerd by
// [github.com/ringsaturn/tzf/preindex.PreIndexTimezones].
type FuzzyFinder struct {
	idxZoom int
	aggZoom int
	m       map[geom.TileID][]string // key = tileID(x,y,z); timezones may have common area
	version string
	names   []string
}

func NewFuzzyFinderFromPB(input *pb.PreindexTimezones) (F, error) {
	f := &FuzzyFinder{
		m:       make(map[geom.TileID][]string),
		idxZoom: int(input.IdxZoom),
		aggZoom: int(input.AggZoom),
		version: input.Version,
	}
	namesMap := map[string]bool{}
	for _, item := range input.Keys {
		k := geom.NewTileIDFromXYZ(uint32(item.X), uint32(item.Y), uint8(item.Z))
		f.m[k] = append(f.m[k], item.Name)
		namesMap[item.Name] = true
	}
	f.names = maps.Keys(namesMap)
	slices.Sort(f.names)
	return f, nil
}

func (f *FuzzyFinder) GetTimezoneName(lng float64, lat float64) string {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		if v, ok := f.m[tile.Shift(uint8(f.idxZoom-z))]; ok {
			return v[0]
		}
	}
	return ""
}

func (f *FuzzyFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		if v, ok := f.m[tile.Shift(uint8(f.idxZoom-z))]; ok {
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

func fuzzyFeatureItem(name string, ids []geom.TileID) *convert.FeatureItem {
	coords := make(convert.MultiPolygonCoordinates, 0, len(ids))
	for _, t := range ids {
		coords = append(coords, convert.PolygonCoordinates{t.Polygon()})
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
	var ids []geom.TileID
	for id, names := range f.m {
		if slices.Contains(names, tzName) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, ErrNoTimezoneFound
	}
	return fuzzyFeatureItem(tzName, ids), nil
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones, where
// each polygon is the bounding box of one preindex tile.
func (f *FuzzyFinder) GetGeoJSON() *convert.BoundaryFile {
	nameToIDs := make(map[string][]geom.TileID, len(f.names))
	for id, names := range f.m {
		for _, n := range names {
			nameToIDs[n] = append(nameToIDs[n], id)
		}
	}
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for _, name := range f.names { // sorted, deterministic order
		output.Features = append(output.Features, fuzzyFeatureItem(name, nameToIDs[name]))
	}
	return output
}
