package tzf

import (
	"fmt"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/ringsaturn/tzf/pb"
)

// FuzzyFinder will iter tile zoom level for input points and check if tile in a map.
type FuzzyFinder struct {
	idxZoom int
	aggZoom int
	m       map[maptile.Tile][]string // timezones may have common area
}

func NewFuzzyFinderFromPB(input *pb.PreindexTimezones) (*FuzzyFinder, error) {
	fmt.Println(input.AggZoom, input.IdxZoom)
	f := &FuzzyFinder{
		m:       make(map[maptile.Tile][]string),
		idxZoom: int(input.IdxZoom),
		aggZoom: int(input.AggZoom),
	}
	for _, item := range input.Keys {
		tile := maptile.New(uint32(item.X), uint32(item.Y), maptile.Zoom(item.Z))
		if _, ok := f.m[tile]; !ok {
			f.m[tile] = make([]string, 0)
		}
		f.m[tile] = append(f.m[tile], item.Name)
	}
	return f, nil
}

func (f *FuzzyFinder) GetTimezoneName(lng float64, lat float64) string {
	p := orb.Point{lng, lat}
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		key := maptile.At(p, maptile.Zoom(z))
		v, ok := f.m[key]
		if ok {
			// TODO(ringsaturn): support return all matched tz names
			return v[0]
		}
	}
	return ""
}
