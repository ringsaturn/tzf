// i32compare cross-checks the scaled-integer Finder (NewFinderFromCompressedTopo)
// against the float reference path (DecompressTopoTimezones → DecodeTopoTimezones
// → NewFinderFromPB, which round-trips coordinates through float32 pb.Point).
//
// GridIndex is stripped from the integer finder so both sides run the same
// full-scan lookup. Mismatches are expected only within ~1e-6° of a border,
// where the old float32 rounding moved the boundary.
package main

import (
	"fmt"
	"math/rand"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/topology"
	"github.com/ringsaturn/tzf/reduce"
	"google.golang.org/protobuf/proto"
)

func main() {
	newInt32Finder := func() tzf.F {
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			panic(err)
		}
		input.GridIndex = nil
		f, err := tzf.NewFinderFromCompressedTopo(input)
		if err != nil {
			panic(err)
		}
		return f
	}

	newFloatReference := func() tzf.F {
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			panic(err)
		}
		flat := topology.DecodeTopoTimezones(reduce.DecompressTopoTimezones(input))
		f, err := tzf.NewFinderFromPB(flat)
		if err != nil {
			panic(err)
		}
		return f
	}

	intF := newInt32Finder()
	refF := newFloatReference()

	rng := rand.New(rand.NewSource(42))
	const n = 500000
	diff := 0
	for range n {
		lng := rng.Float64()*360 - 180
		lat := rng.Float64()*180 - 90
		a := intF.GetTimezoneName(lng, lat)
		b := refF.GetTimezoneName(lng, lat)
		if a != b {
			diff++
			if diff <= 20 {
				fmt.Printf("DIFF\t(%.7f, %.7f)\tint32=%q\tfloat32=%q\n", lng, lat, a, b)
			}
		}
	}
	fmt.Printf("random: %d/%d mismatches\n", diff, n)

	// Grid-snapped points sit exactly on 1e-5 vertices/edges, the worst case
	// for representation differences.
	diff = 0
	const m = 200000
	for range m {
		lng := float64(rng.Intn(360*100000)-180*100000) / 1e5
		lat := float64(rng.Intn(180*100000)-90*100000) / 1e5
		a := intF.GetTimezoneName(lng, lat)
		b := refF.GetTimezoneName(lng, lat)
		if a != b {
			diff++
			if diff <= 20 {
				fmt.Printf("DIFF-GRID\t(%.5f, %.5f)\tint32=%q\tfloat32=%q\n", lng, lat, a, b)
			}
		}
	}
	fmt.Printf("grid-snapped: %d/%d mismatches\n", diff, m)
}
