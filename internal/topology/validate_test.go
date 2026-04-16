package topology

import (
	"strings"
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

func TestValidate_AcceptsSharedBorders(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 0},
					})},
				},
			},
		},
	}

	if err := Validate(input); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidate_RejectsOpenRing(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Broken",
				Polygons: []*pb.Polygon{
					{Points: []*pb.Point{
						{Lng: 0, Lat: 0},
						{Lng: 1, Lat: 0},
						{Lng: 1, Lat: 1},
						{Lng: 0, Lat: 1},
					}},
				},
			},
		},
	}

	err := Validate(input)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "not closed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsSameDirectionSharedEdge(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "A",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "B",
				Polygons: []*pb.Polygon{
					{
						Points: lineToRing([][2]float32{
							{0, -1}, {3, -1}, {3, 2}, {0, 2}, {0, -1},
						}),
						Holes: []*pb.Polygon{
							{Points: lineToRing([][2]float32{
								{1, 0}, {1, 1}, {2, 1}, {2, 0}, {1, 0},
							})},
						},
					},
				},
			},
		},
	}

	err := Validate(input)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "same direction") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsClockwiseExterior(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Clockwise",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0},
					})},
				},
			},
		},
	}

	err := Validate(input)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "clockwise winding") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsCounterclockwiseHole(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "BadHole",
				Polygons: []*pb.Polygon{
					{
						Points: lineToRing([][2]float32{
							{0, 0}, {3, 0}, {3, 3}, {0, 3}, {0, 0},
						}),
						Holes: []*pb.Polygon{
							{Points: lineToRing([][2]float32{
								{1, 1}, {2, 1}, {2, 2}, {1, 2}, {1, 1},
							})},
						},
					},
				},
			},
		},
	}

	err := Validate(input)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "counterclockwise winding") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsSelfIntersection(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "BowTie",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 2}, {0, 2}, {2, 0}, {0, 0},
					})},
				},
			},
		},
	}

	err := Validate(input)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "self intersects") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWithOptions_AllowsSelfIntersectionForReduction(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "BowTie",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {3, 2}, {0, 3}, {2, 0}, {0, 0},
					})},
				},
			},
		},
	}

	if err := ValidateWithOptions(input, ReductionValidateOptions()); err != nil {
		t.Fatalf("unexpected reduction validation error: %v", err)
	}
}

func TestValidate_AllowsIntentionalThreeWayOverlap(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "A",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
					})},
				},
			},
			{
				Name: "B",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
					})},
				},
			},
			{
				Name: "C",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0},
					})},
				},
			},
		},
	}

	if err := Validate(input); err != nil {
		t.Fatalf("unexpected validation error for three-way overlap: %v", err)
	}
}
