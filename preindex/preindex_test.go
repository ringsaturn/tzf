package preindex

import (
	"testing"

	"github.com/paulmach/orb/maptile"
	"github.com/ringsaturn/tzf/internal/geom"
)

func TestEnsureInsideRejectsCrossBoundaryTile(t *testing.T) {
	tile := maptile.New(2805, 6396, 13)
	b := tile.Bound()
	west, south := b.Min.Lon(), b.Min.Lat()
	east, north := b.Max.Lon(), b.Max.Lat()
	midLng := (west + east) / 2

	partial := rectanglePolygon(west-0.01, south-0.01, midLng, north+0.01)
	if got := EnsureInside([]*geom.Polygon{partial}, []maptile.Tile{tile}); len(got) != 0 {
		t.Fatalf("expected cross-boundary tile to be rejected, got %v", got)
	}
}

func TestEnsureInsideAcceptsTileContainedByAnyPolygon(t *testing.T) {
	tile := maptile.New(2805, 6396, 13)
	b := tile.Bound()
	west, south := b.Min.Lon(), b.Min.Lat()
	east, north := b.Max.Lon(), b.Max.Lat()

	disjoint := rectanglePolygon(east+1, north+1, east+2, north+2)
	containing := rectanglePolygon(west-0.01, south-0.01, east+0.01, north+0.01)
	got := EnsureInside([]*geom.Polygon{disjoint, containing}, []maptile.Tile{tile})
	if len(got) != 1 || got[0] != tile {
		t.Fatalf("expected contained tile to be accepted, got %v", got)
	}
}

func TestEnsureInsideRejectsTileWithoutPolygons(t *testing.T) {
	tile := maptile.New(2805, 6396, 13)
	if got := EnsureInside(nil, []maptile.Tile{tile}); len(got) != 0 {
		t.Fatalf("expected tile without polygons to be rejected, got %v", got)
	}
}

func rectanglePolygon(west, south, east, north float64) *geom.Polygon {
	return geom.NewPolygon([]geom.Point{
		{X: west, Y: south},
		{X: east, Y: south},
		{X: east, Y: north},
		{X: west, Y: north},
		{X: west, Y: south},
	}, nil)
}
