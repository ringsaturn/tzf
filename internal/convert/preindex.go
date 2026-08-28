package convert

import (
	"encoding/json"
	"slices"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// PreindexTilesFor collects the FUZZY preindex tiles naming any of the target
// timezone indices, from the single/multi maps a reader rebuilds out of the
// FUZZY section. The same name may map to more than one directory item, so
// callers pass every index carrying it.
func PreindexTilesFor(
	single map[geom.TileID]uint16,
	multi map[geom.TileID][]uint16,
	targets []uint16,
) []geom.TileID {
	var tiles []geom.TileID
	for tile, idx := range single {
		if slices.Contains(targets, idx) {
			tiles = append(tiles, tile)
		}
	}
	for tile, idxs := range multi {
		for _, idx := range idxs {
			if slices.Contains(targets, idx) {
				tiles = append(tiles, tile)
				break
			}
		}
	}
	return tiles
}

// PreindexTilesGrouped groups every FUZZY preindex tile by the timezone index
// it names; a boundary tile lands in every group it names.
func PreindexTilesGrouped(
	single map[geom.TileID]uint16,
	multi map[geom.TileID][]uint16,
) map[uint16][]geom.TileID {
	grouped := make(map[uint16][]geom.TileID)
	for tile, idx := range single {
		grouped[idx] = append(grouped[idx], tile)
	}
	for tile, idxs := range multi {
		for _, idx := range idxs {
			grouped[idx] = append(grouped[idx], tile)
		}
	}
	return grouped
}

// RevertItemFromTiles builds one GeoJSON Feature from FUZZY preindex tiles: a
// MultiPolygon holding each tile's bounding rectangle as one closed ring.
// Tiles are sorted by packed key — coarsest zoom first — so the output is
// deterministic despite map iteration order.
func RevertItemFromTiles(name string, tiles []geom.TileID) *FeatureItem {
	slices.Sort(tiles)
	coords := make(MultiPolygonCoordinates, len(tiles))
	for i, tile := range tiles {
		coords[i] = PolygonCoordinates{tile.Polygon()}
	}
	raw, err := json.Marshal(coords)
	if err != nil {
		panic(err) // unreachable: float64 coords are always marshalable
	}
	return &FeatureItem{
		Type:       FeatureType,
		Properties: PropertiesDefine{Tzid: name},
		Geometry: GeometryDefine{
			Type:        MultiPolygonType,
			Coordinates: raw,
		},
	}
}
