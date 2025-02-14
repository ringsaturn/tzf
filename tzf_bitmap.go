package tzf

import (
	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/ringsaturn/tzf/internal/pmtiles"
	"github.com/ringsaturn/tzf/pb"
)

type BitmapFinder struct {
	aggZoom int
	idxZoom int
	version string
	Maps    map[string]*roaring64.Bitmap
	Names   []string
}

// type F interface {
// 	GetTimezoneName(lng float64, lat float64) string
// 	GetTimezoneNames(lng float64, lat float64) ([]string, error)
// 	TimezoneNames() []string
// 	DataVersion() string
// }

func NewBitmapFinderFrompb(input *pb.PreindexBitmaps) (*BitmapFinder, error) {
	maps := make(map[string]*roaring64.Bitmap)
	names := []string{}

	for _, item := range input.Items {
		bitmap := roaring64.New()
		err := bitmap.UnmarshalBinary(item.Data)
		if err != nil {
			return nil, err
		}
		maps[item.Name] = bitmap
		names = append(names, item.Name)
	}

	return &BitmapFinder{
		Maps:    maps,
		Names:   names,
		aggZoom: int(input.BitmapAggZoom),
		idxZoom: int(input.BitmapIdxZoom),
		version: input.Version,
	}, nil
}

func (f *BitmapFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	p := orb.Point{lng, lat}
	output := []string{}
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		key := maptile.At(p, maptile.Zoom(z))
		id := pmtiles.ZxyToID(uint8(z), key.X, key.Y)

		for _, name := range f.Names {
			if f.Maps[name].Contains(id) {
				output = append(output, name)
			}
		}
	}

	if len(output) == 0 {
		return nil, ErrNoTimezoneFound
	}

	return output, nil
}

func (f *BitmapFinder) GetTimezoneName(lng float64, lat float64) string {
	names, err := f.GetTimezoneNames(lng, lat)
	if err != nil {
		return ""
	}
	return names[0]
}

func (f *BitmapFinder) TimezoneNames() []string {
	return f.Names
}

func (f *BitmapFinder) DataVersion() string {
	return f.version
}
