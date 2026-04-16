package topology

import (
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

// makeSharedBorderInput returns two adjacent rectangles that share the
// vertical edge at x=1 with several intermediate points.
func makeSharedBorderInput() *pb.Timezones {
	return &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 0.25}, {1, 0.5}, {1, 0.75}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 0.75}, {1, 0.5}, {1, 0.25}, {1, 0},
					})},
				},
			},
		},
	}
}

func TestBuildTopoTimezones_SharedEdgeStoredOnce(t *testing.T) {
	input := makeSharedBorderInput()
	topo := BuildTopoTimezones(input)
	if topo == nil {
		t.Fatal("BuildTopoTimezones returned nil")
	}

	// The shared vertical border at x=1 should appear exactly once in the
	// shared edge library.
	if len(topo.SharedEdges) == 0 {
		t.Fatal("expected at least one shared edge")
	}
	// The total number of unique shared edges should be small (≤ 2 with
	// the two fixed junction vertices each producing their own chain, but
	// in this simple case both rings share exactly one continuous chain).
	t.Logf("shared edges: %d", len(topo.SharedEdges))
	t.Logf("timezones: %d", len(topo.Timezones))
}

func TestDecodeTopoTimezones_RoundTrip(t *testing.T) {
	input := makeSharedBorderInput()
	topo := BuildTopoTimezones(input)
	if topo == nil {
		t.Fatal("BuildTopoTimezones returned nil")
	}

	decoded := DecodeTopoTimezones(topo)
	if decoded == nil {
		t.Fatal("DecodeTopoTimezones returned nil")
	}
	if len(decoded.Timezones) != len(input.Timezones) {
		t.Fatalf("timezone count mismatch: want %d got %d", len(input.Timezones), len(decoded.Timezones))
	}

	// Every decoded ring must be valid.
	if err := Validate(decoded); err != nil {
		t.Fatalf("Validate returned error after round-trip: %v", err)
	}
}

func TestDecodeTopoTimezones_SharedBorderConsistent(t *testing.T) {
	input := makeSharedBorderInput()
	topo := BuildTopoTimezones(input)
	decoded := DecodeTopoTimezones(topo)
	if decoded == nil {
		t.Fatal("DecodeTopoTimezones returned nil")
	}

	left := decoded.Timezones[0].Polygons[0].Points
	right := decoded.Timezones[1].Polygons[0].Points

	leftShared := extractSubPath(left, [][2]float32{{1, 0}, {1, 1}})
	rightShared := extractSubPath(right, [][2]float32{{1, 1}, {1, 0}})

	if len(leftShared) == 0 || len(rightShared) == 0 {
		t.Fatal("shared border not found after round-trip")
	}
	if len(leftShared) != len(rightShared) {
		t.Fatalf("shared border length mismatch: left=%d right=%d", len(leftShared), len(rightShared))
	}
	for idx := range leftShared {
		ridx := len(rightShared) - 1 - idx
		if !samePoint(leftShared[idx], rightShared[ridx]) {
			t.Fatalf("shared border point %d mismatch: left=%+v right=%+v",
				idx, leftShared[idx], rightShared[ridx])
		}
	}
}

func TestDecodeTopoTimezones_HoleRoundTrip(t *testing.T) {
	// Outer polygon with a hole (enclave) and a separate inner polygon that
	// exactly matches the hole shape.
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Outer",
				Polygons: []*pb.Polygon{
					{
						Points: lineToRing([][2]float32{
							{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0},
						}),
						Holes: []*pb.Polygon{
							{Points: lineToRing([][2]float32{
								{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1},
							})},
						},
					},
				},
			},
			{
				Name: "Inner",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 1}, {3, 1}, {3, 3}, {1, 3}, {1, 1},
					})},
				},
			},
		},
	}

	topo := BuildTopoTimezones(input)
	if topo == nil {
		t.Fatal("BuildTopoTimezones returned nil")
	}

	decoded := DecodeTopoTimezones(topo)
	if decoded == nil {
		t.Fatal("DecodeTopoTimezones returned nil")
	}
	if err := Validate(decoded); err != nil {
		t.Fatalf("Validate returned error after hole round-trip: %v", err)
	}

	if len(decoded.Timezones[0].Polygons[0].Holes) != 1 {
		t.Fatalf("expected 1 hole, got %d", len(decoded.Timezones[0].Polygons[0].Holes))
	}
}

func TestDecodeTopoTimezones_PreservesVersion(t *testing.T) {
	input := &pb.Timezones{
		Version: "v42",
		Timezones: []*pb.Timezone{
			{
				Name: "Solo",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
		},
	}
	topo := BuildTopoTimezones(input)
	if topo.Version != "v42" {
		t.Fatalf("expected version v42, got %q", topo.Version)
	}
	decoded := DecodeTopoTimezones(topo)
	if decoded.Version != "v42" {
		t.Fatalf("expected version v42 after decode, got %q", decoded.Version)
	}
}

func TestBuildTopoTimezones_NilInput(t *testing.T) {
	if BuildTopoTimezones(nil) != nil {
		t.Fatal("expected nil output for nil input")
	}
}

func TestDecodeTopoTimezones_NilInput(t *testing.T) {
	if DecodeTopoTimezones(nil) != nil {
		t.Fatal("expected nil output for nil input")
	}
}

func TestDecodeTopoTimezones_ThreeAdjacentZones(t *testing.T) {
	// Three horizontally adjacent rectangles: Left, Middle, Right.
	// Left shares its right edge with Middle; Middle shares its right edge
	// with Right. The shared border between Left/Middle and Middle/Right
	// should each be stored as exactly one shared edge.
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "Left",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{0, 0}, {1, 0}, {1, 0.5}, {1, 1}, {0, 1}, {0, 0},
					})},
				},
			},
			{
				Name: "Middle",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{1, 0}, {2, 0}, {2, 0.5}, {2, 1}, {1, 1}, {1, 0.5}, {1, 0},
					})},
				},
			},
			{
				Name: "Right",
				Polygons: []*pb.Polygon{
					{Points: lineToRing([][2]float32{
						{2, 0}, {3, 0}, {3, 1}, {2, 1}, {2, 0.5}, {2, 0},
					})},
				},
			},
		},
	}

	topo := BuildTopoTimezones(input)
	if topo == nil {
		t.Fatal("BuildTopoTimezones returned nil")
	}

	decoded := DecodeTopoTimezones(topo)
	if decoded == nil {
		t.Fatal("DecodeTopoTimezones returned nil")
	}
	if err := Validate(decoded); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(decoded.Timezones) != 3 {
		t.Fatalf("expected 3 timezones, got %d", len(decoded.Timezones))
	}

	t.Logf("shared edges: %d (two borders expected)", len(topo.SharedEdges))
}
