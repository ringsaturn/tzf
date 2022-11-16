package tzf

import (
	"runtime"

	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

// DefaultFinder is a finder impl combine both [FuzzyFinder] and [Finder].
//
// It's designed for performance first and allow some not so correct return at some area.
type DefaultFinder struct {
	fuzzyFinder *FuzzyFinder
	finder      *Finder
}

func NewDefaultFinder() (*DefaultFinder, error) {
	fuzzyFinder, err := func() (*FuzzyFinder, error) {
		input := &pb.PreindexTimezones{}
		if err := proto.Unmarshal(tzfrel.PreindexData, input); err != nil {
			panic(err)
		}
		return NewFuzzyFinderFromPB(input)
	}()
	if err != nil {
		return nil, err
	}

	finder, err := func() (*Finder, error) {
		input := &pb.CompressedTimezones{}
		if err := proto.Unmarshal(tzfrel.LiteCompressData, input); err != nil {
			panic(err)
		}
		return NewFinderFromCompressed(input, SetDropPBTZ)
	}()
	if err != nil {
		return nil, err
	}

	f := &DefaultFinder{}
	f.fuzzyFinder = fuzzyFinder
	f.finder = finder

	// Force free mem by probuf, about 80MB
	runtime.GC()

	return f, nil
}

func (f *DefaultFinder) GetTimezoneName(lng float64, lat float64) string {
	fuzzyRes := f.fuzzyFinder.GetTimezoneName(lng, lat)
	if fuzzyRes != "" {
		return fuzzyRes
	}
	name := f.finder.GetTimezoneName(lng, lat)
	if name != "" {
		return name
	}
	for _, dx := range []float64{-0.02, 0, 0.02} {
		for _, dy := range []float64{-0.02, 0, 0.02} {
			dlng := dx + lng
			dlat := dy + lat
			fuzzyRes := f.fuzzyFinder.GetTimezoneName(dlng, dlat)
			if fuzzyRes != "" {
				return fuzzyRes
			}
			name := f.finder.GetTimezoneName(dlng, dlat)
			if name != "" {
				return name
			}
		}
	}
	return ""
}

func (f *DefaultFinder) GetTimezoneNames(lng float64, lat float64) ([]string, error) {
	fuzzyRes, err := f.fuzzyFinder.GetTimezoneNames(lng, lat)
	if err == nil {
		return fuzzyRes, nil
	}
	return f.finder.GetTimezoneNames(lng, lat)
}

func (f *DefaultFinder) TimezoneNames() []string {
	return f.finder.TimezoneNames()
}
