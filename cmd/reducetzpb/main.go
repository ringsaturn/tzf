// CLI tool to reduce polygon filesize
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/reduce"
	"github.com/ringsaturn/tzf/internal/topology"
	"google.golang.org/protobuf/proto"
)

const (
	SKIP        int     = 5     // At least skip how many point
	PRECISE     float64 = 10000 // round float precise
	MINDISTENCE float64 = 10    // min dist to previous point, except begin&end point
)

func main() {
	topologyAware := flag.Bool("topology", true, "use topology-aware simplification")
	epsilon := flag.Float64("epsilon", 0.001, "douglas-peucker epsilon")
	report := flag.Bool("report", true, "print reduction report to stderr")
	outputPath := flag.String("o", "", "output path (default: input with .bin replaced by .reduce.bin)")
	flag.Parse()
	if flag.NArg() < 1 {
		panic("missing input .bin file path")
	}

	originalProbufPath := flag.Arg(0)
	rawFile, err := os.ReadFile(originalProbufPath)
	if err != nil {
		panic(err)
	}
	input := &pb.Timezones{}
	if err := proto.Unmarshal(rawFile, input); err != nil {
		panic(err)
	}

	var (
		output      *pb.Timezones
		topoStats   topology.Stats
		hasTopoStats bool
	)
	if *topologyAware {
		output, topoStats = reduce.DoTopologyAwareWithStats(input, *epsilon)
		hasTopoStats = true
	} else {
		output = reduce.Do(input, SKIP, PRECISE, MINDISTENCE)
	}

	if *report {
		printReport(os.Stderr, input, output, *topologyAware, *epsilon, topoStats, hasTopoStats)
	}

	dest := *outputPath
	if dest == "" {
		dest = strings.Replace(originalProbufPath, ".bin", ".reduce.bin", 1)
	}
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(dest)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(dest)
}

type datasetStats struct {
	Timezones int
	Polygons  int
	Holes     int
	Points    int
}

func printReport(
	w io.Writer,
	input *pb.Timezones,
	output *pb.Timezones,
	topologyAware bool,
	epsilon float64,
	topoStats topology.Stats,
	hasTopoStats bool,
) {
	before := collectStats(input)
	after := collectStats(output)

	mode := "classic"
	if topologyAware {
		mode = "topology"
	}

	bytesBefore := proto.Size(&pb.Timezones{Timezones: input.Timezones})
	bytesAfter := proto.Size(&pb.Timezones{Timezones: output.Timezones})

	pointRatio := 0.0
	if before.Points > 0 {
		pointRatio = 100 * (1 - float64(after.Points)/float64(before.Points))
	}
	byteRatio := 0.0
	if bytesBefore > 0 {
		byteRatio = 100 * (1 - float64(bytesAfter)/float64(bytesBefore))
	}

	_, _ = fmt.Fprintf(w, "mode: %s\n", mode)
	if topologyAware {
		_, _ = fmt.Fprintf(w, "epsilon: %.6f\n", epsilon)
	}
	_, _ = fmt.Fprintf(w, "dataset_before: timezones=%d polygons=%d holes=%d points=%d bytes=%d\n",
		before.Timezones, before.Polygons, before.Holes, before.Points, bytesBefore)
	_, _ = fmt.Fprintf(w, "dataset_after:  timezones=%d polygons=%d holes=%d points=%d bytes=%d\n",
		after.Timezones, after.Polygons, after.Holes, after.Points, bytesAfter)
	_, _ = fmt.Fprintf(w, "dataset_reduction: points=%.2f%% bytes=%.2f%%\n", pointRatio, byteRatio)
	if hasTopoStats {
		_, _ = fmt.Fprintf(w, "%s\n", topoStats.String())
	}
}

func collectStats(input *pb.Timezones) datasetStats {
	stats := datasetStats{}
	if input == nil {
		return stats
	}

	stats.Timezones = len(input.Timezones)
	for _, timezone := range input.Timezones {
		for _, polygon := range timezone.Polygons {
			stats.Polygons++
			stats.Points += len(polygon.Points)
			stats.Holes += len(polygon.Holes)
			for _, hole := range polygon.Holes {
				stats.Points += len(hole.Points)
			}
		}
	}

	return stats
}
