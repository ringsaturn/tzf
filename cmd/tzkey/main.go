package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/pb"
	"github.com/ringsaturn/tzf/tzkey"
	"google.golang.org/protobuf/proto"
)

func main() {
	originalProbufPath := os.Args[1]
	rawFile, err := ioutil.ReadFile(originalProbufPath)
	if err != nil {
		panic(err)
	}
	input := &pb.Timezones{}
	if err := proto.Unmarshal(rawFile, input); err != nil {
		panic(err)
	}

	output := tzkey.Do(input, 5)

	outputPath := strings.Replace(originalProbufPath, ".pb", ".tzkey.json", 1)
	outputBin, _ := json.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)
}
