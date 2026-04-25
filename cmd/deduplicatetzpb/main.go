// CLI tool to convert a Timezones .bin file into the topology-aware
// TopoTimezones format where shared boundary edges are stored only once.
//
// Usage:
//
//	deduplicatetzpb [-o output.topo.bin] [-report] input.bin
//
// The output format uses the TopoTimezones protobuf schema with a global
// shared-edge library. Adjacent timezone boundaries that appear in multiple
// polygons are stored exactly once and referenced by ID, reducing the ~96 MB
// full dataset by approximately 30–35 MB while preserving full geometric
// accuracy.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/topology"
	"google.golang.org/protobuf/proto"
)

func main() {
	outputPath := flag.String("o", "", "output path (default: input with .bin replaced by .topo.bin)")
	report := flag.Bool("report", true, "print deduplication report to stderr")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: deduplicatetzpb [-o output.topo.bin] [-report] input.bin")
		os.Exit(1)
	}

	inputPath := flag.Arg(0)
	rawFile, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}
	input := &pb.Timezones{}
	if err := proto.Unmarshal(rawFile, input); err != nil {
		fmt.Fprintf(os.Stderr, "error unmarshaling input: %v\n", err)
		os.Exit(1)
	}

	output := topology.BuildTopoTimezones(input)

	if *report {
		printReport(os.Stderr, input, output, len(rawFile))
	}

	dest := *outputPath
	if dest == "" {
		dest = strings.Replace(inputPath, ".bin", ".topo.bin", 1)
		if dest == inputPath {
			dest = inputPath + ".topo.bin"
		}
	}

	outputBin, err := proto.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling output: %v\n", err)
		os.Exit(1)
	}
	f, err := os.Create(dest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating output file: %v\n", err)
		os.Exit(1)
	}
	if _, err := f.Write(outputBin); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
	_ = f.Close()
	fmt.Println(dest)
}

func printReport(w io.Writer, input *pb.Timezones, output *pb.TopoTimezones, inputFileBytes int) {
	// Count input statistics.
	inputTimezones := len(input.Timezones)
	inputPolygons, inputHoles, inputPoints := 0, 0, 0
	for _, tz := range input.Timezones {
		for _, poly := range tz.Polygons {
			inputPolygons++
			inputPoints += len(poly.Points)
			inputHoles += len(poly.Holes)
			for _, hole := range poly.Holes {
				inputPoints += len(hole.Points)
			}
		}
	}

	// Count output statistics.
	sharedEdges := len(output.SharedEdges)
	sharedPoints := 0
	for _, e := range output.SharedEdges {
		sharedPoints += len(e.Points)
	}

	inlineSegments, edgeRefSegments := 0, 0
	for _, tz := range output.Timezones {
		for _, poly := range tz.Polygons {
			countSegments(poly.Exterior, &inlineSegments, &edgeRefSegments)
			for _, hole := range poly.Holes {
				countSegments(hole.Exterior, &inlineSegments, &edgeRefSegments)
			}
		}
	}

	outputBytes := proto.Size(output)
	byteReduction := 0.0
	if inputFileBytes > 0 {
		byteReduction = 100 * (1 - float64(outputBytes)/float64(inputFileBytes))
	}

	_, _ = fmt.Fprintf(w, "input:  timezones=%d polygons=%d holes=%d points=%d bytes=%d\n",
		inputTimezones, inputPolygons, inputHoles, inputPoints, inputFileBytes)
	_, _ = fmt.Fprintf(w, "output: shared_edges=%d shared_points=%d inline_segs=%d edge_ref_segs=%d bytes=%d\n",
		sharedEdges, sharedPoints, inlineSegments, edgeRefSegments, outputBytes)
	_, _ = fmt.Fprintf(w, "reduction: bytes=%.2f%%\n", byteReduction)
	if inlineSegments+edgeRefSegments > 0 {
		dedupPct := 100.0 * float64(edgeRefSegments) / float64(inlineSegments+edgeRefSegments)
		_, _ = fmt.Fprintf(w, "dedup_rate: %.2f%% of segments reference shared edges\n", dedupPct)
	}
}

func countSegments(segs []*pb.RingSegment, inline, edgeRef *int) {
	for _, seg := range segs {
		switch seg.Content.(type) {
		case *pb.RingSegment_Inline:
			*inline++
		case *pb.RingSegment_EdgeForward, *pb.RingSegment_EdgeReversed:
			*edgeRef++
		}
	}
}
