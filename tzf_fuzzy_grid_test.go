package tzf

import (
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"

	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// probeEveryZoom is the reference lookup: a map probe per zoom from aggZoom
// to idxZoom, coarsest first — the pre-grid implementation.
type probeEveryZoom struct {
	idxZoom, aggZoom int
	single           map[geom.TileID]uint16
	multi            map[geom.TileID][]uint16
	names            []string
}

func newProbeEveryZoom(t *testing.T, reader *embedbin.Reader, names []string) *probeEveryZoom {
	t.Helper()
	single, multi, err := reader.FuzzyMaps()
	if err != nil {
		t.Fatal(err)
	}
	idx, agg := reader.FuzzyZooms()
	return &probeEveryZoom{idx, agg, single, multi, names}
}

func (p *probeEveryZoom) get(lng, lat float64) string {
	if !(lng >= -180 && lng <= 180 && lat >= -90 && lat <= 90) {
		return ""
	}
	tile := geom.NewTileID(lng, lat, uint(p.idxZoom))
	for z := p.aggZoom; z <= p.idxZoom; z++ {
		t := tile.Shift(uint8(p.idxZoom - z))
		if idx, ok := p.single[t]; ok {
			return p.names[idx]
		}
		if idxs, ok := p.multi[t]; ok {
			return p.names[idxs[0]]
		}
	}
	return ""
}

func gridPoints(leafZoom uint8) [][2]float64 {
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
	// tile corners and centres at every zoom the grid stores, where a block
	// or root boundary sits
	for z := uint8(0); z <= leafZoom; z++ {
		n := float64(uint32(1) << z)
		for range 2_000 {
			tx, ty := math.Floor(rng.Float64()*n), math.Floor(rng.Float64()*n)
			for _, f := range []float64{0, 0.5, 1} {
				lng := (tx+f)/n*360 - 180
				lat := math.Atan(math.Sinh(math.Pi*(1-2*(ty+f)/n))) * 180 / math.Pi
				pts = append(pts, [2]float64{lng, lat})
			}
		}
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
	return append(pts, [2]float64{math.NaN(), 0}, [2]float64{0, math.NaN()},
		[2]float64{180.000001, 0}, [2]float64{0, -90.000001})
}

func checkGrid(t *testing.T, data []byte) {
	t.Helper()
	reader, err := embedbin.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFuzzyIndexFromReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	ref := newProbeEveryZoom(t, reader, f.names)
	hits := 0
	for _, p := range gridPoints(f.leafZoom) {
		got, want := f.getTimezoneName(p[0], p[1]), ref.get(p[0], p[1])
		if got != want {
			t.Fatalf("at (%v, %v): grid %q, per-zoom probe %q", p[0], p[1], got, want)
		}
		if got != "" {
			hits++
		}
	}
	if hits == 0 {
		t.Fatal("the sample never reached the fuzzy index")
	}
}

func TestFuzzyGridMatchesProbingEveryZoom(t *testing.T) {
	checkGrid(t, tzfdist.LiteTZB)
}

func TestFuzzyGridMatchesProbingEveryZoomFull(t *testing.T) {
	checkGrid(t, tzfdist.FullTZB)
}

// TestFuzzyGridMatchesProbingEveryZoomExtra checks extra datasets, e.g.
// preindex variants kept to finer zooms: colon-separated .tzb paths in
// TZF_FUZZY_GRID_EXTRA.
func TestFuzzyGridMatchesProbingEveryZoomExtra(t *testing.T) {
	for _, path := range strings.Split(os.Getenv("TZF_FUZZY_GRID_EXTRA"), ":") {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(path, func(t *testing.T) { checkGrid(t, data) })
	}
}

// TestFuzzyEntriesMatchMaps pins what the grid and the export rely on:
// FuzzyEntries yields every tile of both maps, with the same indices, in
// strictly ascending key order.
func TestFuzzyEntriesMatchMaps(t *testing.T) {
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
		err = reader.FuzzyEntries(func(k geom.TileID, indices []uint16) {
			if n > 0 && k <= prev {
				t.Fatalf("%s: key %d not above previous %d", name, k, prev)
			}
			want, ok := multi[k]
			if idx, inSingle := single[k]; inSingle {
				want, ok = []uint16{idx}, true
			}
			if !ok || !slices.Equal(indices, want) {
				t.Fatalf("%s: key %d: entries %v, maps %v", name, k, indices, want)
			}
			prev = k
			n++
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != len(single)+len(multi) {
			t.Fatalf("%s: %d entries, maps hold %d", name, n, len(single)+len(multi))
		}
	}
}
