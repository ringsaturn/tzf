// Package geom provides zero-dependency 2-D geometry primitives and a
// YStripes-indexed point-in-polygon query optimised for timezone lookup.
//
// The YStripes index divides each polygon ring's Y-axis span into horizontal
// stripes. A PIP query needs only to scan the segments stored in the single
// stripe that contains the query latitude, reducing work from O(n) to roughly
// O(n/stripes) in the common case.
//
// The ray-casting algorithm is a direct Go port of github.com/tidwall/geojson
// (MIT), which is itself the canonical implementation used throughout this
// project.
//
// =============================================================================
//
// The MIT License (MIT)
//
// # Copyright (c) 2018 Josh Baker
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
package geom

// Point is a 2-D coordinate. X represents longitude; Y represents latitude.
type Point struct {
	X, Y float64
}

// Rect is an axis-aligned bounding box.
type Rect struct {
	Min, Max Point
}

func (r Rect) containsPoint(p Point) bool {
	return p.X >= r.Min.X && p.X <= r.Max.X &&
		p.Y >= r.Min.Y && p.Y <= r.Max.Y
}
