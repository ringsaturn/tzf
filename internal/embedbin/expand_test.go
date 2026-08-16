package embedbin

import (
	"slices"
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
	"github.com/ringsaturn/tzf/internal/polyline"
)

func inlineSegment(points ...[2]float64) *pb.CompressedRingSegment {
	coords := make([][]float64, len(points))
	for i, p := range points {
		coords[i] = []float64{p[0], p[1]}
	}
	return &pb.CompressedRingSegment{
		Content: &pb.CompressedRingSegment_Inline{
			Inline: &pb.CompressedInlinePoints{Points: polyline.EncodeCoords(coords)},
		},
	}
}

func scaled(points ...[2]float64) []geom.I32Point {
	out := make([]geom.I32Point, len(points))
	for i, p := range points {
		out[i] = geom.I32Point{X: int32(p[0] * 1e5), Y: int32(p[1] * 1e5)}
	}
	return out
}

// sharedEdgeFixture builds two squares that share the (5,0)-(5,5)-(5,10)
// meridian edge, referenced forward by A and reversed by B.
func sharedEdgeFixture() *pb.CompressedTopoTimezones {
	edge := polyline.EncodeCoords([][]float64{{5, 0}, {5, 5}, {5, 10}})
	return &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: "edges",
		SharedEdges: []*pb.CompressedSharedEdge{
			{Id: 0, Points: edge},
		},
		Timezones: []*pb.CompressedTopoTimezone{
			{
				Name: "A/West",
				Polygons: []*pb.CompressedTopoPolygon{{
					Exterior: []*pb.CompressedRingSegment{
						inlineSegment([2]float64{0, 10}, [2]float64{0, 0}, [2]float64{5, 0}),
						{Content: &pb.CompressedRingSegment_EdgeForward{EdgeForward: 0}},
						inlineSegment([2]float64{5, 10}, [2]float64{0, 10}),
					},
				}},
			},
			{
				Name: "B/East",
				Polygons: []*pb.CompressedTopoPolygon{{
					Exterior: []*pb.CompressedRingSegment{
						inlineSegment([2]float64{5, 0}, [2]float64{10, 0}, [2]float64{10, 10}, [2]float64{5, 10}),
						{Content: &pb.CompressedRingSegment_EdgeReversed{EdgeReversed: 0}},
					},
				}},
			},
		},
	}
}

func TestExpandJunctionSkipping(t *testing.T) {
	_, r := openFixture(t, sharedEdgeFixture(), EncodeOptions{})
	ex, err := r.Expand()
	if err != nil {
		t.Fatal(err)
	}
	if ex.Version != "edges" || !slices.Equal(ex.Names, []string{"A/West", "B/East"}) {
		t.Fatalf("metadata = %q %v", ex.Version, ex.Names)
	}
	// Open rings: every junction duplicate and the closing vertex removed.
	wantA := scaled([2]float64{0, 10}, [2]float64{0, 0}, [2]float64{5, 0}, [2]float64{5, 5}, [2]float64{5, 10})
	wantB := scaled([2]float64{5, 0}, [2]float64{10, 0}, [2]float64{10, 10}, [2]float64{5, 10}, [2]float64{5, 5})
	gotA := ex.Polygons[0][0].Exterior
	gotB := ex.Polygons[1][0].Exterior
	if !slices.Equal(gotA, wantA) {
		t.Fatalf("ring A = %v, want %v", gotA, wantA)
	}
	if !slices.Equal(gotB, wantB) {
		t.Fatalf("ring B (reversed edge) = %v, want %v", gotB, wantB)
	}
	if ex.Grid == nil {
		t.Fatal("grid map missing")
	}
}

func TestExpandSingleOpRing(t *testing.T) {
	_, r := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	ex, err := r.Expand()
	if err != nil {
		t.Fatal(err)
	}
	want := scaled([2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 10}, [2]float64{0, 10})
	got := ex.Polygons[0][0].Exterior
	if !slices.Equal(got, want) {
		t.Fatalf("single-op ring = %v, want %v (closing duplicate must be dropped)", got, want)
	}
}

func TestExpandHoles(t *testing.T) {
	ext := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}
	hole := [][2]float64{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}}
	input := &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: "holes",
		Timezones: []*pb.CompressedTopoTimezone{{
			Name: "Hole/Test", Polygons: []*pb.CompressedTopoPolygon{polygon(ext, hole)},
		}},
	}
	_, r := openFixture(t, input, EncodeOptions{})
	ex, err := r.Expand()
	if err != nil {
		t.Fatal(err)
	}
	p := ex.Polygons[0][0]
	if len(p.Holes) != 1 {
		t.Fatalf("hole count = %d", len(p.Holes))
	}
	want := scaled([2]float64{3, 3}, [2]float64{7, 3}, [2]float64{7, 7}, [2]float64{3, 7})
	if !slices.Equal(p.Holes[0], want) {
		t.Fatalf("hole ring = %v, want %v", p.Holes[0], want)
	}
}

func TestExpandGridMatchesLookup(t *testing.T) {
	_, r := openFixture(t, sharedEdgeFixture(), EncodeOptions{})
	ex, err := r.Expand()
	if err != nil {
		t.Fatal(err)
	}
	// Grid candidate lists carry the same indices the reader's GRID resolves.
	for key, indices := range ex.Grid {
		if len(indices) == 0 {
			t.Fatalf("empty candidate list stored for %v", key)
		}
		for _, idx := range indices {
			if idx < 0 || int(idx) >= r.TimezoneCount() {
				t.Fatalf("candidate %d out of range at %v", idx, key)
			}
		}
	}
	if _, ok := ex.Grid[[2]int16{2, 5}]; !ok {
		t.Fatal("expected cell (2,5) in grid map")
	}
}
