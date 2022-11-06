// CLI tool to reduce polygon filesize
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

	IdxZoom := 11
	AggZoom := 3
	layerDrop := 1

	output := &pb.PreindexTimezones{
		IdxZoom: int32(IdxZoom),
		AggZoom: int32(AggZoom),
		Keys:    make([]*pb.PreindexTimezone, 0),
	}

	for _, tz := range input.Timezones {
		fmt.Println(tz.Name)
		tiles, err := preindex.PreIndexTimezone(tz, maptile.Zoom(IdxZoom), maptile.Zoom(AggZoom), layerDrop)
		if err != nil {
			continue
		}
		output.Keys = append(output.Keys, tiles...)
	}

	outputPath := strings.Replace(originalProbufPath, ".pb", ".preindex.pb", 1)
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
