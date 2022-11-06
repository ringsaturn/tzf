package preindex

import (
	"encoding/json"
	"fmt"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	"github.com/ringsaturn/tzf/convert"
	"github.com/ringsaturn/tzf/pb"
	"github.com/tidwall/geojson/geometry"
	"golang.org/x/exp/maps"
)

func DropEdgeTiles(tiles []maptile.Tile) []maptile.Tile {
	ret := []maptile.Tile{}
	tilehash := map[maptile.Tile]bool{}

	// setup tilehash
	for _, tile := range tiles {
		tilehash[tile] = true
	}

	// filter all neighbor in tiles
	for _, tile := range tiles {
		neighbors := []maptile.Tile{
			maptile.New(tile.X-1, tile.Y-1, tile.Z),
			maptile.New(tile.X, tile.Y-1, tile.Z),
			maptile.New(tile.X+1, tile.Y-1, tile.Z),

			maptile.New(tile.X-1, tile.Y, tile.Z),
			// maptile.New(tile.X, tile.Y, tile.Z),
			maptile.New(tile.X+1, tile.Y, tile.Z),

			maptile.New(tile.X-1, tile.Y+1, tile.Z),
			maptile.New(tile.X, tile.Y+1, tile.Z),
			maptile.New(tile.X+1, tile.Y+1, tile.Z),
		}

		var allNeighorIn bool = func() bool {
			for _, neighborTile := range neighbors {
				if _, ok := tilehash[neighborTile]; !ok {
					return false
				}
			}
			return true
		}()
		if !allNeighorIn {
			continue
		}
		ret = append(ret, tile)
	}

	return ret
}

// PreIndexTimezone will gen tiles at `idxZoom` level and merge up to `aggZoom`.
//
// The `idxZoom` level tiles will be removed before final return.
func PreIndexTimezone(input *pb.Timezone, idxZoom, aggZoom, maxZoomLevelToKeep maptile.Zoom, dropEdgeLayger int) ([]*pb.PreindexTimezone, error) {
	// Generate all tiles event not included in timezone shape
	tiles := []maptile.Tile{}
	for _, poly := range input.Polygons {
		orbPoly := orb.Polygon{}

		ring := orb.Ring{}
		for _, point := range poly.Points {
			ring = append(ring, orb.Point{float64(point.Lng), float64(point.Lat)})
		}
		// bypass too little
		if len(ring) < 10 {
			continue
		}
		// add first point
		ring = append(ring, ring[0])
		orbPoly = append(orbPoly, ring)

		// add polygon holes
		for _, hole := range poly.Holes {
			holering := orb.Ring{}
			for _, point := range hole.Points {
				holering = append(holering, orb.Point{float64(point.Lng), float64(point.Lat)})
			}
			if len(holering) < 3 {
				continue
			}
			holering = append(holering, holering[0])
			orbPoly = append(orbPoly, holering)
		}

		// gen polygon tiles
		polytiles, err := tilecover.Geometry(orbPoly, idxZoom)
		if err != nil {
			panic(err)
		}
		tiles = append(tiles, maps.Keys(polytiles)...)
	}
	// unable to agg
	if len(tiles) < 9 {
		return nil, fmt.Errorf("too little")
	}

	// Iter all tile's polygon if inside original polygon
	insideTZTiles := []maptile.Tile{}
	geopolys := convert.FromTimezonePBToGeometryPoly(input)
	for _, tile := range tiles {
		minLon := tile.Bound().Min.Lon()
		minLat := tile.Bound().Min.Lat()
		maxLng := tile.Bound().Max.Lon()
		maxLat := tile.Bound().Max.Lat()

		geometryPoints := []geometry.Point{
			{X: minLon, Y: minLat},
			{X: maxLng, Y: minLat},
			{X: maxLng, Y: maxLat},
			{X: minLon, Y: maxLat},
			{X: minLon, Y: minLat},
		}
		tilePoly := geometry.NewPoly(geometryPoints, nil, nil)

		for _, geopoly := range geopolys {
			if !geopoly.ContainsPoly(tilePoly) {
				continue
			}
		}
		insideTZTiles = append(insideTZTiles, tile)
	}

	// Drop edge tiles
	for i := 0; i < dropEdgeLayger; i++ {
		insideTZTiles = DropEdgeTiles(insideTZTiles)
	}

	// Gen tileset
	newtileset := maptile.Set{}
	for _, tile := range insideTZTiles {
		newtileset[tile] = true
	}

	// Merge all filterd tiles
	mergedtiles := tilecover.MergeUp(newtileset, aggZoom)

	// // Dumps JSON for debug
	// b, _ := json.Marshal(mergedtiles.ToFeatureCollection())
	// _ = os.WriteFile("preindexex.geojson", b, 0644)

	// Dumps as pb
	ret := []*pb.PreindexTimezone{}
	for _, v := range maps.Keys(mergedtiles) {
		if int(v.Z) > int(maxZoomLevelToKeep) {
			continue
		}
		ret = append(ret, &pb.PreindexTimezone{
			Name: input.Name,
			X:    int32(v.X),
			Y:    int32(v.Y),
			Z:    int32(v.Z),
		})
	}
	return ret, nil
}

func PreIndexTimezones(input *pb.Timezones, idxZoom, aggZoom, maxZoomLevelToKeep maptile.Zoom, dropEdgeLayger int) *pb.PreindexTimezones {
	ret := &pb.PreindexTimezones{
		IdxZoom: int32(idxZoom),
		AggZoom: int32(aggZoom),
		Keys:    make([]*pb.PreindexTimezone, 0),
	}
	for _, tz := range input.Timezones {
		preindexes, err := PreIndexTimezone(tz, idxZoom, aggZoom, maxZoomLevelToKeep, dropEdgeLayger)
		if err == nil {
			ret.Keys = append(ret.Keys, preindexes...)
		}
	}
	return ret
}

func PreIndexTimezonesToGeoJSON(input *pb.PreindexTimezones) []byte {
	tileset := maptile.Set{}
	for _, key := range input.Keys {
		tileset[maptile.New(uint32(key.X), uint32(key.Y), maptile.Zoom(key.Z))] = true
	}
	b, _ := json.Marshal(tileset.ToFeatureCollection())
	return b
}
