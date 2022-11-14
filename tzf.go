// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"errors"
	"fmt"
	"time"

	"github.com/ringsaturn/tzf/convert"
	"github.com/ringsaturn/tzf/pb"
	"github.com/ringsaturn/tzf/reduce"
	"github.com/tidwall/geojson/geometry"
	"github.com/tidwall/rtree"
)

var ErrNoTimezoneFound = errors.New("tzf: no timezone found")

type Option struct {
	DropPBTZ bool
}

type OptionFunc = func(opt *Option)

// SetDropPBTZ will make Finder not save [github.com/ringsaturn/tzf/pb.Timezone] in memory
func SetDropPBTZ(opt *Option) {
	opt.DropPBTZ = true
}

type tzitem struct {
	pbtz     *pb.Timezone
	location *time.Location
	name     string
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
		if minx < retmin[0] {
			retmin[0] = minx
		}
		if miny < retmin[1] {
			retmin[1] = miny
		}

		maxx := poly.Rect().Max.X
		maxy := poly.Rect().Max.Y
		if minx < retmax[0] {
			retmax[0] = maxx

		}
		if miny < retmax[1] {
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
	opt     *Option
}

func NewFinderFromRawJSON(input *convert.BoundaryFile, opts ...OptionFunc) (*Finder, error) {
	timezones, err := convert.Do(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(timezones, opts...)
}

func NewFinderFromPB(input *pb.Timezones, opts ...OptionFunc) (*Finder, error) {
	now := time.Now()
	items := make([]*tzitem, 0)
	names := make([]string, 0)

	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}

	tr := &rtree.RTreeG[*tzitem]{}
	for _, timezone := range input.Timezones {
		names = append(names, timezone.Name)
		location, err := time.LoadLocation(timezone.Name)
		if err != nil {
			// check if changed
			oldname, ok := backportstz[timezone.Name]
			if !ok {
				return nil, err
			}
			location, err = time.LoadLocation(oldname)
			if err != nil {
				return nil, err
			}

		}
		_, tzOffset := now.In(location).Zone()

		newItem := &tzitem{
			location: location,
			shift:    tzOffset,
			name:     timezone.Name,
		}
		if !opt.DropPBTZ {
			newItem.pbtz = timezone
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
		minp, maxp := newItem.GetMinMax()

		items = append(items, newItem)
		tr.Insert(minp, maxp, newItem)
	}
	finder := &Finder{}
	finder.items = items
	finder.names = names
	finder.reduced = input.Reduced
	finder.tr = tr
	finder.opt = opt
	return finder, nil
}

func NewFinderFromCompressed(input *pb.CompressedTimezones, opts ...OptionFunc) (*Finder, error) {
	tzs, err := reduce.Decompress(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(tzs, opts...)
}

func (f *Finder) getItem(lng float64, lat float64) ([]*tzitem, error) {
	p := geometry.Point{
		X: float64(lng),
		Y: float64(lat),
	}
	ret := []*tzitem{}
	candicates := []*tzitem{}

	// TODO(ringsaturn): fix this range
	shifted := 70.0
	f.tr.Search([2]float64{lng - shifted, lat - shifted}, [2]float64{lng + shifted, lat + shifted}, func(min, max [2]float64, data *tzitem) bool {
		candicates = append(candicates, data)
		return true
	})
	if len(candicates) == 0 {
		candicates = f.items
	}
	for _, item := range candicates {
		if item.ContainsPoint(p) {
			ret = append(ret, item)
		}
	}
	if len(ret) == 0 {
		return nil, newNotFoundErr(lng, lat)
	}
	return ret, nil
}

func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	item, err := f.getItem(lng, lat)
	if err != nil {
		return ""
	}
	return item[0].name
}

func (f *Finder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	item, err := f.getItem(lng, lat)
	if err != nil {
		return nil, err
	}
	ret := []string{}
	for i := 0; i < len(item); i++ {
		ret = append(ret, item[i].name)
	}
	return ret, nil
}

func (f *Finder) GetTimezoneLoc(lng float64, lat float64) (*time.Location, error) {
	item, err := f.getItem(lng, lat)
	if err != nil {
		return nil, err
	}
	return item[0].location, nil
}

func (f *Finder) GetTimezone(lng float64, lat float64) (*pb.Timezone, error) {
	if f.opt.DropPBTZ {
		return nil, errors.New("tzf: not suppor when reduce mem")
	}
	item, err := f.getItem(lng, lat)
	if err != nil {
		return nil, err
	}
	return item[0].pbtz, nil
}

func (f *Finder) GetTimezoneShapeByName(name string) (*pb.Timezone, error) {
	for _, item := range f.items {
		if item.name == name {
			return item.pbtz, nil
		}
	}
	return nil, fmt.Errorf("timezone=%v not found", name)
}

func (f *Finder) GetTimezoneShapeByShift(shift int) ([]*pb.Timezone, error) {
	if f.opt.DropPBTZ {
		return nil, errors.New("tzf: not suppor when reduce mem")
	}
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
