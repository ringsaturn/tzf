// CLI tool to reduce polygon filesize
package main

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/reduce"
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
	output := reduce.CompressWithPolyline(input)

	outputPath := strings.Replace(originalProbufPath, ".pb", ".compress.pb", 1)
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
