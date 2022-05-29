// CLI tool to convert GeoJSON based Timezone boundary to tzf's Probuf format.
package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/convert"
	"google.golang.org/protobuf/proto"
)

func main() {
	jsonFilePath := os.Args[1]

	rawFile, err := ioutil.ReadFile(jsonFilePath)
	if err != nil {
		panic(err)
	}

	boundaryFile := &convert.BoundaryFile{}
	if err := json.Unmarshal(rawFile, boundaryFile); err != nil {
		panic(err)
	}

	output, err := convert.Do(boundaryFile)
	if err != nil {
		panic(err)
	}
	outputPath := strings.Replace(jsonFilePath, ".json", ".pb", 1)
	outputBin, _ := proto.Marshal(output)

	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
}
