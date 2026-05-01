// CLI tool to embed a 1°×1° grid candidate-reduction index into an existing
// CompressedTopoTimezones protobuf file.
//
// Usage:
//
//	buildgridtzpb [-o output.compress.topo.bin] input.compress.topo.bin
//
// The tool reads a CompressedTopoTimezones, builds the GridIndex from the
// embedded timezone bboxes, sets the grid_index field, and writes the updated
// protobuf. When -o is omitted the input file is overwritten in place.
package main

import (
	"flag"
	"fmt"
	"os"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/gridindex"
	"google.golang.org/protobuf/proto"
)

func main() {
	outputPath := flag.String("o", "", "output path (default: overwrite input file)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: buildgridtzpb [-o output.compress.topo.bin] input.compress.topo.bin")
		os.Exit(1)
	}

	inputPath := flag.Arg(0)
	rawFile, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}

	compTopo := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(rawFile, compTopo); err != nil || len(compTopo.Timezones) == 0 {
		fmt.Fprintf(os.Stderr, "error: input is not a valid CompressedTopoTimezones: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "input: timezones=%d shared_edges=%d bytes=%d\n",
		len(compTopo.Timezones), len(compTopo.SharedEdges), len(rawFile))

	compTopo.GridIndex = gridindex.BuildFromCompressedTopoTimezones(compTopo)
	fmt.Fprintf(os.Stderr, "grid: cells=%d\n", len(compTopo.GridIndex.Cells))

	outputBin, err := proto.Marshal(compTopo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling output: %v\n", err)
		os.Exit(1)
	}

	dest := *outputPath
	if dest == "" {
		dest = inputPath
	}

	if err := os.WriteFile(dest, outputBin, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", len(outputBin), dest)
}
