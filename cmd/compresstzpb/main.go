// CLI tool to reduce polygon filesize
package main

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/reduce"
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
	output := reduce.CompressWithPolyline(input)

	outputPath := strings.Replace(originalProbufPath, ".bin", ".compress.bin", 1)
	outputBin, _ := pb.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
