package preindex

import (
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

func PreIndexTimezone(input *pb.Timezone, minIndexzoom maptile.Zoom, aggZoom maptile.Zoom, dropEdgeLayger int) ([]*pb.PreindexTimezone, error) {
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
		polytiles, err := tilecover.Geometry(orbPoly, minIndexzoom)
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
		ret = append(ret, &pb.PreindexTimezone{
			Name: input.Name,
			X:    int32(v.X),
			Y:    int32(v.Y),
			Z:    int32(v.Z),
		})
	}
	return ret, nil
}
