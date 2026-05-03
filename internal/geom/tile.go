package geom

import "math"

// TileXY returns the slippy-map tile column (x) and row (y) for (lng, lat)
// at the given zoom level, using the Web Mercator projection (OSM convention).
//
// To derive the tile at any coarser zoom z without repeating the transcendental
// math, right-shift the high-zoom result:
//
//	px, py := TileXY(lng, lat, maxZoom)
//	x = px >> (maxZoom - z)
//	y = py >> (maxZoom - z)
func TileXY(lng, lat float64, zoom uint) (x, y uint32) {
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

// TileBound returns the (lngMin, latMin, lngMax, latMax) geographic bounding
// box of the slippy-map tile at (x, y, zoom), using the Web Mercator inverse.
func TileBound(x, y uint32, zoom uint) (lngMin, latMin, lngMax, latMax float64) {
	n := float64(uint32(1) << zoom)

	lngMin = float64(x)/n*360.0 - 180.0
	lngMax = float64(x+1)/n*360.0 - 180.0

	latMax = math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180.0 / math.Pi
	latMin = math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180.0 / math.Pi
	return
}
