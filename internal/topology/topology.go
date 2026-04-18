package topology

import (
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"strconv"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/simplify"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/tidwall/rtree"
)

const snapTolerance = 1e-6

// Minimum number of points in an open path before attempting Douglas-Peucker.
// Paths this short have at most one interior vertex, so simplification cannot
// meaningfully reduce them and risks destabilising degenerate rings.
// Kept deliberately low so that correctly-merged shared chains get simplified.
const minSimplifyPoints = 4

type ringRef struct {
	TimezoneIdx int
	PolygonIdx  int
	HoleIdx     int
}

type edgeUse struct {
	Ring     ringRef
	EdgeIdx  int
	From     pointKey
	To       pointKey
	Reversed bool
}

type edgeMeta struct {
	Shared      bool
	PartnerRing ringRef
}

type pointKey struct {
	Lng float32
	Lat float32
}

type edgeKey struct {
	A pointKey
	B pointKey
}

type ringData struct {
	Points      []*pb.Point
	OriginalLen int
	Edges       []edgeMeta
	Fixed       map[int]struct{}
}

type vertexRef struct {
	Ring  ringRef
	Point *pb.Point
}

type insertCandidate struct {
	Point *pb.Point
	T     float64
}

type sharedSegmentKey struct {
	Signature string
}

type Stats struct {
	InputRings              int
	InputPoints             int
	SnappedRings            int
	SnappedInsertedVertices int
	FixedVertices           int
	RingsNoFixed            int
	RingsOneFixed           int
	RingsMultiFixed         int
	RingsFallbackOriginal   int
	RingsFallbackPoints     int
	Segments                int
	SharedSegments          int
	SharedCacheHits         int
	SharedCacheMisses       int
	SegmentsSkippedShort    int
	SegmentInputPoints      int
	SegmentOutputPoints     int
	SegmentPointsLE10       int
	SegmentPointsLE25       int
	SegmentPointsLE50       int
	SegmentPointsLE100      int
	SegmentPointsGT100      int
}

func (s Stats) String() string {
	skippedPct := percent(s.SegmentsSkippedShort, s.Segments)
	sharedPct := percent(s.SharedSegments, s.Segments)
	cacheHitPct := percent(s.SharedCacheHits, s.SharedCacheHits+s.SharedCacheMisses)
	segmentReduction := percent(s.SegmentInputPoints-s.SegmentOutputPoints, s.SegmentInputPoints)
	return fmt.Sprintf(
		"topology_rings: total=%d no_fixed=%d one_fixed=%d multi_fixed=%d fallback=%d\n"+
			"topology_points: input=%d snapped_inserted=%d fallback_points=%d fixed_vertices=%d\n"+
			"topology_segments: total=%d shared=%d(%.2f%%) skipped_short=%d(%.2f%%) cache_hits=%d cache_misses=%d cache_hit_rate=%.2f%%\n"+
			"topology_segment_points: input=%d output=%d reduction=%.2f%%\n"+
			"topology_segment_length_buckets: le10=%d le25=%d le50=%d le100=%d gt100=%d",
		s.InputRings,
		s.RingsNoFixed,
		s.RingsOneFixed,
		s.RingsMultiFixed,
		s.RingsFallbackOriginal,
		s.InputPoints,
		s.SnappedInsertedVertices,
		s.RingsFallbackPoints,
		s.FixedVertices,
		s.Segments,
		s.SharedSegments,
		sharedPct,
		s.SegmentsSkippedShort,
		skippedPct,
		s.SharedCacheHits,
		s.SharedCacheMisses,
		cacheHitPct,
		s.SegmentInputPoints,
		s.SegmentOutputPoints,
		segmentReduction,
		s.SegmentPointsLE10,
		s.SegmentPointsLE25,
		s.SegmentPointsLE50,
		s.SegmentPointsLE100,
		s.SegmentPointsGT100,
	)
}

func Do(input *pb.Timezones, epsilon float64) *pb.Timezones {
	output, _ := DoWithStats(input, epsilon)
	return output
}

func DoWithStats(input *pb.Timezones, epsilon float64) (*pb.Timezones, Stats) {
	stats := Stats{}
	if input == nil {
		return nil, stats
	}

	output := normalizeTimezones(input)
	// Normalize winding order before topology analysis so that adjacent rings
	// always traverse their shared boundary in opposite directions. Without this,
	// hole rings stored with incorrect CCW winding (instead of CW) appear to share
	// edges in the same direction as the adjacent exterior ring, causing them to be
	// misclassified as disputed-territory overlaps and skipped.
	normalizeWindings(output)
	snapVertices(output, &stats)
	rings, edgeIndex, vertexIndex := collectRings(output)
	stats.InputRings = len(rings)
	for _, ring := range rings {
		stats.InputPoints += ring.OriginalLen
	}
	markSharedEdges(rings, edgeIndex)
	markFixedVertices(rings, vertexIndex, &stats)
	sharedCache := make(map[sharedSegmentKey][]*pb.Point)

	for ref, ring := range rings {
		simplified := simplifyRing(ring, epsilon, sharedCache, &stats)
		result := cleanRing(simplified)
		if len(ringUniquePoints(result)) < 3 {
			// Topology simplification produced a degenerate ring; keep the
			// original unmodified input geometry instead of a corrupted result.
			stats.RingsFallbackOriginal++
			stats.RingsFallbackPoints += ring.OriginalLen
			result = cloneRing(getOriginalRing(input, ref))
		}
		assignRing(output, ref, result)
	}
	normalizeWindings(output)

	return output, stats
}

func normalizeWindings(input *pb.Timezones) {
	for _, timezone := range input.Timezones {
		for _, polygon := range timezone.Polygons {
			polygon.Points = normalizeRingWinding(polygon.Points, false)
			for _, hole := range polygon.Holes {
				hole.Points = normalizeRingWinding(hole.Points, true)
			}
		}
	}
}

func normalizeRingWinding(points []*pb.Point, isHole bool) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	unique := ringUniquePoints(points)
	if len(unique) < 3 {
		return cloneRing(points)
	}
	area := signedArea(unique)
	if isHole && area < 0 {
		return cloneRing(points)
	}
	if !isHole && area > 0 {
		return cloneRing(points)
	}
	reversed := reverseOpenPath(unique)
	return closeRing(reversed)
}

func cleanRing(points []*pb.Point) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	cleaned := make([]*pb.Point, 0, len(points))
	for _, point := range points {
		if len(cleaned) > 0 && samePoint(cleaned[len(cleaned)-1], point) {
			continue
		}
		cleaned = append(cleaned, clonePoint(point))
	}
	if len(cleaned) > 1 && samePoint(cleaned[0], cleaned[len(cleaned)-1]) {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return closeRing(cleaned)
}

func snapVertices(input *pb.Timezones, stats *Stats) {
	// A single snapping pass is intentional. The source data is expected to be
	// nearly aligned already, so inserting first-order T-junction vertices is
	// enough for the shared-edge reconstruction that follows. Recursive snapping
	// would add noticeable cost and is only needed if newly inserted vertices
	// must trigger more insertions on other rings.
	ringPoints := map[ringRef][]*pb.Point{}
	ringVertices := map[ringRef]map[pointKey]struct{}{}
	allVertices := make([]vertexRef, 0)

	for timezoneIdx, timezone := range input.Timezones {
		for polygonIdx, polygon := range timezone.Polygons {
			ref := ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: -1}
			points := ringUniquePoints(polygon.Points)
			ringPoints[ref] = points
			ringVertices[ref] = buildVertexSet(points)
			for _, point := range points {
				allVertices = append(allVertices, vertexRef{Ring: ref, Point: point})
			}

			for holeIdx, hole := range polygon.Holes {
				holeRef := ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: holeIdx}
				holePoints := ringUniquePoints(hole.Points)
				ringPoints[holeRef] = holePoints
				ringVertices[holeRef] = buildVertexSet(holePoints)
				for _, point := range holePoints {
					allVertices = append(allVertices, vertexRef{Ring: holeRef, Point: point})
				}
			}
		}
	}

	var vertexTree rtree.RTreeG[vertexRef]
	for _, vertex := range allVertices {
		box := pointBounds(vertex.Point)
		vertexTree.Insert(box[0], box[1], vertex)
	}

	for ref, points := range ringPoints {
		if len(points) < 2 {
			continue
		}

		insertions := make(map[int][]insertCandidate)
		for edgeIdx := range points {
			from := points[edgeIdx]
			to := points[(edgeIdx+1)%len(points)]
			bounds := segmentBounds(from, to)
			vertexTree.Search(bounds[0], bounds[1], func(_ [2]float64, _ [2]float64, vertex vertexRef) bool {
				if vertex.Ring == ref {
					return true
				}
				key := newPointKey(vertex.Point)
				if _, ok := ringVertices[ref][key]; ok {
					return true
				}
				ok, t := pointOnSegment(vertex.Point, from, to)
				if !ok || t <= 0 || t >= 1 {
					return true
				}
				insertions[edgeIdx] = append(insertions[edgeIdx], insertCandidate{
					Point: vertex.Point,
					T:     t,
				})
				return true
			})
		}

		if len(insertions) == 0 {
			continue
		}
		if stats != nil {
			stats.SnappedRings++
		}

		rebuilt := make([]*pb.Point, 0, len(points)+len(insertions))
		seen := buildVertexSet(points)
		for edgeIdx := range points {
			rebuilt = append(rebuilt, clonePoint(points[edgeIdx]))
			candidates := dedupeInsertCandidates(insertions[edgeIdx], seen)
			slices.SortFunc(candidates, func(a, b insertCandidate) int {
				if a.T < b.T {
					return -1
				}
				if a.T > b.T {
					return 1
				}
				return 0
			})
			for _, candidate := range candidates {
				seen[newPointKey(candidate.Point)] = struct{}{}
				rebuilt = append(rebuilt, clonePoint(candidate.Point))
				if stats != nil {
					stats.SnappedInsertedVertices++
				}
			}
		}
		assignRing(input, ref, closeRing(rebuilt))
	}
}

func normalizeTimezones(input *pb.Timezones) *pb.Timezones {
	output := &pb.Timezones{
		Version: input.Version,
	}
	for _, timezone := range input.Timezones {
		newTimezone := &pb.Timezone{
			Name: timezone.Name,
		}
		for _, polygon := range timezone.Polygons {
			newPolygon := &pb.Polygon{
				Points: normalizeRing(polygon.Points),
				Holes:  make([]*pb.Polygon, 0, len(polygon.Holes)),
			}
			for _, hole := range polygon.Holes {
				newPolygon.Holes = append(newPolygon.Holes, &pb.Polygon{
					Points: normalizeRing(hole.Points),
				})
			}
			newTimezone.Polygons = append(newTimezone.Polygons, newPolygon)
		}
		output.Timezones = append(output.Timezones, newTimezone)
	}
	return output
}

func normalizeRing(points []*pb.Point) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	normalized := make([]*pb.Point, 0, len(points)+1)
	for _, point := range points {
		normalized = append(normalized, normalizePoint(point))
	}
	if !samePoint(normalized[0], normalized[len(normalized)-1]) {
		normalized = append(normalized, clonePoint(normalized[0]))
	}
	return normalized
}

func normalizePoint(point *pb.Point) *pb.Point {
	if point == nil {
		return nil
	}
	// Do NOT normalize -180 to +180 here. Keeping the original coordinate sign
	// is necessary for correct geometry: a polygon whose western boundary sits
	// at exactly -180° would otherwise have a mix of +180 and -179.xx points,
	// making it look like it spans the entire globe to area and simplification
	// routines. The -180/+180 unification is handled exclusively in newPointKey
	// and newEdgeKey for topology matching purposes.
	return &pb.Point{
		Lng: float32(point.Lng),
		Lat: float32(point.Lat),
	}
}

func normalizeLng(v float32) float32 {
	if almostEqual(v, -180) {
		return 180
	}
	return v
}

func collectRings(
	input *pb.Timezones,
) (map[ringRef]*ringData, map[edgeKey][]edgeUse, map[pointKey]map[ringRef]struct{}) {
	rings := make(map[ringRef]*ringData)
	edgeIndex := make(map[edgeKey][]edgeUse)
	vertexIndex := make(map[pointKey]map[ringRef]struct{})

	for timezoneIdx, timezone := range input.Timezones {
		for polygonIdx, polygon := range timezone.Polygons {
			addRing(rings, edgeIndex, vertexIndex, ringRef{
				TimezoneIdx: timezoneIdx,
				PolygonIdx:  polygonIdx,
				HoleIdx:     -1,
			}, polygon.Points)
			for holeIdx, hole := range polygon.Holes {
				addRing(rings, edgeIndex, vertexIndex, ringRef{
					TimezoneIdx: timezoneIdx,
					PolygonIdx:  polygonIdx,
					HoleIdx:     holeIdx,
				}, hole.Points)
			}
		}
	}

	return rings, edgeIndex, vertexIndex
}

func addRing(
	rings map[ringRef]*ringData,
	edgeIndex map[edgeKey][]edgeUse,
	vertexIndex map[pointKey]map[ringRef]struct{},
	ref ringRef,
	points []*pb.Point,
) {
	if len(points) == 0 {
		rings[ref] = &ringData{}
		return
	}

	unique := ringUniquePoints(points)
	rings[ref] = &ringData{
		Points:      points,
		OriginalLen: len(unique),
		Edges:       make([]edgeMeta, len(unique)),
		Fixed:       make(map[int]struct{}),
	}
	for idx, point := range unique {
		key := newPointKey(point)
		if vertexIndex[key] == nil {
			vertexIndex[key] = make(map[ringRef]struct{})
		}
		vertexIndex[key][ref] = struct{}{}

		next := unique[(idx+1)%len(unique)]
		from := newPointKey(point)
		to := newPointKey(next)
		keyEdge := newEdgeKey(from, to)
		edgeIndex[keyEdge] = append(edgeIndex[keyEdge], edgeUse{
			Ring:     ref,
			EdgeIdx:  idx,
			From:     from,
			To:       to,
			Reversed: edgeCanonicalOrder(from, to),
		})
	}
}

func markSharedEdges(rings map[ringRef]*ringData, edgeIndex map[edgeKey][]edgeUse) {
	for _, uses := range edgeIndex {
		if len(uses) != 2 {
			continue
		}
		if uses[0].Ring == uses[1].Ring {
			continue
		}
		if uses[0].From == uses[1].From && uses[0].To == uses[1].To {
			continue
		}
		rings[uses[0].Ring].Edges[uses[0].EdgeIdx] = edgeMeta{
			Shared:      true,
			PartnerRing: uses[1].Ring,
		}
		rings[uses[1].Ring].Edges[uses[1].EdgeIdx] = edgeMeta{
			Shared:      true,
			PartnerRing: uses[0].Ring,
		}
	}
}

// markFixedVertices marks ring vertices that must not be moved during
// Douglas-Peucker simplification. Only true multi-way topological nodes
// (3+ distinct rings meeting at a point) are fixed. Two-ring transition
// points (shared↔non-shared boundary, or partner-change) are intentionally
// left unfixed so that D-P can see longer continuous segments.
func markFixedVertices(rings map[ringRef]*ringData, vertexIndex map[pointKey]map[ringRef]struct{}, stats *Stats) {
	for ref, ring := range rings {
		unique := ringUniquePoints(ring.Points)
		if len(unique) == 0 {
			continue
		}
		for idx, point := range unique {
			vertexKey := newPointKey(point)
			if len(vertexIndex[vertexKey]) > 2 {
				ring.Fixed[idx] = struct{}{}
			}
		}
		if stats != nil {
			stats.FixedVertices += len(ring.Fixed)
		}
		rings[ref] = ring
	}
}

// markFixedVerticesForDedup marks ring vertices that serve as segment
// boundaries for the shared-edge deduplication pass. Unlike the simplification
// variant, this also fixes two-ring transition points so that rings are
// correctly split into purely-shared vs purely-non-shared segments.
func markFixedVerticesForDedup(rings map[ringRef]*ringData, vertexIndex map[pointKey]map[ringRef]struct{}) {
	for ref, ring := range rings {
		unique := ringUniquePoints(ring.Points)
		if len(unique) == 0 {
			continue
		}
		for idx, point := range unique {
			prev := ring.Edges[(idx-1+len(ring.Edges))%len(ring.Edges)]
			next := ring.Edges[idx]
			vertexKey := newPointKey(point)

			// Transition between a shared edge and a non-shared edge.
			if prev.Shared != next.Shared {
				ring.Fixed[idx] = struct{}{}
				continue
			}
			// Both adjacent edges are shared but with different partners.
			if prev.Shared && next.Shared && prev.PartnerRing != next.PartnerRing {
				ring.Fixed[idx] = struct{}{}
				continue
			}
			// Three or more rings meet at this vertex.
			if len(vertexIndex[vertexKey]) > 2 {
				ring.Fixed[idx] = struct{}{}
			}
		}
		rings[ref] = ring
	}
}

func getOriginalRing(input *pb.Timezones, ref ringRef) []*pb.Point {
	tz := input.Timezones[ref.TimezoneIdx]
	poly := tz.Polygons[ref.PolygonIdx]
	if ref.HoleIdx == -1 {
		return poly.Points
	}
	return poly.Holes[ref.HoleIdx].Points
}

// isEntirelyShared returns true when every edge in the ring is shared with the
// same single partner ring. This identifies complete enclaves: a hole ring
// whose shape exactly matches another timezone's exterior polygon.
func isEntirelyShared(edges []edgeMeta) bool {
	if len(edges) == 0 {
		return false
	}
	if !edges[0].Shared {
		return false
	}
	partner := edges[0].PartnerRing
	for _, e := range edges[1:] {
		if !e.Shared || e.PartnerRing != partner {
			return false
		}
	}
	return true
}

// findCanonicalStart returns the index of the lexicographically smallest point
// (by Lng then Lat). Using this as the rotation origin ensures that two partner
// rings sharing all their edges — traversing in opposite directions — both
// independently rotate to the same start vertex, making their open-path
// signatures consistent for the shared segment cache.
func findCanonicalStart(points []*pb.Point) int {
	best := 0
	for i, p := range points {
		b := points[best]
		if p.Lng < b.Lng || (p.Lng == b.Lng && p.Lat < b.Lat) {
			best = i
		}
	}
	return best
}

func simplifyRing(ring *ringData, epsilon float64, sharedCache map[sharedSegmentKey][]*pb.Point, stats *Stats) []*pb.Point {
	points := ring.Points
	fixed := ring.Fixed
	if len(points) == 0 {
		return nil
	}
	unique := ringUniquePoints(points)
	if len(unique) <= 3 {
		return cloneRing(points)
	}

	fixedIndices := make([]int, 0, len(fixed))
	for idx := range fixed {
		if idx >= 0 && idx < len(unique) {
			fixedIndices = append(fixedIndices, idx)
		}
	}
	slices.Sort(fixedIndices)
	if stats != nil {
		switch len(fixedIndices) {
		case 0:
			stats.RingsNoFixed++
		case 1:
			stats.RingsOneFixed++
		default:
			stats.RingsMultiFixed++
		}
	}

	var simplified []*pb.Point
	switch len(fixedIndices) {
	case 0:
		if isEntirelyShared(ring.Edges) {
			// The entire ring boundary is shared with one partner ring (classic
			// enclave: a hole in an outer timezone whose shape matches the inner
			// timezone's exterior). Rotate to the lexicographically smallest
			// vertex so both this ring and its partner independently arrive at
			// the same canonical open-path representation, enabling the shared
			// segment cache to produce identical simplification results.
			canonStart := findCanonicalStart(unique)
			rotated := rotatePoints(unique, canonStart)
			rotatedEdges := rotateEdges(ring.Edges, canonStart)
			openPath := make([]*pb.Point, 0, len(rotated)+1)
			openPath = append(openPath, clonePoints(rotated)...)
			openPath = append(openPath, clonePoint(rotated[0]))
			simplified = closeRing(simplifySegment(openPath, rotatedEdges, epsilon, sharedCache, stats))
		} else {
			simplified = simplifyClosedRing(unique, epsilon, stats)
		}
	case 1:
		start := fixedIndices[0]
		rotated := rotatePoints(unique, start)
		rotatedEdges := rotateEdges(ring.Edges, start)
		openPath := make([]*pb.Point, 0, len(rotated)+1)
		openPath = append(openPath, clonePoints(rotated)...)
		openPath = append(openPath, clonePoint(rotated[0]))
		if isEntirelyShared(rotatedEdges) {
			simplified = closeRing(simplifySegment(openPath, rotatedEdges, epsilon, sharedCache, stats))
		} else {
			reduced := simplifyOpenPath(openPath, epsilon)
			recordSegmentStats(stats, len(openPath), len(reduced), false)
			if stats != nil && len(openPath) <= minSimplifyPoints {
				stats.SegmentsSkippedShort++
			}
			simplified = closeRing(reduced)
		}
	default:
		start := fixedIndices[0]
		rotated := rotatePoints(unique, start)
		rotatedEdges := rotateEdges(ring.Edges, start)
		adjusted := make([]int, 0, len(fixedIndices))
		for _, idx := range fixedIndices {
			if idx < start {
				adjusted = append(adjusted, idx+len(unique)-start)
				continue
			}
			adjusted = append(adjusted, idx-start)
		}
		slices.Sort(adjusted)
		simplified = simplifyFixedSegments(rotated, rotatedEdges, adjusted, epsilon, sharedCache, stats)
	}

	return simplified
}

func simplifyFixedSegments(
	points []*pb.Point,
	edges []edgeMeta,
	fixed []int,
	epsilon float64,
	sharedCache map[sharedSegmentKey][]*pb.Point,
	stats *Stats,
) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	assembled := make([]*pb.Point, 0, len(points)+1)
	for idx := range fixed {
		start := fixed[idx]
		end := fixed[(idx+1)%len(fixed)]
		if idx == len(fixed)-1 {
			end += len(points)
		}
		segment := make([]*pb.Point, 0, end-start+1)
		for cursor := start; cursor <= end; cursor++ {
			segment = append(segment, clonePoint(points[cursor%len(points)]))
		}
		segmentEdges := make([]edgeMeta, 0, end-start)
		for cursor := start; cursor < end; cursor++ {
			segmentEdges = append(segmentEdges, edges[cursor%len(edges)])
		}
		reduced := simplifySegment(segment, segmentEdges, epsilon, sharedCache, stats)
		if idx < len(fixed)-1 {
			reduced = reduced[:len(reduced)-1]
		}
		assembled = append(assembled, reduced...)
	}
	return closeRing(assembled)
}

func simplifySegment(
	segment []*pb.Point,
	segmentEdges []edgeMeta,
	epsilon float64,
	sharedCache map[sharedSegmentKey][]*pb.Point,
	stats *Stats,
) []*pb.Point {
	key, reversed, ok := sharedSegmentCacheKey(segment, segmentEdges)
	if !ok {
		reduced := simplifyOpenPath(segment, epsilon)
		recordSegmentStats(stats, len(segment), len(reduced), false)
		if stats != nil && len(segment) <= minSimplifyPoints {
			stats.SegmentsSkippedShort++
		}
		return reduced
	}

	if cached, exists := sharedCache[key]; exists {
		if stats != nil {
			stats.SharedCacheHits++
			recordSegmentStats(stats, len(segment), len(cached), true)
		}
		if reversed {
			return reverseOpenPath(cached)
		}
		return clonePoints(cached)
	}
	if stats != nil {
		stats.SharedCacheMisses++
	}

	reduced := simplifyOpenPath(segment, epsilon)
	recordSegmentStats(stats, len(segment), len(reduced), true)
	if stats != nil && len(segment) <= minSimplifyPoints {
		stats.SegmentsSkippedShort++
	}
	if reversed {
		sharedCache[key] = reverseOpenPath(reduced)
		return reduced
	}
	sharedCache[key] = clonePoints(reduced)
	return reduced
}

func sharedSegmentCacheKey(
	segment []*pb.Point,
	segmentEdges []edgeMeta,
) (sharedSegmentKey, bool, bool) {
	if len(segment) < 2 || len(segmentEdges) == 0 {
		return sharedSegmentKey{}, false, false
	}
	partner := segmentEdges[0].PartnerRing
	for _, edge := range segmentEdges {
		if !edge.Shared || edge.PartnerRing != partner {
			return sharedSegmentKey{}, false, false
		}
	}

	forward := segmentSignature(segment)
	reversedPoints := reverseOpenPath(segment)
	reverse := segmentSignature(reversedPoints)
	if reverse < forward {
		return sharedSegmentKey{Signature: reverse}, true, true
	}
	return sharedSegmentKey{Signature: forward}, false, true
}

func simplifyClosedRing(points []*pb.Point, epsilon float64, stats *Stats) []*pb.Point {
	path := make([]*pb.Point, 0, len(points))
	path = append(path, clonePoints(points)...)
	path = append(path, clonePoint(points[0]))
	reduced := simplifyOpenPath(path, epsilon)
	recordSegmentStats(stats, len(path), len(reduced), false)
	if stats != nil && len(path) <= minSimplifyPoints {
		stats.SegmentsSkippedShort++
	}
	return closeRing(reduced)
}

func simplifyOpenPath(points []*pb.Point, epsilon float64) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	if len(points) <= minSimplifyPoints {
		return clonePoints(points)
	}

	original := make(orb.LineString, 0, len(points))
	for _, point := range points {
		original = append(original, orb.Point{float64(point.Lng), float64(point.Lat)})
	}
	reduced := simplify.DouglasPeucker(epsilon).Simplify(original.Clone()).(orb.LineString)
	if len(reduced) < 2 {
		return clonePoints(points)
	}

	output := make([]*pb.Point, 0, len(reduced))
	for _, point := range reduced {
		output = append(output, &pb.Point{
			Lng: float32(point.Lon()),
			Lat: float32(point.Lat()),
		})
	}
	return output
}

func recordSegmentStats(stats *Stats, inputPoints, outputPoints int, shared bool) {
	if stats == nil {
		return
	}
	stats.Segments++
	stats.SegmentInputPoints += inputPoints
	stats.SegmentOutputPoints += outputPoints
	if shared {
		stats.SharedSegments++
	}
	switch {
	case inputPoints <= 10:
		stats.SegmentPointsLE10++
	case inputPoints <= 25:
		stats.SegmentPointsLE25++
	case inputPoints <= 50:
		stats.SegmentPointsLE50++
	case inputPoints <= 100:
		stats.SegmentPointsLE100++
	default:
		stats.SegmentPointsGT100++
	}
}

func percent(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return 100 * float64(numerator) / float64(denominator)
}

func assignRing(output *pb.Timezones, ref ringRef, points []*pb.Point) {
	if ref.HoleIdx == -1 {
		output.Timezones[ref.TimezoneIdx].Polygons[ref.PolygonIdx].Points = points
		return
	}
	output.Timezones[ref.TimezoneIdx].Polygons[ref.PolygonIdx].Holes[ref.HoleIdx].Points = points
}

func ringUniquePoints(points []*pb.Point) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	if len(points) == 1 {
		return []*pb.Point{clonePoint(points[0])}
	}
	unique := clonePoints(points)
	if samePoint(unique[0], unique[len(unique)-1]) {
		return unique[:len(unique)-1]
	}
	return unique
}

func closeRing(points []*pb.Point) []*pb.Point {
	if len(points) == 0 {
		return nil
	}
	if !samePoint(points[0], points[len(points)-1]) {
		points = append(points, clonePoint(points[0]))
	}
	if len(points) <= 4 {
		return cloneRing(points)
	}
	unique := ringUniquePoints(points)
	if len(unique) < 3 {
		return cloneRing(points)
	}
	return points
}

func rotatePoints(points []*pb.Point, start int) []*pb.Point {
	rotated := make([]*pb.Point, 0, len(points))
	for idx := 0; idx < len(points); idx++ {
		rotated = append(rotated, clonePoint(points[(start+idx)%len(points)]))
	}
	return rotated
}

func rotateEdges(edges []edgeMeta, start int) []edgeMeta {
	rotated := make([]edgeMeta, 0, len(edges))
	for idx := 0; idx < len(edges); idx++ {
		rotated = append(rotated, edges[(start+idx)%len(edges)])
	}
	return rotated
}

func reverseOpenPath(points []*pb.Point) []*pb.Point {
	reversed := make([]*pb.Point, 0, len(points))
	for idx := len(points) - 1; idx >= 0; idx-- {
		reversed = append(reversed, clonePoint(points[idx]))
	}
	return reversed
}

func segmentSignature(points []*pb.Point) string {
	hasher := fnv.New64a()
	buf := make([]byte, 0, len(points)*24)
	for _, point := range points {
		buf = strconv.AppendFloat(buf, float64(point.Lng), 'f', -1, 32)
		buf = append(buf, ',')
		buf = strconv.AppendFloat(buf, float64(point.Lat), 'f', -1, 32)
		buf = append(buf, ';')
	}
	_, _ = hasher.Write(buf)
	return strconv.FormatUint(hasher.Sum64(), 16)
}

func buildVertexSet(points []*pb.Point) map[pointKey]struct{} {
	out := make(map[pointKey]struct{}, len(points))
	for _, point := range points {
		out[newPointKey(point)] = struct{}{}
	}
	return out
}

func dedupeInsertCandidates(
	candidates []insertCandidate,
	existing map[pointKey]struct{},
) []insertCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]insertCandidate, 0, len(candidates))
	local := make(map[pointKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		key := newPointKey(candidate.Point)
		if _, ok := existing[key]; ok {
			continue
		}
		if _, ok := local[key]; ok {
			continue
		}
		local[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func newPointKey(point *pb.Point) pointKey {
	return pointKey{
		Lng: normalizeLng(float32(point.Lng)),
		Lat: float32(point.Lat),
	}
}

func newEdgeKey(a, b pointKey) edgeKey {
	if edgeCanonicalOrder(a, b) {
		return edgeKey{A: b, B: a}
	}
	return edgeKey{A: a, B: b}
}

func edgeCanonicalOrder(a, b pointKey) bool {
	if a.Lng != b.Lng {
		return a.Lng > b.Lng
	}
	return a.Lat > b.Lat
}

func samePoint(a, b *pb.Point) bool {
	if a == nil || b == nil {
		return a == b
	}
	return almostEqual(float32(a.Lng), float32(b.Lng)) && almostEqual(float32(a.Lat), float32(b.Lat))
}

func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}

func pointOnSegment(point, from, to *pb.Point) (bool, float64) {
	px := float64(point.Lng)
	py := float64(point.Lat)
	ax := float64(from.Lng)
	ay := float64(from.Lat)
	bx := float64(to.Lng)
	by := float64(to.Lat)

	dx := bx - ax
	dy := by - ay
	segLen2 := dx*dx + dy*dy
	if segLen2 == 0 {
		return false, 0
	}

	t := ((px-ax)*dx + (py-ay)*dy) / segLen2
	if t < -snapTolerance || t > 1+snapTolerance {
		return false, t
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	projX := ax + t*dx
	projY := ay + t*dy
	dist2 := (projX-px)*(projX-px) + (projY-py)*(projY-py)
	if dist2 > snapTolerance*snapTolerance {
		return false, t
	}
	return true, t
}

func pointBounds(point *pb.Point) [2][2]float64 {
	x := float64(point.Lng)
	y := float64(point.Lat)
	return [2][2]float64{{x - snapTolerance, y - snapTolerance}, {x + snapTolerance, y + snapTolerance}}
}

func segmentBounds(from, to *pb.Point) [2][2]float64 {
	minX := math.Min(float64(from.Lng), float64(to.Lng))
	minY := math.Min(float64(from.Lat), float64(to.Lat))
	maxX := math.Max(float64(from.Lng), float64(to.Lng))
	maxY := math.Max(float64(from.Lat), float64(to.Lat))
	return [2][2]float64{{minX, minY}, {maxX, maxY}}
}

func cloneRing(points []*pb.Point) []*pb.Point {
	return clonePoints(points)
}

func clonePoints(points []*pb.Point) []*pb.Point {
	cloned := make([]*pb.Point, 0, len(points))
	for _, point := range points {
		cloned = append(cloned, clonePoint(point))
	}
	return cloned
}

func clonePoint(point *pb.Point) *pb.Point {
	if point == nil {
		return nil
	}
	return &pb.Point{
		Lng: point.Lng,
		Lat: point.Lat,
	}
}
