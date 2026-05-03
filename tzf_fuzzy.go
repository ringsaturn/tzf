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
//
// Tiles are split into two maps for memory efficiency:
//   - single: tiles that belong to exactly one timezone (vast majority); value is a names index.
//   - multi:  tiles that straddle a timezone boundary; value is a slice of names indices.
type FuzzyFinder struct {
	idxZoom int
	aggZoom int
	single  map[geom.TileID]uint16   // tile → single timezone index into names
	multi   map[geom.TileID][]uint16 // tile → multiple timezone indices (boundary tiles only)
	version string
	names   []string
}

func NewFuzzyFinderFromPB(input *pb.PreindexTimezones) (F, error) {
	f := &FuzzyFinder{
		single:  make(map[geom.TileID]uint16),
		multi:   make(map[geom.TileID][]uint16),
		idxZoom: int(input.IdxZoom),
		aggZoom: int(input.AggZoom),
		version: input.Version,
	}

	// First pass: collect all timezone names for stable index assignment.
	namesMap := map[string]bool{}
	for _, item := range input.Keys {
		namesMap[item.Name] = true
	}
	f.names = maps.Keys(namesMap)
	slices.Sort(f.names)
	nameIdx := make(map[string]uint16, len(f.names))
	for i, n := range f.names {
		nameIdx[n] = uint16(i)
	}

	// Second pass: populate maps using indices.
	// Use a temporary full map to detect single vs multi.
	tmp := make(map[geom.TileID][]uint16)
	for _, item := range input.Keys {
		k := geom.NewTileIDFromXYZ(uint32(item.X), uint32(item.Y), uint8(item.Z))
		idx := nameIdx[item.Name]
		tmp[k] = append(tmp[k], idx)
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

func (f *FuzzyFinder) GetTimezoneName(lng float64, lat float64) string {
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

func (f *FuzzyFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
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
	nameIdx := slices.Index(f.names, tzName)
	if nameIdx < 0 {
		return nil, ErrNoTimezoneFound
	}
	target := uint16(nameIdx)
	var ids []geom.TileID
	for id, idx := range f.single {
		if idx == target {
			ids = append(ids, id)
		}
	}
	for id, idxs := range f.multi {
		if slices.Contains(idxs, target) {
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
	nameToIDs := make(map[uint16][]geom.TileID, len(f.names))
	for id, idx := range f.single {
		nameToIDs[idx] = append(nameToIDs[idx], id)
	}
	for id, idxs := range f.multi {
		for _, idx := range idxs {
			nameToIDs[idx] = append(nameToIDs[idx], id)
		}
	}
	output := &convert.BoundaryFile{Type: "FeatureCollection"}
	for i, name := range f.names { // sorted, deterministic order
		output.Features = append(output.Features, fuzzyFeatureItem(name, nameToIDs[uint16(i)]))
	}
	return output
}
