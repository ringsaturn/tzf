package tzf

import (
	"fmt"
	"runtime"

	tzfdist "github.com/ringsaturn/tzf-dist"
	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"google.golang.org/protobuf/proto"
)

// DefaultFinder is a finder impl combine both [FuzzyFinder] and [Finder].
//
// It's designed for performance first and allow some not so correct return at some area.
type DefaultFinder struct {
	fuzzyFinder F
	finder      F
}

func NewDefaultFinder() (F, error) {
	fuzzyFinder, err := func() (F, error) {
		input := &pb.PreindexTimezones{}
		if err := proto.Unmarshal(tzfdist.PreindexData, input); err != nil {
			panic(err)
		}
		return NewFuzzyFinderFromPB(input)
	}()
	if err != nil {
		return nil, err
	}

	finder, err := func() (F, error) {
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			panic(err)
		}
		return NewFinderFromCompressedTopo(input, SetDropPBTZ)
	}()
	if err != nil {
		return nil, err
	}

	if finder.DataVersion() != fuzzyFinder.DataVersion() {
		return nil, fmt.Errorf(
			"tzf: DefaultFinder only support same data version for Finder(version=%v) and FuzzyFinder(version=%v)",
			finder.DataVersion(),
			fuzzyFinder.DataVersion(),
		)
	}

	f := &DefaultFinder{}
	f.fuzzyFinder = fuzzyFinder
	f.finder = finder

	// Force free mem by probuf, about 80MB
	runtime.GC()

	return f, nil
}

// newDefaultFinderFromCompressedTopo builds a [DefaultFinder] from the tzf-dist data files:
//   - preindex:       combined-with-oceans.topology.preindex.bin  → [pb.PreindexTimezones]
//   - compressedTopo: combined-with-oceans.topology.compress.topo.bin or
//     combined-with-oceans.compress.topo.bin → [pb.CompressedTopoTimezones]
func newDefaultFinderFromCompressedTopo(preindex *pb.PreindexTimezones, compressedTopo *pb.CompressedTopoTimezones) (F, error) {
	fuzzyFinder, err := NewFuzzyFinderFromPB(preindex)
	if err != nil {
		return nil, err
	}

	finder, err := NewFinderFromCompressedTopo(compressedTopo, SetDropPBTZ)
	if err != nil {
		return nil, err
	}

	if finder.DataVersion() != fuzzyFinder.DataVersion() {
		return nil, fmt.Errorf(
			"tzf: DefaultFinder only support same data version for Finder(version=%v) and FuzzyFinder(version=%v)",
			finder.DataVersion(),
			fuzzyFinder.DataVersion(),
		)
	}

	f := &DefaultFinder{}
	f.fuzzyFinder = fuzzyFinder
	f.finder = finder

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
	return f.finder.GetTimezoneNames(lng, lat)
}

func (f *DefaultFinder) TimezoneNames() []string {
	return f.finder.TimezoneNames()
}

func (f *DefaultFinder) DataVersion() string {
	return f.finder.DataVersion()
}

// GetTZGeoJSON returns a GeoJSON FeatureCollection for the named timezone.
func (f *DefaultFinder) GetTZGeoJSON(tzName string) (*convert.BoundaryFile, error) {
	return f.finder.(*Finder).GetTZGeoJSON(tzName)
}

// GetGeoJSON returns a GeoJSON FeatureCollection covering all timezones.
func (f *DefaultFinder) GetGeoJSON() *convert.BoundaryFile {
	return f.finder.(*Finder).GetGeoJSON()
}
