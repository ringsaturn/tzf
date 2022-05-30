// Package tzf is a package convert (lng,lat) to timezone.
//
// Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
// fast python package for finding the timezone of any point on earth (coordinates) offline.
package tzf

import (
	"fmt"
	"time"

	"github.com/ringsaturn/tzf/compress"
	"github.com/ringsaturn/tzf/convert"
	"github.com/ringsaturn/tzf/pb"
	"github.com/tidwall/geometry"
)

type item struct {
	pbtz  *pb.Timezone
	polys []*geometry.Poly
}

func newNotFoundErr(lng float64, lat float64) error {
	return fmt.Errorf("tzf: not found for %v,%v", lng, lat)
}

func (i *item) ContainsPoint(p geometry.Point) bool {
	for _, poly := range i.polys {
		if poly.ContainsPoint(p) {
			return true
		}
	}
	return false
}

type Finder struct {
	items []*item
}

func NewFinderFromRawJSON(input *convert.BoundaryFile) (*Finder, error) {
	timezones, err := convert.Do(input)
	if err != nil {
		return nil, err
	}
	return NewFinderFromPB(timezones)
}

func NewFinderFromPB(input *pb.Timezones) (*Finder, error) {
	items := make([]*item, 0)
	for _, timezone := range input.Timezones {
		newItem := &item{
			pbtz: timezone,
		}
		for _, polygon := range timezone.Polygons {

			newPoints := make([]geometry.Point, 0)
			for _, point := range polygon.Points {
				newPoints = append(newPoints, geometry.Point{
					X: float64(point.Lng),
					Y: float64(point.Lat),
				})
			}
			newPoly := geometry.NewPoly(newPoints, nil, nil)
			newItem.polys = append(newItem.polys, newPoly)
		}
		items = append(items, newItem)
	}
	finder := &Finder{}
	finder.items = items
	return finder, nil
}

func NewFinderFromCompressPB(input *pb.CompressTimezones) (*Finder, error) {
	return NewFinderFromPB(compress.ToNormalPB(input))
}

func (f *Finder) GetTimezoneName(lng float64, lat float64) string {
	p := geometry.Point{
		X: float64(lng),
		Y: float64(lat),
	}
	for _, item := range f.items {
		if item.ContainsPoint(p) {
			return item.pbtz.Name
		}
	}
	return ""
}

func (f *Finder) GetTimezoneLoc(lng float64, lat float64) (*time.Location, error) {
	name := f.GetTimezoneName(lng, lat)
	if name == "" {
		return nil, newNotFoundErr(lng, lat)
	}
	return time.LoadLocation(name)
}

func (f *Finder) GetTimezone(lng float64, lat float64) (*pb.Timezone, error) {
	p := geometry.Point{
		X: float64(lng),
		Y: float64(lat),
	}
	for _, item := range f.items {
		if item.ContainsPoint(p) {
			return item.pbtz, nil
		}
	}
	return nil, newNotFoundErr(lng, lat)
}
