// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"fmt"
	"time"

	"github.com/ringsaturn/tzf/convert"
	"github.com/ringsaturn/tzf/pb"
	"github.com/ringsaturn/tzf/reduce"
	"github.com/tidwall/geojson/geometry"
	"github.com/tidwall/rtree"
)

type tzitem struct {
	pbtz     *pb.Timezone
	location *time.Location
	shift    int
	polys    []*geometry.Poly
}

func newNotFoundErr(lng float64, lat float64) error {
	return fmt.Errorf("tzf: not found for %v,%v", lng, lat)
}

func (i *tzitem) ContainsPoint(p geometry.Point) bool {
	for _, poly := range i.polys {
		if poly.ContainsPoint(p) {
			return true
		}
	}
	return false
}

func (i *tzitem) GetMinMax() ([2]float64, [2]float64) {
	retmin := [2]float64{
		i.polys[0].Rect().Min.X,
		i.polys[0].Rect().Min.Y,
	}
	retmax := [2]float64{
		i.polys[0].Rect().Max.X,
		i.polys[0].Rect().Max.Y,
	}

	for _, poly := range i.polys {
		minx := poly.Rect().Min.X
		miny := poly.Rect().Min.Y
		if minx < retmin[0] && miny < retmin[1] {
			retmin[0] = minx
			retmin[1] = miny
		}

		maxx := poly.Rect().Max.X
		maxy := poly.Rect().Max.Y
		if minx < retmax[0] && miny < retmax[1] {
			retmax[0] = maxx
			retmax[1] = maxy
		}
	}
	return retmin, retmax
}

// Finder is based on point-in-polygon search algo.
//
// Memeory will use about 100MB if lite data and 1G if full data.
// Performance is very stable and very accuate.
type Finder struct {
	items   []*tzitem
	names   []string
	reduced bool
	tr      *rtree.RTreeG[*tzitem]
}

func NewFinderFromRawJSON(input *convert.BoundaryFile) (*Finder, error) {
	timezones, err := convert.Do(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(timezones)
}

func NewFinderFromPB(input *pb.Timezones) (*Finder, error) {
	now := time.Now()
	items := make([]*tzitem, 0)
	names := make([]string, 0)

	tr := &rtree.RTreeG[*tzitem]{}
	for _, timezone := range input.Timezones {
		names = append(names, timezone.Name)
		location, err := time.LoadLocation(timezone.Name)
		if err != nil {
			return nil, err
		}
		_, tzOffset := now.In(location).Zone()

		newItem := &tzitem{
			pbtz:     timezone,
			location: location,
			shift:    tzOffset,
		}
		for _, polygon := range timezone.Polygons {

			newPoints := make([]geometry.Point, 0)
			for _, point := range polygon.Points {
				newPoints = append(newPoints, geometry.Point{
					X: float64(point.Lng),
					Y: float64(point.Lat),
				})
			}

			holes := [][]geometry.Point{}
			for _, holePoly := range polygon.Holes {
				newHolePoints := make([]geometry.Point, 0)
				for _, point := range holePoly.Points {
					newHolePoints = append(newHolePoints, geometry.Point{
						X: float64(point.Lng),
						Y: float64(point.Lat),
					})
				}
				holes = append(holes, newHolePoints)
			}

			newPoly := geometry.NewPoly(newPoints, holes, nil)
			newItem.polys = append(newItem.polys, newPoly)
		}
		items = append(items, newItem)
		minp, maxp := newItem.GetMinMax()
		tr.Insert(minp, maxp, newItem)
	}
	finder := &Finder{}
	finder.items = items
	finder.names = names
	finder.reduced = input.Reuced
	finder.tr = tr
	return finder, nil
}

func NewFinderFromCompressed(input *pb.CompressedTimezones) (*Finder, error) {
	tzs, err := reduce.Decompress(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(tzs)
}

func (f *Finder) getItem(lng float64, lat float64) (*tzitem, error) {
	p := geometry.Point{
		X: float64(lng),
		Y: float64(lat),
	}
	for _, item := range f.items {
		if item.ContainsPoint(p) {
			return item, nil
		}
	}
	return nil, newNotFoundErr(lng, lat)
}

func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	item, err := f.getItem(lng, lat)
	if err != nil {
		return ""
	}
	return item.pbtz.GetName()
}

func (f *Finder) GetTimezoneLoc(lng float64, lat float64) (*time.Location, error) {
	item, err := f.getItem(lng, lat)
	if err != nil {
		return nil, err
	}
	return item.location, nil
}

func (f *Finder) GetTimezone(lng float64, lat float64) (*pb.Timezone, error) {
	p := geometry.Point{
		X: float64(lng),
		Y: float64(lat),
	}
	candicates := []*tzitem{}
	for _, shifted := range []float64{3, 10, 15} {
		f.tr.Search([2]float64{lng - shifted, lat - shifted}, [2]float64{lng + shifted, lat + shifted}, func(min, max [2]float64, data *tzitem) bool {
			candicates = append(candicates, data)
			return true
		})
		if len(candicates) > 10 {
			break
		}
	}

	for _, item := range candicates {
		if item.ContainsPoint(p) {
			return item.pbtz, nil
		}
	}
	return nil, newNotFoundErr(lng, lat)
}

func (f *Finder) GetTimezoneShapeByName(name string) (*pb.Timezone, error) {
	for _, item := range f.items {
		if item.pbtz.Name == name {
			return item.pbtz, nil
		}
	}
	return nil, fmt.Errorf("timezone=%v not found", name)
}

func (f *Finder) GetTimezoneShapeByShift(shift int) ([]*pb.Timezone, error) {
	res := make([]*pb.Timezone, 0)
	for _, item := range f.items {
		if item.shift == shift {
			res = append(res, item.pbtz)
		}
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("shift=%v not found", shift)
	}
	return res, nil
}

func (f *Finder) TimezoneNames() []string {
	return f.names
}
