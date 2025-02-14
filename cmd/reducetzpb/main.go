// CLI tool to reduce polygon filesize
package main

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/gen/go/pb/v1"
	"github.com/ringsaturn/tzf/reduce"
	"google.golang.org/protobuf/proto"
)

const (
	SKIP        int     = 5     // At least skip how many point
	PRECISE     float64 = 10000 // round float precise
	MINDISTENCE float64 = 10    // min dist to previous point, except begin&end point
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
	output := reduce.Do(input, SKIP, PRECISE, MINDISTENCE)

	outputPath := strings.Replace(originalProbufPath, ".pb", ".reduce.pb", 1)
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
	fmt.Println(outputPath)
}
