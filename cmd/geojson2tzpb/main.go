// CLI tool to convert GeoJSON timezone boundaries to the pipeline's
// intermediate format.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/v2/internal/convert"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

func main() {
	jsonFilePath := os.Args[1]

	rawFile, err := os.ReadFile(jsonFilePath)
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
	outputPath := strings.Replace(jsonFilePath, ".json", ".gob", 1)
	outputBin, err := pb.Marshal(output)
	if err != nil {
		panic(err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
