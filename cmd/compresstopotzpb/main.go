// CLI tool to apply polyline coordinate compression to a TopoTimezones .topo.bin file.
//
// Usage:
//
//	compresstopotzpb [-o output.compress.topo.gob] input.topo.gob
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/reduce"
)

func main() {
	outputPath := flag.String("o", "", "output path (default: input with .topo.gob replaced by .compress.topo.gob)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: compresstopotzpb [-o output.compress.topo.gob] input.topo.gob")
		os.Exit(1)
	}

	inputPath := flag.Arg(0)
	rawFile, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}
	input := &pb.TopoTimezones{}
	if err := pb.Unmarshal(rawFile, input); err != nil {
		fmt.Fprintf(os.Stderr, "error unmarshaling input: %v\n", err)
		os.Exit(1)
	}

	output := reduce.CompressTopoTimezones(input)

	dest := *outputPath
	if dest == "" {
		dest = strings.Replace(inputPath, ".topo.gob", ".compress.topo.gob", 1)
		if dest == inputPath {
			dest = inputPath + ".compress.topo.gob"
		}
	}

	outputBin, err := pb.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling output: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dest, outputBin, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}

	inputBytes := len(rawFile)
	outputBytes := len(outputBin)
	reduction := 100 * (1 - float64(outputBytes)/float64(inputBytes))
	fmt.Fprintf(os.Stderr, "input:  bytes=%d\n", inputBytes)
	fmt.Fprintf(os.Stderr, "output: bytes=%d\n", outputBytes)
	fmt.Fprintf(os.Stderr, "reduction: bytes=%.2f%%\n", reduction)
	fmt.Println(dest)
}
