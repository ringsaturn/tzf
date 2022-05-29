package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/ringsaturn/tzf"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

func ConvertBoundfileToPbTimezones(input *tzf.BoundaryFile) []*pb.Timezone {
	output := make([]*pb.Timezone, 0)

	for _, item := range input.Features {
		pbtzItem := &pb.Timezone{
			Tzid: item.Properties.Tzid,
		}
		output = append(output, pbtzItem)
	}

	return output
}

func main() {
	jsonFilePath := os.Args[1]

	rawFile, err := ioutil.ReadFile(jsonFilePath)
	if err != nil {
		panic(err)
	}

	boundaryFile := &tzf.BoundaryFile{}
	if err := json.Unmarshal(rawFile, boundaryFile); err != nil {
		panic(err)
	}
	log.Println(len(boundaryFile.Features))

	output := &pb.Timezones{}
	output.Timezones = ConvertBoundfileToPbTimezones(boundaryFile)
	outputPath := strings.Replace(jsonFilePath, ".json", ".pb", 1)
	outputBin, _ := proto.Marshal(output)

	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	f.Write(outputBin)
}
