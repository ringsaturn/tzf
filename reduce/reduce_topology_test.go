package reduce

import (
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/topology"
)

func TestDoTopologyAware_ProducesValidTopology(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 0.2}, {1.1, 0.4}, {0.9, 0.6}, {1, 0.8}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 0.8}, {0.9, 0.6}, {1.1, 0.4}, {1, 0.2}, {1, 0},
					})},
				},
			},
		},
	}

	output := DoTopologyAware(input, 0.15)
	if err := topology.Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func lineToRing(coords [][2]float32) []*pb.Point {
	points := make([]*pb.Point, 0, len(coords))
	for _, coord := range coords {
		points = append(points, &pb.Point{Lng: coord[0], Lat: coord[1]})
	}
	return points
}
