// Command embedcompare performs deep and query parity validation for .tzb data.
package main

import (
	"errors"
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

// checker bundles the query-parity pairs exercised on every sample point:
// the in-place reader, the expansion loader, (when a preindex is given) the
// FUZZY view, and (when a .tzm is given) the M-profile finder, each against
// its protobuf-backed reference.
type checker struct {
	reader      *embedbin.Reader
	reference   tzf.F
	expanded    tzf.F
	expandedRef tzf.F
	fuzzy       tzf.F
	fuzzyRef    tzf.F
	mFinder     tzf.F
	dst         []int32
}

func main() {
	step := flag.Float64("dense-step", 0.1, "global grid spacing in degrees, 0 disables")
	boundarySamples := flag.Int("boundary-samples", 50000, "number of boundary-biased samples")
	seed := flag.Int64("seed", 42, "random seed")
	deepOnly := flag.Bool("deep-only", false, "run semantic verification without query sampling")
	preindexPath := flag.String("preindex", "", "source PreindexTimezones .bin for FUZZY parity")
	tzmPath := flag.String("tzm", "", "M-profile .tzm built from the same source, for Phase 2 parity")
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
	fmt.Fprintf(os.Stderr, "deep verification passed: version=%s timezones=%d bytes=%d fuzzy=%v\n",
		reader.DataVersion(), reader.TimezoneCount(), len(tzb), reader.HasFuzzy())
	var tzmData []byte
	if *tzmPath != "" {
		tzmData, err = os.ReadFile(*tzmPath)
		if err != nil {
			fail(err)
		}
		mReader, err := embedbin.Open(tzmData)
		if err != nil {
			fail(err)
		}
		if err := embedbin.VerifyM(&input, mReader); err != nil {
			fail(err)
		}
		fmt.Fprintf(os.Stderr, "deep M verification passed: bytes=%d fuzzy=%v\n", len(tzmData), mReader.HasFuzzy())
	}
	if *deepOnly {
		return
	}

	c := &checker{reader: reader, dst: make([]int32, 0, reader.TimezoneCount())}
	if tzmData != nil {
		c.mFinder, err = tzf.NewFinderFromTZM(tzmData)
		if err != nil {
			fail(err)
		}
	}
	referenceInput := proto.Clone(&input).(*pb.CompressedTopoTimezones)
	if !reader.ShortcutEnabled() {
		referenceInput.GridIndex = nil
	}
	c.reference, err = tzf.NewFinderFromCompressedTopo(referenceInput)
	if err != nil {
		fail(err)
	}
	// The expansion parity contract is against NewFinderFromCompressedTopo
	// over the unmodified source (grid included): the shortcut flag governs
	// only the in-place reader.
	c.expanded, err = tzf.NewFinderFromTZBExpanded(tzb)
	if err != nil {
		fail(err)
	}
	c.expandedRef, err = tzf.NewFinderFromCompressedTopo(&input)
	if err != nil {
		fail(err)
	}
	if *preindexPath != "" {
		if !reader.HasFuzzy() {
			fail(errors.New("-preindex given but .tzb has no FUZZY section"))
		}
		preRaw, err := os.ReadFile(*preindexPath)
		if err != nil {
			fail(err)
		}
		preindex := &pb.PreindexTimezones{}
		if err := proto.Unmarshal(preRaw, preindex); err != nil {
			fail(err)
		}
		c.fuzzy, err = tzf.NewFuzzyFinderFromTZB(tzb)
		if err != nil {
			fail(err)
		}
		c.fuzzyRef, err = tzf.NewFuzzyFinderFromPB(preindex)
		if err != nil {
			fail(err)
		}
	}

	checked := 0
	if *step > 0 {
		for lat := -90.0; lat <= 90.0+1e-12; lat += *step {
			for lng := -180.0; lng <= 180.0+1e-12; lng += *step {
				if err := c.compareSingle(lng, lat); err != nil {
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
		if err := c.compareSingle(lng, lat); err != nil {
			fail(err)
		}
		if err := c.compareMulti(lng, lat); err != nil {
			fail(err)
		}
	}
	fmt.Fprintf(os.Stderr, "boundary parity passed: points=%d seed=%d\n", len(reservoir), *seed)
}

func (c *checker) compareSingle(lng, lat float64) error {
	idx, ok, err := c.reader.Lookup(lng, lat)
	if err != nil {
		return err
	}
	var got string
	if ok {
		name, err := c.reader.Name(idx)
		if err != nil {
			return err
		}
		got = string(name)
	}
	want := c.reference.GetTimezoneName(lng, lat)
	if got != want {
		return fmt.Errorf("single parity at (%f,%f): got %q want %q", lng, lat, got, want)
	}
	if got, want := c.expanded.GetTimezoneName(lng, lat), c.expandedRef.GetTimezoneName(lng, lat); got != want {
		return fmt.Errorf("expanded single parity at (%f,%f): got %q want %q", lng, lat, got, want)
	}
	if c.mFinder != nil {
		if got, want := c.mFinder.GetTimezoneName(lng, lat), c.expandedRef.GetTimezoneName(lng, lat); got != want {
			return fmt.Errorf("tzm single parity at (%f,%f): got %q want %q", lng, lat, got, want)
		}
	}
	if c.fuzzy != nil {
		if got, want := c.fuzzy.GetTimezoneName(lng, lat), c.fuzzyRef.GetTimezoneName(lng, lat); got != want {
			return fmt.Errorf("fuzzy single parity at (%f,%f): got %q want %q", lng, lat, got, want)
		}
	}
	return nil
}

func (c *checker) compareMulti(lng, lat float64) error {
	got, err := c.reader.LookupInto(lng, lat, c.dst)
	if err != nil {
		return err
	}
	want, err := c.reference.GetTimezoneNames(lng, lat)
	if err != nil {
		return err
	}
	gotNames := make([]string, len(got))
	for i, idx := range got {
		name, err := c.reader.Name(idx)
		if err != nil {
			return err
		}
		gotNames[i] = string(name)
	}
	if !slices.Equal(gotNames, want) {
		return fmt.Errorf("multi parity at (%f,%f): got %v want %v", lng, lat, gotNames, want)
	}
	expGot, err := c.expanded.GetTimezoneNames(lng, lat)
	if err != nil {
		return err
	}
	expWant, err := c.expandedRef.GetTimezoneNames(lng, lat)
	if err != nil {
		return err
	}
	if !slices.Equal(expGot, expWant) {
		return fmt.Errorf("expanded multi parity at (%f,%f): got %v want %v", lng, lat, expGot, expWant)
	}
	if c.mFinder != nil {
		mGot, err := c.mFinder.GetTimezoneNames(lng, lat)
		if err != nil {
			return err
		}
		if !slices.Equal(mGot, expWant) {
			return fmt.Errorf("tzm multi parity at (%f,%f): got %v want %v", lng, lat, mGot, expWant)
		}
	}
	if c.fuzzy != nil {
		fzGot, gotErr := c.fuzzy.GetTimezoneNames(lng, lat)
		fzWant, wantErr := c.fuzzyRef.GetTimezoneNames(lng, lat)
		if (gotErr == nil) != (wantErr == nil) {
			return fmt.Errorf("fuzzy multi parity at (%f,%f): errors %v vs %v", lng, lat, gotErr, wantErr)
		}
		if !slices.Equal(fzGot, fzWant) {
			return fmt.Errorf("fuzzy multi parity at (%f,%f): got %v want %v", lng, lat, fzGot, fzWant)
		}
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
