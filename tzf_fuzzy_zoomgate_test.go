package tzf

import (
	"math"
	"math/rand/v2"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"

	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// getAllZooms is the lookup before the zoom gate: every zoom from aggZoom to
// idxZoom, with no 1° cell table in front.
func (f *fuzzyIndex) getAllZooms(lng, lat float64) string {
	tile := geom.NewTileID(lng, lat, uint(f.idxZoom))
	for z := f.aggZoom; z <= f.idxZoom; z++ {
		t := tile.Shift(uint8(f.idxZoom - z))
		if idx, ok := f.single[t]; ok {
			return f.names[idx]
		}
		if idxs, ok := f.multi[t]; ok {
			return f.names[idxs[0]]
		}
	}
	return ""
}

func zoomGatePoints() [][2]float64 {
	rng := rand.New(rand.NewPCG(1, 2))
	var pts [][2]float64
	for range 300_000 {
		pts = append(pts, [2]float64{rng.Float64()*360 - 180, rng.Float64()*180 - 90})
	}
	// polar caps, where the Mercator rows are clamped
	for range 50_000 {
		pts = append(pts, [2]float64{rng.Float64()*360 - 180, 80 + rng.Float64()*10})
		pts = append(pts, [2]float64{rng.Float64()*360 - 180, -90 + rng.Float64()*10})
	}
	// points exactly on 1° cell edges and the domain edges
	for lat := -90; lat <= 90; lat += 5 {
		for lng := -180; lng <= 180; lng += 5 {
			pts = append(pts, [2]float64{float64(lng), float64(lat)})
		}
	}
	for _, lat := range []float64{-85.0511, 85.0511, -85.05113, 85.05113} {
		for _, lng := range []float64{-180, math.Copysign(0, -1), 0, 179.999999, 180} {
			pts = append(pts, [2]float64{lng, lat})
		}
	}
	return pts
}

func checkZoomGate(t *testing.T, data []byte) {
	t.Helper()
	reader, err := embedbin.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFuzzyIndexFromReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	for _, p := range zoomGatePoints() {
		got, want := f.getTimezoneName(p[0], p[1]), f.getAllZooms(p[0], p[1])
		if got != want {
			t.Fatalf("at (%v, %v): gated %q, all zooms %q", p[0], p[1], got, want)
		}
		if got != "" {
			hits++
		}
	}
	if hits == 0 {
		t.Fatal("the sample never reached the fuzzy index")
	}
}

func TestZoomGateMatchesProbingEveryZoom(t *testing.T) {
	checkZoomGate(t, tzfdist.LiteTZB)
}

func TestZoomGateMatchesProbingEveryZoomFull(t *testing.T) {
	checkZoomGate(t, tzfdist.FullTZB)
}

// TestFuzzyKeysAscendingAndComplete pins what buildZoomRanges relies on:
// FuzzyKeys yields every tile of both maps, in strictly ascending order.
func TestFuzzyKeysAscendingAndComplete(t *testing.T) {
	for name, data := range map[string][]byte{"lite": tzfdist.LiteTZB, "full": tzfdist.FullTZB} {
		reader, err := embedbin.Open(data)
		if err != nil {
			t.Fatal(err)
		}
		single, multi, err := reader.FuzzyMaps()
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		var prev geom.TileID
		err = reader.FuzzyKeys(func(k geom.TileID) {
			if n > 0 && k <= prev {
				t.Fatalf("%s: key %d not above previous %d", name, k, prev)
			}
			_, inSingle := single[k]
			_, inMulti := multi[k]
			if !inSingle && !inMulti {
				t.Fatalf("%s: key %d in neither map", name, k)
			}
			prev = k
			n++
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != len(single)+len(multi) {
			t.Fatalf("%s: %d keys, maps hold %d", name, n, len(single)+len(multi))
		}
	}
}
