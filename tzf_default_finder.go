package tzf

import (
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
		if err := proto.Unmarshal(tzfrel.LiteData, input); err != nil {
			panic(err)
		}
		return NewFinderFromCompressed(input)
	}()
	if err != nil {
		return nil, err
	}

	f := &DefaultFinder{}
	f.fuzzyFinder = fuzzyFinder
	f.finder = finder

	return f, nil
}

func (f *DefaultFinder) GetTimezoneName(lng float64, lat float64) string {
	fuzzyRes := f.fuzzyFinder.GetTimezoneName(lng, lat)
	if fuzzyRes != "" {
		return fuzzyRes
	}
	return f.finder.GetTimezoneName(lng, lat)
}

func (f *DefaultFinder) TimezoneNames() []string {
	return f.finder.TimezoneNames()
}
