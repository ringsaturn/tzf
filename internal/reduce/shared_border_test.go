package reduce

import (
	"testing"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/topology"
)

// TestDoTopologyAware_SharedBorderFormsPartition drives the whole lite
// simplification entry point and checks that a border shared by two rings
// still partitions the plane afterwards. This is the pipeline-level guard for
// the shipped lite coverage gap at 16.06440,-82.33148: the two rings used to
// simplify independently and drift apart, leaving a sliver covered by neither.
func TestDoTopologyAware_SharedBorderFormsPartition(t *testing.T) {
	reverse := func(in [][2]float32) [][2]float32 {
		out := make([][2]float32, 0, len(in))
		for i := len(in) - 1; i >= 0; i-- {
			out = append(out, in[i])
		}
		return out
	}
	cat := func(parts ...[][2]float32) [][2]float32 {
		var out [][2]float32
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	// Wavy border, with the left ring rotated so its start vertex lies inside
	// the shared chain (as real rings do).
	border := [][2]float32{
		{0, 4}, {0.0020, 3.6}, {-0.0015, 3.2}, {0.0025, 2.8}, {-0.0020, 2.4},
		{0.0010, 2.0}, {-0.0025, 1.6}, {0.0015, 1.2}, {0, 1.0},
	}
	leftSide := [][2]float32{
		{-0.5, 0.8}, {-1.2, 0.6}, {-2.0, 1.0}, {-2.6, 2.0},
		{-2.8, 3.0}, {-2.5, 3.8}, {-1.8, 4.4}, {-0.8, 4.5},
	}
	rightSide := [][2]float32{
		{0.8, 4.5}, {1.8, 4.4}, {2.5, 3.8}, {2.8, 3.0},
		{2.6, 2.0}, {2.0, 1.0}, {1.2, 0.6}, {0.5, 0.8},
	}
	left := cat(border, leftSide)
	left = append(left[4:], left[:4]...)

	input := &pb.Timezones{Version: "t", Timezones: []*pb.Timezone{
		{Name: "Left", Polygons: []*pb.Polygon{{Points: lineToRing(left)}}},
		{Name: "Right", Polygons: []*pb.Polygon{{Points: lineToRing(cat(rightSide, reverse(border)))}}},
	}}

	output := DoTopologyAware(input, 0.005)
	if err := topology.Validate(output); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	lr := output.Timezones[0].Polygons[0].Points
	rr := output.Timezones[1].Polygons[0].Points

	const delta = 0.0005
	for i := 0; i+1 < len(border); i++ {
		ax, ay := border[i][0], border[i][1]
		bx, by := border[i+1][0], border[i+1][1]
		dx, dy := bx-ax, by-ay
		l := dx*dx + dy*dy
		if l == 0 {
			continue
		}
		nx, ny := -dy/l, dx/l
		for s := float32(0.05); s < 1; s += 0.05 {
			mx, my := ax+dx*s, ay+dy*s
			for _, side := range []float64{-1, 1} {
				px, py := float64(mx)+side*float64(nx)*delta, float64(my)+side*float64(ny)*delta
				inL := ringContains(px, py, lr)
				inR := ringContains(px, py, rr)
				if !inL && !inR {
					t.Fatalf("coverage gap at (%.5f,%.5f) after lite simplification", px, py)
				}
				if inL && inR {
					t.Fatalf("coverage overlap at (%.5f,%.5f) after lite simplification", px, py)
				}
			}
		}
	}
}

// ringContains is a plain even-odd ray cast for the partition check.
func ringContains(lng, lat float64, ring []*pb.Point) bool {
	inside := false
	n := len(ring)
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := float64(ring[i].Lng), float64(ring[i].Lat)
		xj, yj := float64(ring[j].Lng), float64(ring[j].Lat)
		if (yi > lat) != (yj > lat) && lng < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
