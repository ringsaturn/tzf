// CLI tool to preindex timezone shape.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/paulmach/orb/maptile"
	"github.com/ringsaturn/tzf/pb"
	"github.com/ringsaturn/tzf/preindex"
	"google.golang.org/protobuf/proto"
)

var (
	idxZoom            = 11
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
	if err := proto.Unmarshal(rawFile, input); err != nil {
		panic(err)
	}

	output := preindex.PreIndexTimezones(input, maptile.Zoom(idxZoom), maptile.Zoom(aggZoom), maptile.Zoom(maxZoomLevelToKeep), layerDrop)

	outputPath := strings.Replace(originalProbufPath, ".pb", ".preindex.pb", 1)
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
