package tzf

import (
	"github.com/ringsaturn/tzf/pb"
	"github.com/tidwall/geojson"
	"github.com/tidwall/geojson/geometry"
)

type treeItem struct {
	name      string
	multiPoly *geojson.MultiPolygon
}

// FinderByRTree will use RTree to search.
type FinderByRTree struct {
	items []*treeItem
}

func NewFinderByRTreeFromPB(input *pb.Timezones) (*FinderByRTree, error) {
	items := make([]*treeItem, 0)
	for _, timezone := range input.Timezones {
		polys := []*geometry.Poly{}
		for _, polygon := range timezone.Polygons {
			newPoints := make([]geometry.Point, 0)
			for _, point := range polygon.Points {
				newPoints = append(newPoints, geometry.Point{
					X: float64(point.Lng),
					Y: float64(point.Lat),
				})
			}
			newPoly := geometry.NewPoly(newPoints, nil, nil)
			polys = append(polys, newPoly)
		}
		multiPoly := geojson.NewMultiPolygon(polys)
		newItem := &treeItem{
			name:      timezone.Name,
			multiPoly: multiPoly,
		}
		items = append(items, newItem)
	}
	return &FinderByRTree{
		items: items,
	}, nil
}

func (f *FinderByRTree) GetTimezoneName(lng float64, lat float64) string {
	p := geojson.NewPoint(geometry.Point{X: lng, Y: lat})
	for _, item := range f.items {
		if item.multiPoly.Contains(p) {
			return item.name
		}
	}
	return ""
}
