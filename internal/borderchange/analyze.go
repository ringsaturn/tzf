// Package borderchange measures the geometric error introduced by boundary
// simplification.
package borderchange

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

const authalicRadiusM = 6371007.180918475
const (
	maxGeographicChordM = 5000.0
	targetChordSagittaM = 0.4
	maxChordSagittaM    = 0.5
)

type Options struct {
	CertificationToleranceM float64
	// Workers bounds analysis parallelism; zero means GOMAXPROCS.
	Workers int
	// Progress enables periodic stderr logging for long runs.
	Progress bool
}

const distributionResolutionM = 0.1

type Distribution struct {
	bins  map[int64]float64
	total float64
}

type Location struct {
	Lng       float64
	Lat       float64
	DistanceM float64
	TimezoneA string
	TimezoneB string
}

type PairArea struct {
	TimezoneA string
	TimezoneB string
	AreaKM2   float64
}

type Report struct {
	UniqueArcs              int
	ChangedArcs             int
	OriginalLengthKM        float64
	ChangedLengthKM         float64
	ErrorAreaKM2            float64
	MaxStripAreaKM2         float64
	MaxCertifiedM           float64
	CertificationToleranceM float64
	MaxLocation             Location
	LengthDistances         Distribution
	AreaWidths              Distribution
	PairAreas               []PairArea

	pairAreas map[pairKey]float64
}

type pairKey struct {
	a string
	b string
}

type point struct {
	lng float64
	lat float64
}

type arc struct {
	original []point
	zones    []string
}

type pendingArc struct {
	arc      *arc
	zoneA    string
	zoneB    string
	estimate float64
}

// sharedMax is the certification pruning threshold shared across workers. It
// only grows, so any interval pruned against it was pruned against a valid
// lower bound of the final maximum.
type sharedMax struct {
	bits atomic.Uint64
}

func (s *sharedMax) load() float64 {
	return math.Float64frombits(s.bits.Load())
}

func (s *sharedMax) update(v float64) {
	for {
		old := s.bits.Load()
		if v <= math.Float64frombits(old) {
			return
		}
		if s.bits.CompareAndSwap(old, math.Float64bits(v)) {
			return
		}
	}
}

func Analyze(original, simplified *pb.Timezones, opts Options) (*Report, error) {
	if original == nil || simplified == nil {
		return nil, errors.New("original and simplified datasets are required")
	}
	if len(original.Timezones) != len(simplified.Timezones) {
		return nil, fmt.Errorf("timezone count differs: %d vs %d", len(original.Timezones), len(simplified.Timezones))
	}
	if opts.CertificationToleranceM <= 0 {
		opts.CertificationToleranceM = 0.5
	}

	arcs := make(map[[sha256.Size]byte]*arc)
	for tzIdx, originalTZ := range original.Timezones {
		simplifiedTZ := simplified.Timezones[tzIdx]
		if originalTZ.Name != simplifiedTZ.Name {
			return nil, fmt.Errorf("timezone %d differs: %q vs %q", tzIdx, originalTZ.Name, simplifiedTZ.Name)
		}
		if len(originalTZ.Polygons) != len(simplifiedTZ.Polygons) {
			return nil, fmt.Errorf("polygon count differs for %q", originalTZ.Name)
		}
		for polygonIdx, originalPolygon := range originalTZ.Polygons {
			simplifiedPolygon := simplifiedTZ.Polygons[polygonIdx]
			if err := collectRingArcs(arcs, originalTZ.Name, originalPolygon.Points, simplifiedPolygon.Points); err != nil {
				return nil, fmt.Errorf("%s polygon %d exterior: %w", originalTZ.Name, polygonIdx, err)
			}
			if len(originalPolygon.Holes) != len(simplifiedPolygon.Holes) {
				return nil, fmt.Errorf("hole count differs for %q polygon %d", originalTZ.Name, polygonIdx)
			}
			for holeIdx, originalHole := range originalPolygon.Holes {
				if err := collectRingArcs(arcs, originalTZ.Name, originalHole.Points, simplifiedPolygon.Holes[holeIdx].Points); err != nil {
					return nil, fmt.Errorf("%s polygon %d hole %d: %w", originalTZ.Name, polygonIdx, holeIdx, err)
				}
			}
		}
	}

	pending := make([]pendingArc, 0, len(arcs))
	for _, item := range arcs {
		zoneA, zoneB := zonePair(item.zones)
		pending = append(pending, pendingArc{arc: item, zoneA: zoneA, zoneB: zoneB, estimate: arcDeviationEstimate(item.original)})
	}
	// Processing large-deviation arcs first drives the shared pruning threshold
	// close to the final maximum almost immediately, so the Lipschitz pass can
	// prune nearly every interval on the remaining arcs.
	slices.SortFunc(pending, func(a, b pendingArc) int {
		if a.estimate != b.estimate {
			if a.estimate > b.estimate {
				return -1
			}
			return 1
		}
		if c := strings.Compare(a.zoneA, b.zoneA); c != 0 {
			return c
		}
		if c := strings.Compare(a.zoneB, b.zoneB); c != 0 {
			return c
		}
		return len(a.arc.original) - len(b.arc.original)
	})

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(pending) {
		workers = len(pending)
	}
	if workers < 1 {
		workers = 1
	}
	shared := &sharedMax{}
	locals := make([]*Report, workers)
	var next atomic.Int64
	var wg sync.WaitGroup
	if opts.Progress {
		log.Printf("borderchange: %d arcs collected, analyzing with %d workers", len(pending), workers)
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					done := min(int(next.Load()), len(pending))
					log.Printf("borderchange: %d/%d arcs, current max %.3f m", done, len(pending), shared.load())
				}
			}
		}()
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			local := &Report{}
			locals[slot] = local
			for {
				idx := int(next.Add(1)) - 1
				if idx >= len(pending) {
					return
				}
				item := pending[idx]
				analyzeArc(shared, local, item.arc.original, item.zoneA, item.zoneB, opts.CertificationToleranceM)
			}
		}(w)
	}
	wg.Wait()

	report := &Report{CertificationToleranceM: opts.CertificationToleranceM + maxChordSagittaM}
	for _, local := range locals {
		report.merge(local)
	}
	report.PairAreas = make([]PairArea, 0, len(report.pairAreas))
	for key, area := range report.pairAreas {
		report.PairAreas = append(report.PairAreas, PairArea{TimezoneA: key.a, TimezoneB: key.b, AreaKM2: area})
	}
	slices.SortFunc(report.PairAreas, func(a, b PairArea) int {
		if a.AreaKM2 > b.AreaKM2 {
			return -1
		}
		if a.AreaKM2 < b.AreaKM2 {
			return 1
		}
		return strings.Compare(a.TimezoneA+"\x00"+a.TimezoneB, b.TimezoneA+"\x00"+b.TimezoneB)
	})
	return report, nil
}

func collectRingArcs(arcs map[[sha256.Size]byte]*arc, timezone string, originalPB, simplifiedPB []*pb.Point) error {
	original := uniquePoints(originalPB)
	simplified := uniquePoints(simplifiedPB)
	if len(original) < 3 || len(simplified) < 3 {
		return errors.New("ring has fewer than three unique points")
	}

	positions := make(map[point][]int, len(original))
	for idx, p := range original {
		positions[p] = append(positions[p], idx)
	}
	indices := make([]int, len(simplified))
	previous := -1
	for idx, p := range simplified {
		candidates := positions[p]
		if len(candidates) == 0 {
			return fmt.Errorf("simplified vertex %.7f,%.7f is absent from baseline", p.lng, p.lat)
		}
		if idx == 0 {
			indices[idx] = candidates[0]
			previous = candidates[0]
			continue
		}
		chosen := -1
		for _, candidate := range candidates {
			unwrapped := candidate
			if unwrapped <= previous {
				unwrapped += len(original)
			}
			if chosen == -1 || unwrapped < chosen {
				chosen = unwrapped
			}
		}
		indices[idx] = chosen
		previous = chosen
	}
	if indices[len(indices)-1] >= indices[0]+len(original) {
		return errors.New("simplified vertices do not follow baseline ring order")
	}

	for idx := range simplified {
		start := indices[idx]
		end := indices[(idx+1)%len(indices)]
		if idx == len(indices)-1 {
			end += len(original)
		}
		chain := make([]point, 0, end-start+1)
		for cursor := start; cursor <= end; cursor++ {
			chain = append(chain, original[cursor%len(original)])
		}
		key := canonicalChainKey(chain)
		item, exists := arcs[key]
		if !exists {
			item = &arc{original: chain}
			arcs[key] = item
		}
		if !slices.Contains(item.zones, timezone) {
			item.zones = append(item.zones, timezone)
		}
	}
	return nil
}

// arcDeviationEstimate is a cheap upper-ballpark of how far the chain strays
// from its end-to-end great-circle chord. It is only used to order work; the
// certification never trusts it.
func arcDeviationEstimate(chain []point) float64 {
	if len(chain) <= 2 {
		return 0
	}
	va := vectorOf(chain[0])
	vb := vectorOf(chain[len(chain)-1])
	var estimate float64
	for _, p := range chain[1 : len(chain)-1] {
		estimate = math.Max(estimate, angleToSegment(vectorOf(p), va, vb))
	}
	return estimate * authalicRadiusM
}

func analyzeArc(shared *sharedMax, report *Report, original []point, zoneA, zoneB string, tolerance float64) {
	report.UniqueArcs++
	if len(original) == 2 {
		// The simplified edge shares both endpoints with the original chain, so a
		// two-point chain is geometrically identical to its replacement: distance
		// zero everywhere and no error strip. Both measurement directions
		// contribute the arc length to the zero bin.
		length := polylineLength(densifyGeographicPolyline(original))
		report.OriginalLengthKM += length / 1000
		report.LengthDistances.add(0, 2*length)
		return
	}

	simplified := []point{original[0], original[len(original)-1]}
	denseOriginal := newDensePolyline(densifyGeographicPolyline(original))
	denseSimplified := newDensePolyline(densifyGeographicPolyline(simplified))
	length := polylineLength(denseOriginal.pts)
	report.OriginalLengthKM += length / 1000

	area := math.Abs(sphericalPolygonArea(append(slices.Clone(original), original[0])))
	if area > 1e-6 {
		report.ChangedArcs++
		report.ChangedLengthKM += length / 1000
		report.ErrorAreaKM2 += area / 1e6
		report.MaxStripAreaKM2 = math.Max(report.MaxStripAreaKM2, area/1e6)
		report.addPairArea(zoneA, zoneB, area/1e6)
		addAreaWidthSamples(report, denseOriginal, area)
	}

	certifyPolyline(shared, report, denseOriginal, denseSimplified, zoneA, zoneB, tolerance)
	certifyPolyline(shared, report, denseSimplified, denseOriginal, zoneA, zoneB, tolerance)
}

// certifyPolyline measures every densified segment midpoint of source against
// target (length-weighted distribution) and certifies the maximum displacement
// with Lipschitz interval subdivision. Vertex distances are computed once and
// reused as interval endpoint distances.
func certifyPolyline(shared *sharedMax, report *Report, source, target *densePolyline, zoneA, zoneB string, tolerance float64) {
	distances := make([]float64, len(source.pts))
	for i, v := range source.vecs {
		d := target.distanceTo(v)
		distances[i] = d
		report.noteDistance(shared, d, v, zoneA, zoneB)
	}
	for i := 0; i+1 < len(source.pts); i++ {
		va, vb := source.vecs[i], source.vecs[i+1]
		lengthM := angle(va, vb) * authalicRadiusM
		if lengthM == 0 {
			continue
		}
		vmid := greatCircleMidpoint(va, vb)
		dm := target.distanceTo(vmid)
		report.LengthDistances.add(dm, lengthM)
		report.noteDistance(shared, dm, vmid, zoneA, zoneB)
		upper := math.Max(distances[i], math.Max(distances[i+1], dm)) + lengthM/4
		if upper <= shared.load()+tolerance {
			continue
		}
		certifyInterval(shared, report, va, vmid, distances[i], dm, target, zoneA, zoneB, tolerance, 1)
		certifyInterval(shared, report, vmid, vb, dm, distances[i+1], target, zoneA, zoneB, tolerance, 1)
	}
}

func certifyInterval(shared *sharedMax, report *Report, va, vb vector, da, db float64, target *densePolyline, zoneA, zoneB string, tolerance float64, depth int) {
	lengthM := angle(va, vb) * authalicRadiusM
	if lengthM == 0 {
		return
	}
	vmid := greatCircleMidpoint(va, vb)
	dm := target.distanceTo(vmid)
	report.noteDistance(shared, dm, vmid, zoneA, zoneB)
	upper := math.Max(da, math.Max(db, dm)) + lengthM/4
	if upper <= shared.load()+tolerance || depth >= 60 {
		return
	}
	certifyInterval(shared, report, va, vmid, da, dm, target, zoneA, zoneB, tolerance, depth+1)
	certifyInterval(shared, report, vmid, vb, dm, db, target, zoneA, zoneB, tolerance, depth+1)
}

func addAreaWidthSamples(report *Report, chain *densePolyline, stripArea float64) {
	pts := chain.pts
	if len(pts) < 3 || stripArea == 0 {
		return
	}
	anchor := chain.vecs[0]
	type sample struct{ width, area float64 }
	samples := make([]sample, 0, len(pts)-2)
	var triangulatedArea float64
	var triangle [4]point
	for i := 1; i+1 < len(pts); i++ {
		triangle = [4]point{pts[0], pts[i], pts[i+1], pts[0]}
		area := math.Abs(sphericalPolygonArea(triangle[:]))
		if area == 0 {
			continue
		}
		centroid := normalize(add(add(anchor, chain.vecs[i]), chain.vecs[i+1]))
		samples = append(samples, sample{chain.distanceTo(centroid), area})
		triangulatedArea += area
	}
	if triangulatedArea == 0 {
		return
	}
	scale := stripArea / triangulatedArea
	for _, item := range samples {
		report.AreaWidths.add(item.width, item.area*scale)
	}
}

func (r *Report) noteDistance(shared *sharedMax, distance float64, at vector, zoneA, zoneB string) {
	if distance <= r.MaxCertifiedM {
		return
	}
	r.MaxCertifiedM = distance
	p := pointOf(at)
	r.MaxLocation = Location{Lng: p.lng, Lat: p.lat, DistanceM: distance, TimezoneA: zoneA, TimezoneB: zoneB}
	shared.update(distance)
}

func (r *Report) addPairArea(a, b string, area float64) {
	if r.pairAreas == nil {
		r.pairAreas = make(map[pairKey]float64)
	}
	r.pairAreas[pairKey{a: a, b: b}] += area
}

func (r *Report) merge(o *Report) {
	if o == nil {
		return
	}
	r.UniqueArcs += o.UniqueArcs
	r.ChangedArcs += o.ChangedArcs
	r.OriginalLengthKM += o.OriginalLengthKM
	r.ChangedLengthKM += o.ChangedLengthKM
	r.ErrorAreaKM2 += o.ErrorAreaKM2
	r.MaxStripAreaKM2 = math.Max(r.MaxStripAreaKM2, o.MaxStripAreaKM2)
	if o.MaxCertifiedM > r.MaxCertifiedM {
		r.MaxCertifiedM = o.MaxCertifiedM
		r.MaxLocation = o.MaxLocation
	}
	r.LengthDistances.merge(&o.LengthDistances)
	r.AreaWidths.merge(&o.AreaWidths)
	for key, area := range o.pairAreas {
		r.addPairArea(key.a, key.b, area)
	}
}

func zonePair(zones []string) (string, string) {
	names := slices.Clone(zones)
	sort.Strings(names)
	if len(names) == 0 {
		return "unknown", "unknown"
	}
	if len(names) == 1 {
		return names[0], "outside-dataset"
	}
	if len(names) > 2 {
		return names[0], strings.Join(names[1:], "+")
	}
	return names[0], names[1]
}

func Quantile(distribution *Distribution, q float64) float64 {
	if distribution == nil || distribution.total == 0 {
		return 0
	}
	keys := make([]int64, 0, len(distribution.bins))
	for key := range distribution.bins {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	target := math.Max(0, math.Min(1, q)) * distribution.total
	var cumulative float64
	for _, key := range keys {
		cumulative += distribution.bins[key]
		if cumulative >= target {
			return float64(key) * distributionResolutionM
		}
	}
	return float64(keys[len(keys)-1]) * distributionResolutionM
}

func ProportionAbove(distribution *Distribution, threshold float64) float64 {
	if distribution == nil || distribution.total == 0 {
		return 0
	}
	var above float64
	for key, weight := range distribution.bins {
		if float64(key)*distributionResolutionM > threshold {
			above += weight
		}
	}
	return above / distribution.total
}

func (d *Distribution) add(value, weight float64) {
	if weight <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	if d.bins == nil {
		d.bins = make(map[int64]float64)
	}
	key := int64(math.Round(value / distributionResolutionM))
	d.bins[key] += weight
	d.total += weight
}

func (d *Distribution) merge(o *Distribution) {
	if o == nil || o.total == 0 {
		return
	}
	if d.bins == nil {
		d.bins = make(map[int64]float64, len(o.bins))
	}
	for key, weight := range o.bins {
		d.bins[key] += weight
	}
	d.total += o.total
}

func uniquePoints(input []*pb.Point) []point {
	result := make([]point, 0, len(input))
	for _, p := range input {
		current := point{lng: float64(p.Lng), lat: float64(p.Lat)}
		if len(result) == 0 || result[len(result)-1] != current {
			result = append(result, current)
		}
	}
	if len(result) > 1 && result[0] == result[len(result)-1] {
		result = result[:len(result)-1]
	}
	return result
}

func canonicalChainKey(points []point) [sha256.Size]byte {
	forward := chainKey(points, false)
	reverse := chainKey(points, true)
	if bytes.Compare(reverse[:], forward[:]) < 0 {
		return reverse
	}
	return forward
}

func chainKey(points []point, reverse bool) [sha256.Size]byte {
	h := sha256.New()
	var encoded [16]byte
	for i := range points {
		idx := i
		if reverse {
			idx = len(points) - 1 - i
		}
		binary.LittleEndian.PutUint64(encoded[:8], math.Float64bits(points[idx].lng))
		binary.LittleEndian.PutUint64(encoded[8:], math.Float64bits(points[idx].lat))
		_, _ = h.Write(encoded[:])
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

func polylineLength(points []point) float64 {
	var result float64
	for i := 0; i+1 < len(points); i++ {
		result += angularDistance(points[i], points[i+1]) * authalicRadiusM
	}
	return result
}

// densifyGeographicPolyline preserves GeoJSON's longitude-latitude linear
// interpolation while replacing long edges with short great-circle chords.
// Edges are split recursively on the measured midpoint sagitta, so meridian
// edges — which already follow great circles — stay coarse even when they
// touch the poles, while east-west edges near the poles subdivide until the
// small-circle sagitta fits the budget.
func densifyGeographicPolyline(points []point) []point {
	if len(points) < 2 {
		return slices.Clone(points)
	}
	result := make([]point, 0, len(points))
	result = append(result, points[0])
	for i := 0; i+1 < len(points); i++ {
		result = appendDensifiedEdge(result, points[i], points[i+1], 0)
	}
	return result
}

// appendDensifiedEdge appends the densified interior of edge a-b plus b
// itself. A chord is accepted once it is shorter than maxGeographicChordM and
// the geographic midpoint deviates from the great-circle chord by less than
// half the sagitta budget; the halved budget covers deviation away from the
// tested midpoint.
func appendDensifiedEdge(dst []point, a, b point, depth int) []point {
	mid := interpolateGeographic(a, b, 0.5)
	va, vb := vectorOf(a), vectorOf(b)
	lengthM := angle(va, vb) * authalicRadiusM
	if depth >= 40 ||
		(lengthM <= maxGeographicChordM &&
			angleToSegment(vectorOf(mid), va, vb)*authalicRadiusM <= targetChordSagittaM/2) {
		return append(dst, b)
	}
	dst = appendDensifiedEdge(dst, a, mid, depth+1)
	return appendDensifiedEdge(dst, mid, b, depth+1)
}

func interpolateGeographic(a, b point, t float64) point {
	dlng := b.lng - a.lng
	for dlng > 180 {
		dlng -= 360
	}
	for dlng < -180 {
		dlng += 360
	}
	lng := a.lng + t*dlng
	for lng > 180 {
		lng -= 360
	}
	for lng < -180 {
		lng += 360
	}
	return point{lng: lng, lat: a.lat + t*(b.lat-a.lat)}
}

// densePolyline caches unit vectors for every vertex and groups consecutive
// segments into spherical-cap blocks. distanceTo prunes whole blocks whose cap
// lower bound cannot beat the current best, turning the nearest-segment scan
// from O(n) into roughly O(n/blockSize + blockSize).
type densePolyline struct {
	pts          []point
	vecs         []vector
	blockCenters []vector
	blockRadii   []float64
}

const denseBlockSegments = 16

func newDensePolyline(pts []point) *densePolyline {
	d := &densePolyline{pts: pts, vecs: make([]vector, len(pts))}
	for i, p := range pts {
		d.vecs[i] = vectorOf(p)
	}
	segments := len(pts) - 1
	if segments < 2*denseBlockSegments {
		return d
	}
	blocks := (segments + denseBlockSegments - 1) / denseBlockSegments
	d.blockCenters = make([]vector, blocks)
	d.blockRadii = make([]float64, blocks)
	for b := range blocks {
		start := b * denseBlockSegments
		end := min(start+denseBlockSegments, segments)
		sum := vector{}
		for i := start; i <= end; i++ {
			sum = add(sum, d.vecs[i])
		}
		center := d.vecs[start]
		if n := norm(sum); n > 1e-15 {
			center = scale(sum, 1/n)
		}
		var radius float64
		for i := start; i <= end; i++ {
			radius = math.Max(radius, angle(center, d.vecs[i]))
		}
		// Great-circle arcs between vertices inside a spherical cap stay inside
		// the cap (caps smaller than a hemisphere are geodesically convex), so the
		// vertex radius bounds every point of the block's segments.
		d.blockCenters[b] = center
		d.blockRadii[b] = radius
	}
	return d
}

func (d *densePolyline) distanceTo(vp vector) float64 {
	segments := len(d.pts) - 1
	best := math.Inf(1)
	if d.blockCenters == nil {
		for i := range segments {
			best = math.Min(best, angleToSegment(vp, d.vecs[i], d.vecs[i+1]))
		}
		return best * authalicRadiusM
	}
	for b := range d.blockCenters {
		if angle(vp, d.blockCenters[b])-d.blockRadii[b] >= best {
			continue
		}
		start := b * denseBlockSegments
		end := min(start+denseBlockSegments, segments)
		for i := start; i < end; i++ {
			best = math.Min(best, angleToSegment(vp, d.vecs[i], d.vecs[i+1]))
		}
	}
	return best * authalicRadiusM
}

func distancePointToPolyline(p point, line []point) float64 {
	vp := vectorOf(p)
	best := math.Inf(1)
	for i := 0; i+1 < len(line); i++ {
		best = math.Min(best, angleToSegment(vp, vectorOf(line[i]), vectorOf(line[i+1])))
	}
	return best * authalicRadiusM
}

func angleToSegment(vp, va, vb vector) float64 {
	n := cross(va, vb)
	normN := norm(n)
	if normN < 1e-15 {
		return math.Min(angle(vp, va), angle(vp, vb))
	}
	n = scale(n, 1/normN)
	projection := normalize(sub(vp, scale(n, dot(vp, n))))
	if dot(projection, vp) < 0 {
		projection = scale(projection, -1)
	}
	ab := angle(va, vb)
	if math.Abs(angle(va, projection)+angle(projection, vb)-ab) <= 1e-10 {
		return angle(vp, projection)
	}
	return math.Min(angle(vp, va), angle(vp, vb))
}

func angularDistance(a, b point) float64 { return angle(vectorOf(a), vectorOf(b)) }

// greatCircleMidpoint is the t=0.5 spherical interpolation: the slerp weights
// are equal there, so the midpoint is simply the normalized vector sum.
func greatCircleMidpoint(va, vb vector) vector {
	sum := add(va, vb)
	n := norm(sum)
	if n < 1e-15 {
		return va
	}
	return scale(sum, 1/n)
}

func sphericalPolygonArea(points []point) float64 {
	if len(points) < 4 {
		return 0
	}
	var sum float64
	for i := 0; i+1 < len(points); i++ {
		lon1, lat1 := radians(points[i].lng), radians(points[i].lat)
		lon2, lat2 := radians(points[i+1].lng), radians(points[i+1].lat)
		dlon := lon2 - lon1
		for dlon > math.Pi {
			dlon -= 2 * math.Pi
		}
		for dlon < -math.Pi {
			dlon += 2 * math.Pi
		}
		sum += dlon * (2 + math.Sin(lat1) + math.Sin(lat2))
	}
	return -sum * authalicRadiusM * authalicRadiusM / 2
}

type vector struct{ x, y, z float64 }

func vectorOf(p point) vector {
	lon, lat := radians(p.lng), radians(p.lat)
	c := math.Cos(lat)
	return vector{c * math.Cos(lon), c * math.Sin(lon), math.Sin(lat)}
}
func pointOf(v vector) point {
	return point{lng: math.Atan2(v.y, v.x) * 180 / math.Pi, lat: math.Atan2(v.z, math.Hypot(v.x, v.y)) * 180 / math.Pi}
}
func radians(v float64) float64 { return v * math.Pi / 180 }
func dot(a, b vector) float64   { return a.x*b.x + a.y*b.y + a.z*b.z }
func cross(a, b vector) vector {
	return vector{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func add(a, b vector) vector           { return vector{a.x + b.x, a.y + b.y, a.z + b.z} }
func sub(a, b vector) vector           { return vector{a.x - b.x, a.y - b.y, a.z - b.z} }
func scale(a vector, s float64) vector { return vector{a.x * s, a.y * s, a.z * s} }
func norm(a vector) float64            { return math.Sqrt(dot(a, a)) }
func normalize(a vector) vector {
	n := norm(a)
	if n == 0 {
		return a
	}
	return scale(a, 1/n)
}
func angle(a, b vector) float64 { return math.Atan2(norm(cross(a, b)), dot(a, b)) }
