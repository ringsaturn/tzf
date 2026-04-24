package geom

import "math"

// yStripesMin is the minimum number of stripes, and also the minimum ring
// segment count below which no index is built (linear scan is fast enough).
const yStripesMin = 32

type yStripe struct {
	start, count int
}

// yStripesIndex partitions the segments of one ring into horizontal stripes.
// A PIP lookup for latitude y only needs to examine the segments stored in
// the single stripe that contains y.
//
// Build cost: O(n + n*avgStripesPerSeg).
// Query cost: O(n/stripeCount) on average for convex rings; worst-case O(n).
type yStripesIndex struct {
	minY    float64
	height  float64      // maxY - minY
	stripes []yStripe    // one entry per stripe, pointing into indexes
	indexes []int        // segment indices packed by stripe
	yRanges [][2]float64 // per-segment [minY, maxY] bounding box
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
// Returns nil when r has fewer than 2 points or zero Y span.
func buildYStripes(r Ring) *yStripesIndex {
	n := len(r)
	if n < 2 {
		return nil
	}

	// Pre-compute per-segment Y bounding boxes and overall Y range.
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
	for i := range n {
		lo, hi := segStripeRange(yRanges[i][0], yRanges[i][1], minY, height, stripeCount)
		for s := lo; s <= hi; s++ {
			stripes[s].count++
		}
	}

	// Assign start offsets; temporarily reuse count as a fill cursor.
	total := 0
	starts := make([]int, stripeCount) // fill positions
	for s := range stripes {
		starts[s] = total
		stripes[s].start = total
		total += stripes[s].count
		stripes[s].count = 0 // reset; refilled below
	}

	// Pack segment indices into the flat indexes slice.
	indexes := make([]int, total)
	for i := range n {
		lo, hi := segStripeRange(yRanges[i][0], yRanges[i][1], minY, height, stripeCount)
		for s := lo; s <= hi; s++ {
			pos := starts[s] + stripes[s].count
			indexes[pos] = i
			stripes[s].count++
		}
	}

	return &yStripesIndex{
		minY:    minY,
		height:  height,
		stripes: stripes,
		indexes: indexes,
		yRanges: yRanges,
	}
}

// forEachCandidate calls fn for each segment index whose Y range includes y.
// The iteration stops early when fn returns false.
// No allocation; the caller drives the loop.
func (idx *yStripesIndex) forEachCandidate(y float64, fn func(int) bool) {
	if y < idx.minY || y > idx.minY+idx.height {
		return
	}
	s := pointStripe(y, idx.minY, idx.height, len(idx.stripes))
	stripe := idx.stripes[s]
	for k := stripe.start; k < stripe.start+stripe.count; k++ {
		seg := idx.indexes[k]
		if y >= idx.yRanges[seg][0] && y <= idx.yRanges[seg][1] {
			if !fn(seg) {
				return
			}
		}
	}
}
