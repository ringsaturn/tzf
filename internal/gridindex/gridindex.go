// Package gridindex builds and parses the 1°×1° grid candidate-reduction index.
//
// The grid partitions the globe into 360×180 = 64,800 cells. For each cell
// the index records which timezone indices (positions in the companion
// Timezones / CompressedTopoTimezones file) have a bounding box that overlaps
// that cell. At query time, only those candidates need to be tested with a
// full PIP check, reducing the set from ~444 timezones to typically 1–3.
//
// Inspired by Aaron Roney's rtz <https://github.com/twitchax/rtz>.
package gridindex

import (
	"math"
	"runtime"
	"slices"
	"sync"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/polyline"
)

// Build creates a GridIndex from pre-computed bounding boxes.
// bboxes[i] = {minLng, minLat, maxLng, maxLat} for timezone at index i.
// The resulting tz_indices in each cell are 0-based positions into the same
// timezone slice, matching the item order used by Finder.
func Build(bboxes [][4]float64, version string) *pb.GridIndex {
	type entry struct {
		key [2]int16
		idx uint32
	}

	nWorkers := max(runtime.NumCPU(), 1)
	ch := make(chan []entry, nWorkers)

	var wg sync.WaitGroup
	chunkSize := (len(bboxes) + nWorkers - 1) / nWorkers
	for w := range nWorkers {
		start := w * chunkSize
		if start >= len(bboxes) {
			break
		}
		end := min(start+chunkSize, len(bboxes))
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			var entries []entry
			for i := lo; i < hi; i++ {
				bb := bboxes[i]
				minLng := int(math.Floor(bb[0]))
				minLat := int(math.Floor(bb[1]))
				maxLng := int(math.Floor(bb[2]))
				maxLat := int(math.Floor(bb[3]))
				for lng := minLng; lng <= maxLng; lng++ {
					for lat := minLat; lat <= maxLat; lat++ {
						entries = append(entries, entry{
							key: [2]int16{int16(lng), int16(lat)},
							idx: uint32(i),
						})
					}
				}
			}
			ch <- entries
		}(start, end)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	m := make(map[[2]int16][]uint32)
	for entries := range ch {
		for _, e := range entries {
			m[e.key] = append(m[e.key], e.idx)
		}
	}

	// Sort each cell's indices ascending so that GetTimezoneName iterates
	// candidates in the same order as the original linear scan (lowest index
	// first). This ensures deterministic results for overlapping timezone
	// polygons regardless of goroutine scheduling during build.
	cells := make([]*pb.GridIndexCell, 0, len(m))
	for k, indices := range m {
		slices.Sort(indices)
		cells = append(cells, &pb.GridIndexCell{
			Lng:       int32(k[0]),
			Lat:       int32(k[1]),
			TzIndices: indices,
		})
	}
	return &pb.GridIndex{Cells: cells, Version: version}
}

// BuildFromTimezones constructs a GridIndex from a Timezones protobuf.
// The tz_indices in each cell are 0-based positions into input.Timezones,
// matching the item order produced by tzf.NewFinderFromPB.
func BuildFromTimezones(input *pb.Timezones) *pb.GridIndex {
	bboxes := make([][4]float64, len(input.Timezones))
	for i, tz := range input.Timezones {
		minLng := math.MaxFloat64
		minLat := math.MaxFloat64
		maxLng := -math.MaxFloat64
		maxLat := -math.MaxFloat64
		for _, poly := range tz.Polygons {
			for _, pt := range poly.Points {
				lng := float64(pt.Lng)
				lat := float64(pt.Lat)
				minLng = min(minLng, lng)
				minLat = min(minLat, lat)
				maxLng = max(maxLng, lng)
				maxLat = max(maxLat, lat)
			}
		}
		bboxes[i] = [4]float64{minLng, minLat, maxLng, maxLat}
	}
	return Build(bboxes, input.Version)
}

// BuildFromCompressedTopoTimezones constructs a GridIndex directly from a
// CompressedTopoTimezones by computing timezone bboxes from shared-edge and
// inline polyline bytes, without building a full Finder.
func BuildFromCompressedTopoTimezones(input *pb.CompressedTopoTimezones) *pb.GridIndex {
	// Pre-compute bbox for each shared edge (keyed by edge ID).
	edgeBBox := make(map[int32][4]float64, len(input.SharedEdges))
	for _, e := range input.SharedEdges {
		edgeBBox[e.Id] = bboxFromPolylineBytes(e.Points)
	}

	bboxes := make([][4]float64, len(input.Timezones))
	for i, tz := range input.Timezones {
		b := [4]float64{math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64}
		for _, poly := range tz.Polygons {
			expandFromCompressedPoly(poly, edgeBBox, &b)
		}
		bboxes[i] = b
	}
	return Build(bboxes, input.Version)
}

func bboxFromPolylineBytes(data []byte) [4]float64 {
	b := [4]float64{math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64}
	coords, _, _ := polyline.DecodeCoords(data)
	for _, c := range coords {
		b[0] = min(b[0], c[0])
		b[1] = min(b[1], c[1])
		b[2] = max(b[2], c[0])
		b[3] = max(b[3], c[1])
	}
	return b
}

func expandFromCompressedPoly(poly *pb.CompressedTopoPolygon, edgeBBox map[int32][4]float64, b *[4]float64) {
	for _, seg := range poly.Exterior {
		switch s := seg.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			eb := bboxFromPolylineBytes(s.Inline.Points)
			b[0] = min(b[0], eb[0])
			b[1] = min(b[1], eb[1])
			b[2] = max(b[2], eb[2])
			b[3] = max(b[3], eb[3])
		case *pb.CompressedRingSegment_EdgeForward:
			if eb, ok := edgeBBox[s.EdgeForward]; ok {
				b[0] = min(b[0], eb[0])
				b[1] = min(b[1], eb[1])
				b[2] = max(b[2], eb[2])
				b[3] = max(b[3], eb[3])
			}
		case *pb.CompressedRingSegment_EdgeReversed:
			if eb, ok := edgeBBox[s.EdgeReversed]; ok {
				b[0] = min(b[0], eb[0])
				b[1] = min(b[1], eb[1])
				b[2] = max(b[2], eb[2])
				b[3] = max(b[3], eb[3])
			}
		}
	}
	for _, hole := range poly.Holes {
		expandFromCompressedPoly(hole, edgeBBox, b)
	}
}

// DecodeToMap converts a GridIndex protobuf into the runtime map used by Finder.
// Keys are (floor(lng), floor(lat)); values are slices of item indices (int32).
func DecodeToMap(gi *pb.GridIndex) map[[2]int16][]int32 {
	m := make(map[[2]int16][]int32, len(gi.Cells))
	for _, cell := range gi.Cells {
		key := [2]int16{int16(cell.Lng), int16(cell.Lat)}
		indices := make([]int32, len(cell.TzIndices))
		for i, v := range cell.TzIndices {
			indices[i] = int32(v)
		}
		m[key] = indices
	}
	return m
}
