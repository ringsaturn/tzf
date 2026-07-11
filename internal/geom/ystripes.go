package geom

import "math"

// yStripesMin is the minimum number of stripes, and also the minimum ring
// segment count below which no index is built (linear scan is fast enough).
const yStripesMin = 32

type yStripe struct {
	start, count uint32
}

// yStripesIndex partitions the segments of one ring into horizontal stripes.
// A PIP lookup for latitude y only needs to examine the segments stored in
// the single stripe that contains y.
//
// Per-segment Y ranges are not stored; candidate filtering recomputes them
// from the ring endpoints, which the raycast fetches anyway.
//
// Build cost: O(n + n*avgStripesPerSeg).
// Query cost: O(n/stripeCount) on average for convex rings; worst-case O(n).
type yStripesIndex struct {
	minY    float64
	height  float64   // maxY - minY
	stripes []yStripe // one entry per stripe, pointing into indexes
	indexes []uint32  // segment indices packed by stripe
}

// calcStripeCount returns the number of stripes to use for ring r.
// More circular rings get more stripes; elongated rings get fewer.
// Based on the isoperimetric quotient (circularity score):
//
//	score = (4π·area) / perimeter²
func calcStripeCount(r Ring) int {
	area, perim := ringAreaAndPerimeter(r)
	score := 0.0
	if perim > 0 {
		score = (area * math.Pi * 4) / (perim * perim)
	}
	n := int(math.Floor(float64(len(r)) * score))
	if n < yStripesMin {
		return yStripesMin
	}
	return n
}

// segStripeRange maps the y-range [segMinY, segMaxY] to the inclusive stripe
// index range [lo, hi] that the segment spans.
func segStripeRange(segMinY, segMaxY, minY, height float64, count int) (lo, hi int) {
	if count <= 1 || height == 0 {
		return 0, 0
	}
	last := count - 1
	lo = clampStripe(int(math.Floor((segMinY-minY)/height*float64(count))), last)
	hi = clampStripe(int(math.Floor((segMaxY-minY)/height*float64(count))), last)
	return
}

func clampStripe(i, last int) int {
	if i < 0 {
		return 0
	}
	if i > last {
		return last
	}
	return i
}

// pointStripe returns the stripe index for a single y coordinate.
func pointStripe(y, minY, height float64, count int) int {
	return clampStripe(int(math.Floor((y-minY)/height*float64(count))), count-1)
}

// buildYStripes constructs a yStripesIndex for ring r.
// Returns nil when r has fewer than 2 points, zero Y span, or the index would
// overflow the uint32 storage.
func buildYStripes(r Ring) *yStripesIndex {
	n := len(r)
	if n < 2 || uint64(n) > math.MaxUint32 {
		return nil
	}

	// Pre-compute per-segment Y bounding boxes (build-time only) and overall
	// Y range.
	yRanges := make([][2]float64, n)
	minY := r[0].Y
	maxY := r[0].Y
	for i := range n {
		j := (i + 1) % n
		ay, by := r[i].Y, r[j].Y
		if ay <= by {
			yRanges[i] = [2]float64{ay, by}
		} else {
			yRanges[i] = [2]float64{by, ay}
		}
		if yRanges[i][0] < minY {
			minY = yRanges[i][0]
		}
		if yRanges[i][1] > maxY {
			maxY = yRanges[i][1]
		}
	}

	height := maxY - minY
	if height == 0 {
		return nil
	}

	stripeCount := calcStripeCount(r)
	stripes := make([]yStripe, stripeCount)

	// Count how many segment references each stripe will hold.
	counts := make([]int, stripeCount)
	for i := range n {
		lo, hi := segStripeRange(yRanges[i][0], yRanges[i][1], minY, height, stripeCount)
		for s := lo; s <= hi; s++ {
			counts[s]++
		}
	}

	// Assign start offsets.
	total := 0
	starts := make([]int, stripeCount) // fill positions
	for s := range stripes {
		starts[s] = total
		stripes[s].start = uint32(total)
		total += counts[s]
	}
	if uint64(total) > math.MaxUint32 {
		return nil
	}

	// Pack segment indices into the flat indexes slice.
	indexes := make([]uint32, total)
	for i := range n {
		lo, hi := segStripeRange(yRanges[i][0], yRanges[i][1], minY, height, stripeCount)
		for s := lo; s <= hi; s++ {
			pos := starts[s] + int(stripes[s].count)
			indexes[pos] = uint32(i)
			stripes[s].count++
		}
	}

	return &yStripesIndex{
		minY:    minY,
		height:  height,
		stripes: stripes,
		indexes: indexes,
	}
}

// forEachCandidate calls fn for each segment index of ring r whose Y range
// includes y. The segment Y range is recomputed from the ring endpoints
// instead of being stored. The iteration stops early when fn returns false.
// No allocation; the caller drives the loop.
func (idx *yStripesIndex) forEachCandidate(r Ring, y float64, fn func(int) bool) {
	if y < idx.minY || y > idx.minY+idx.height {
		return
	}
	n := len(r)
	s := pointStripe(y, idx.minY, idx.height, len(idx.stripes))
	stripe := idx.stripes[s]
	start := int(stripe.start)
	for k := start; k < start+int(stripe.count); k++ {
		seg := int(idx.indexes[k])
		ay, by := r[seg].Y, r[(seg+1)%n].Y
		if by < ay {
			ay, by = by, ay
		}
		if y >= ay && y <= by {
			if !fn(seg) {
				return
			}
		}
	}
}
