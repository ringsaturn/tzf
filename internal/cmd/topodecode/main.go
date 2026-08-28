// Decode a CompressedTopoTimezones file back into flat Timezones.
package main

import (
	"fmt"
	"os"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/reduce"
	"github.com/ringsaturn/tzf/v2/internal/topology"
)

func main() {
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	compressed := &pb.CompressedTopoTimezones{}
	if err := pb.Unmarshal(raw, compressed); err != nil {
		panic(err)
	}
	flat := topology.DecodeTopoTimezones(reduce.DecompressTopoTimezones(compressed))
	out, err := pb.Marshal(flat)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[2], out, 0o644); err != nil {
		panic(err)
	}
	var points int
	for _, tz := range flat.Timezones {
		for _, polygon := range tz.Polygons {
			points += len(polygon.Points)
			for _, hole := range polygon.Holes {
				points += len(hole.Points)
			}
		}
	}
	fmt.Println("version:", flat.Version, "timezones:", len(flat.Timezones), "points:", points)
}
