// CLI tool to preindex timezone shape.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ringsaturn/orb/maptile"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/preindex"
)

var (
	idxZoom            = 13
	aggZoom            = 3
	maxZoomLevelToKeep = 10
	layerDrop          = 2
)

func main() {
	originalProbufPath := os.Args[1]
	rawFile, err := os.ReadFile(originalProbufPath)
	if err != nil {
		panic(err)
	}
	input := &pb.Timezones{}
	if err := pb.Unmarshal(rawFile, input); err != nil {
		panic(err)
	}

	output := preindex.PreIndexTimezones(input, maptile.Zoom(idxZoom), maptile.Zoom(aggZoom), maptile.Zoom(maxZoomLevelToKeep), layerDrop)

	outputPath := strings.Replace(originalProbufPath, ".gob", ".preindex.gob", 1)
	outputBin, _ := pb.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)

	fmt.Fprintf(os.Stderr, "input:  timezones=%d bytes=%d\n", len(input.Timezones), len(rawFile))
	fmt.Fprintf(os.Stderr, "params: idxZoom=%d aggZoom=%d maxZoomLevelToKeep=%d layerDrop=%d\n",
		idxZoom, aggZoom, maxZoomLevelToKeep, layerDrop)
	fmt.Fprintf(os.Stderr, "output: total_keys=%d bytes=%d\n", len(output.Keys), len(outputBin))
}
