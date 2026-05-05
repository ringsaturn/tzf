package geom

import "math"

// raycastResult is the outcome of testing one segment against a horizontal ray.
type raycastResult struct {
	inside bool // ray crosses the segment (point is to the left of crossing)
	on     bool // point lies directly on the segment
}

// raycastSeg tests whether a leftward horizontal ray from p crosses segment
// (a, b).  It is a direct Go port of the ray-casting function in
// github.com/tidwall/geojson (MIT licence).
//
// Edge cases handled:
//   - Horizontal and vertical segments.
//   - Points that land exactly on a vertex: py is nudged via math.Nextafter so
//     the vertex is counted at most once, matching the winding convention used
//     by the original geojson library.
//   - Collinear points are reported as "on" rather than "inside".
func raycastSeg(a, b, p Point) raycastResult {
	py := p.Y

	// Quick Y-range rejection.
	if a.Y < b.Y {
		if py < a.Y || py > b.Y {
			return raycastResult{}
		}
	} else if a.Y > b.Y {
		if py < b.Y || py > a.Y {
			return raycastResult{}
		}
	}

	// Detect if p lies on the segment before the raycast nudge.
	if a.Y == b.Y { // horizontal segment
		if a.X == b.X { // degenerate (single point)
			if p.X == a.X && py == a.Y {
				return raycastResult{on: true}
			}
			return raycastResult{}
		}
		if py == b.Y {
			if a.X < b.X {
				if p.X >= a.X && p.X <= b.X {
					return raycastResult{on: true}
				}
			} else if p.X >= b.X && p.X <= a.X {
				return raycastResult{on: true}
			}
		}
	}
	if a.X == b.X && p.X == b.X { // vertical segment
		if a.Y < b.Y {
			if py >= a.Y && py <= b.Y {
				return raycastResult{on: true}
			}
		} else if py >= b.Y && py <= a.Y {
			return raycastResult{on: true}
		}
	}
	// General collinearity check. Division by zero yields Inf/NaN; NaN != NaN
	// and Inf != finite, so the comparison safely returns false in those cases.
	if (p.X-a.X)/(b.X-a.X) == (py-a.Y)/(b.Y-a.Y) {
		return raycastResult{on: true}
	}

	// Nudge py off any vertex to avoid double-counting shared polygon vertices.
	for py == a.Y || py == b.Y {
		py = math.Nextafter(py, math.Inf(1))
	}

	// Re-check Y bounds after nudge.
	if a.Y < b.Y {
		if py < a.Y || py > b.Y {
			return raycastResult{}
		}
	} else if py < b.Y || py > a.Y {
		return raycastResult{}
	}

	// X-axis shortcuts: if p.X is clearly to the right or left of both
	// endpoints, the crossing result is trivial.
	if a.X > b.X {
		if p.X >= a.X {
			return raycastResult{}
		}
		if p.X <= b.X {
			return raycastResult{inside: true}
		}
	} else {
		if p.X >= b.X {
			return raycastResult{}
		}
		if p.X <= a.X {
			return raycastResult{inside: true}
		}
	}

	// Slope comparison to determine which side of the segment p lies on.
	if a.Y < b.Y {
		if (py-a.Y)/(p.X-a.X) >= (b.Y-a.Y)/(b.X-a.X) {
			return raycastResult{inside: true}
		}
	} else if (py-b.Y)/(p.X-b.X) >= (a.Y-b.Y)/(a.X-b.X) {
		return raycastResult{inside: true}
	}
	return raycastResult{}
}

// ringContainsPoint reports whether p is strictly inside ring r using the
// even-odd ray-casting rule.  Points on the ring boundary return false.
//
// When idx is non-nil, only the candidate segments returned by the YStripes
// index are examined; otherwise all n segments are checked linearly.
func ringContainsPoint(r Ring, idx *yStripesIndex, p Point) bool {
	n := len(r)
	if n < 3 {
		return false
	}

	inside := false

	if idx != nil {
		// Indexed path: iterate only the stripe containing p.Y.
		idx.forEachCandidate(p.Y, func(i int) bool {
			j := (i + 1) % n
			res := raycastSeg(r[i], r[j], p)
			if res.on {
				inside = false
				return false // stop
			}
			if res.inside {
				inside = !inside
			}
			return true
		})
		return inside
	}

	// Linear fallback for small rings.
	for i := range n {
		j := (i + 1) % n
		res := raycastSeg(r[i], r[j], p)
		if res.on {
			return false
		}
		if res.inside {
			inside = !inside
		}
	}
	return inside
}
