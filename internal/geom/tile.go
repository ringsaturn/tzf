package geom

import "math"

// tileXY returns the slippy-map tile column (x) and row (y) for (lng, lat)
// at the given zoom level, using the Web Mercator projection (OSM convention).
func tileXY(lng, lat float64, zoom uint) (x, y uint32) {
	n := float64(uint32(1) << zoom)

	x = uint32((lng/360.0 + 0.5) * n)

	switch {
	case lat > 85.0511:
		y = 0
	case lat < -85.0511:
		y = uint32(n) - 1
	default:
		siny := math.Sin(lat * math.Pi / 180.0)
		y = uint32((0.5 - math.Log((1+siny)/(1-siny))/(4*math.Pi)) * n)
	}
	return
}

// TileID packs (x, y, z) into a single TileID.
// Layout: bits 56-63 = zoom (0-255), bits 28-55 = x (up to 2^28), bits 0-27 = y (up to 2^28).
// This covers all OSM zoom levels (0-28) without collision.
type TileID uint64

func NewTileID(lng float64, lat float64, zoom uint) TileID {
	x, y := tileXY(lng, lat, zoom)
	return NewTileIDFromXYZ(x, y, uint8(zoom))
}

func NewTileIDFromXYZ(x, y uint32, z uint8) TileID {
	return TileID(uint64(z)<<56 | uint64(x)<<28 | uint64(y))
}
func (t TileID) XYZ() (x, y uint32, z uint8) {
	return uint32(t>>28) & 0x0FFFFFFF, uint32(t) & 0x0FFFFFFF, uint8(t >> 56)
}

func (t TileID) Polygon() [][2]float64 {
	x, y, z := t.XYZ()

	n := float64(uint32(1) << z)

	lngMin := float64(x)/n*360.0 - 180.0
	lngMax := float64(x+1)/n*360.0 - 180.0

	latMax := math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180.0 / math.Pi
	latMin := math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180.0 / math.Pi

	return [][2]float64{
		{lngMin, latMin},
		{lngMax, latMin},
		{lngMax, latMax},
		{lngMin, latMax},
		{lngMin, latMin},
	}
}

func (t TileID) Shift(shift uint8) TileID {
	x, y, z := t.XYZ()
	if shift > z {
		return 0
	}
	// To derive the tile at any coarser zoom z without repeating the transcendental
	// math, right-shift the high-zoom result:
	//
	//	px, py := tileXY(lng, lat, maxZoom)
	//	x = px >> (maxZoom - z)
	//	y = py >> (maxZoom - z)
	return NewTileIDFromXYZ(x>>shift, y>>shift, z-shift)
}
