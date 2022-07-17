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
	"github.com/tidwall/geojson/geometry"
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

type Finder struct {
	items []*tzitem
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
	for _, timezone := range input.Timezones {
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
			newPoly := geometry.NewPoly(newPoints, nil, nil)
			newItem.polys = append(newItem.polys, newPoly)
		}
		items = append(items, newItem)
	}
	finder := &Finder{}
	finder.items = items
	return finder, nil
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
	for _, item := range f.items {
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
