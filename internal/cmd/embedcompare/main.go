// Command embedcompare performs deep and query parity validation for .tzb data.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"slices"

	"github.com/ringsaturn/tzf"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"github.com/ringsaturn/tzf/internal/polyline"
	"google.golang.org/protobuf/proto"
)

type point struct{ lng, lat float64 }

func main() {
	step := flag.Float64("dense-step", 0.1, "global grid spacing in degrees, 0 disables")
	boundarySamples := flag.Int("boundary-samples", 50000, "number of boundary-biased samples")
	seed := flag.Int64("seed", 42, "random seed")
	deepOnly := flag.Bool("deep-only", false, "run semantic verification without query sampling")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: embedcompare [flags] input.compress.topo.bin input.tzb")
		os.Exit(2)
	}
	sourceRaw, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(sourceRaw, &input); err != nil {
		fail(err)
	}
	tzb, err := os.ReadFile(flag.Arg(1))
	if err != nil {
		fail(err)
	}
	reader, err := embedbin.Open(tzb)
	if err != nil {
		fail(err)
	}
	if err := embedbin.Verify(&input, reader); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "deep verification passed: version=%s timezones=%d bytes=%d\n",
		reader.DataVersion(), reader.TimezoneCount(), len(tzb))
	if *deepOnly {
		return
	}

	referenceInput := proto.Clone(&input).(*pb.CompressedTopoTimezones)
	if !reader.ShortcutEnabled() {
		referenceInput.GridIndex = nil
	}
	reference, err := tzf.NewFinderFromCompressedTopo(referenceInput)
	if err != nil {
		fail(err)
	}
	dst := make([]int32, 0, reader.TimezoneCount())
	checked := 0
	if *step > 0 {
		for lat := -90.0; lat <= 90.0+1e-12; lat += *step {
			for lng := -180.0; lng <= 180.0+1e-12; lng += *step {
				if err := compareSingle(reader, reference, lng, lat); err != nil {
					fail(err)
				}
				checked++
			}
		}
		fmt.Fprintf(os.Stderr, "dense parity passed: points=%d step=%g\n", checked, *step)
	}

	rng := rand.New(rand.NewSource(*seed))
	reservoir := collectBoundaryReservoir(&input, *boundarySamples, rng)
	for _, p := range reservoir {
		lng := p.lng + (rng.Float64()-0.5)*0.01
		lat := p.lat + (rng.Float64()-0.5)*0.01
		if lng < -180 || lng > 180 || lat < -90 || lat > 90 {
			continue
		}
		if err := compareSingle(reader, reference, lng, lat); err != nil {
			fail(err)
		}
		got, err := reader.LookupInto(lng, lat, dst)
		if err != nil {
			fail(err)
		}
		want, err := reference.GetTimezoneNames(lng, lat)
		if err != nil {
			fail(err)
		}
		gotNames := make([]string, len(got))
		for i, idx := range got {
			name, err := reader.Name(idx)
			if err != nil {
				fail(err)
			}
			gotNames[i] = string(name)
		}
		if !slices.Equal(gotNames, want) {
			fail(fmt.Errorf("multi parity at (%f,%f): got %v want %v", lng, lat, gotNames, want))
		}
	}
	fmt.Fprintf(os.Stderr, "boundary parity passed: points=%d seed=%d\n", len(reservoir), *seed)
}

func compareSingle(reader *embedbin.Reader, reference tzf.F, lng, lat float64) error {
	idx, ok, err := reader.Lookup(lng, lat)
	if err != nil {
		return err
	}
	var got string
	if ok {
		name, err := reader.Name(idx)
		if err != nil {
			return err
		}
		got = string(name)
	}
	want := reference.GetTimezoneName(lng, lat)
	if got != want {
		return fmt.Errorf("single parity at (%f,%f): got %q want %q", lng, lat, got, want)
	}
	return nil
}

func collectBoundaryReservoir(input *pb.CompressedTopoTimezones, limit int, rng *rand.Rand) []point {
	if limit <= 0 {
		return nil
	}
	out := make([]point, 0, limit)
	seen := 0
	addBytes := func(data []byte) {
		coords, err := polyline.DecodeCoordsInt32(data)
		if err != nil {
			fail(err)
		}
		for _, coord := range coords {
			p := point{lng: float64(coord[0]) / 1e5, lat: float64(coord[1]) / 1e5}
			seen++
			if len(out) < limit {
				out = append(out, p)
			} else if j := rng.Intn(seen); j < limit {
				out[j] = p
			}
		}
	}
	for _, edge := range input.SharedEdges {
		addBytes(edge.Points)
	}
	var walk func(*pb.CompressedTopoPolygon)
	walk = func(poly *pb.CompressedTopoPolygon) {
		for _, segment := range poly.Exterior {
			if in, ok := segment.Content.(*pb.CompressedRingSegment_Inline); ok && in.Inline != nil {
				addBytes(in.Inline.Points)
			}
		}
		for _, hole := range poly.Holes {
			walk(hole)
		}
	}
	for _, timezone := range input.Timezones {
		for _, poly := range timezone.Polygons {
			walk(poly)
		}
	}
	return out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "embedcompare:", err)
	os.Exit(1)
}
