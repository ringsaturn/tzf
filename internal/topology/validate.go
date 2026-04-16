package topology

import (
	"fmt"
	"math"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

const minRingArea = 1e-12

type ValidateOptions struct {
	CheckSelfIntersections bool
	// CheckSameDirectionSharedEdges reports an error when two different rings
	// share an edge in the same direction. Enable this for strict correctness
	// checks. Disable for reduction output validation because some source data
	// (disputed territories, antimeridian splits) contains intentional same-
	// direction overlaps that should be preserved, not rejected.
	CheckSameDirectionSharedEdges bool
}

func DefaultValidateOptions() ValidateOptions {
	return ValidateOptions{
		CheckSelfIntersections:        true,
		CheckSameDirectionSharedEdges: true,
	}
}

func ReductionValidateOptions() ValidateOptions {
	return ValidateOptions{
		CheckSelfIntersections:        false,
		CheckSameDirectionSharedEdges: false,
	}
}

func Validate(input *pb.Timezones) error {
	return ValidateWithOptions(input, DefaultValidateOptions())
}

func ValidateWithOptions(input *pb.Timezones, opts ValidateOptions) error {
	if input == nil {
		return nil
	}

	edgeIndex := make(map[edgeKey][]edgeUse)
	for timezoneIdx, timezone := range input.Timezones {
		for polygonIdx, polygon := range timezone.Polygons {
			if err := validateRing(
				ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: -1},
				polygon.Points,
				false,
				opts,
				edgeIndex,
			); err != nil {
				return err
			}
			for holeIdx, hole := range polygon.Holes {
				if err := validateRing(
					ringRef{TimezoneIdx: timezoneIdx, PolygonIdx: polygonIdx, HoleIdx: holeIdx},
					hole.Points,
					true,
					opts,
					edgeIndex,
				); err != nil {
					return err
				}
			}
		}
	}

	if opts.CheckSameDirectionSharedEdges {
		for key, uses := range edgeIndex {
			if len(uses) != 2 {
				continue
			}
			if uses[0].Ring == uses[1].Ring {
				continue
			}
			if uses[0].From == uses[1].From && uses[0].To == uses[1].To {
				return fmt.Errorf(
					"topology: shared edge has same direction between rings %+v and %+v for edge %+v",
					uses[0].Ring,
					uses[1].Ring,
					key,
				)
			}
		}
	}

	return nil
}

func MustValidate(input *pb.Timezones) {
	if err := Validate(input); err != nil {
		panic(err)
	}
}

func MustValidateForReduction(input *pb.Timezones) {
	if err := ValidateWithOptions(input, ReductionValidateOptions()); err != nil {
		panic(err)
	}
}

func validateRing(
	ref ringRef,
	points []*pb.Point,
	isHole bool,
	opts ValidateOptions,
	edgeIndex map[edgeKey][]edgeUse,
) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 4 {
		return fmt.Errorf("topology: ring %+v has fewer than 4 points", ref)
	}
	if !samePoint(points[0], points[len(points)-1]) {
		return fmt.Errorf("topology: ring %+v is not closed", ref)
	}

	unique := ringUniquePoints(points)
	if len(unique) < 3 {
		return fmt.Errorf("topology: ring %+v has fewer than 3 unique points", ref)
	}
	if opts.CheckSelfIntersections && hasSelfIntersection(unique) {
		return fmt.Errorf("topology: ring %+v self intersects", ref)
	}
	area := signedArea(unique)
	if math.Abs(area) <= minRingArea {
		return fmt.Errorf("topology: ring %+v has degenerate area", ref)
	}
	if isHole && area > 0 {
		return fmt.Errorf("topology: hole ring %+v has counterclockwise winding", ref)
	}
	if !isHole && area < 0 {
		return fmt.Errorf("topology: exterior ring %+v has clockwise winding", ref)
	}
	for idx, point := range unique {
		next := unique[(idx+1)%len(unique)]
		if samePoint(point, next) {
			return fmt.Errorf("topology: ring %+v has zero-length edge at index %d", ref, idx)
		}
		from := newPointKey(point)
		to := newPointKey(next)
		key := newEdgeKey(from, to)
		edgeIndex[key] = append(edgeIndex[key], edgeUse{
			Ring:    ref,
			EdgeIdx: idx,
			From:    from,
			To:      to,
		})
	}

	return nil
}

func signedArea(points []*pb.Point) float64 {
	if len(points) < 3 {
		return 0
	}
	area := 0.0
	for idx := range points {
		next := points[(idx+1)%len(points)]
		area += float64(points[idx].Lng)*float64(next.Lat) - float64(next.Lng)*float64(points[idx].Lat)
	}
	return area / 2
}

func hasSelfIntersection(points []*pb.Point) bool {
	if len(points) < 4 {
		return false
	}
	for i := range points {
		a1 := points[i]
		a2 := points[(i+1)%len(points)]
		for j := i + 1; j < len(points); j++ {
			if edgesShareVertex(i, j, len(points)) {
				continue
			}
			b1 := points[j]
			b2 := points[(j+1)%len(points)]
			if segmentsIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func edgesShareVertex(i, j, n int) bool {
	if i == j {
		return true
	}
	if (i+1)%n == j || (j+1)%n == i {
		return true
	}
	return false
}

func segmentsIntersect(a1, a2, b1, b2 *pb.Point) bool {
	o1 := orientation(a1, a2, b1)
	o2 := orientation(a1, a2, b2)
	o3 := orientation(b1, b2, a1)
	o4 := orientation(b1, b2, a2)

	if o1 != o2 && o3 != o4 {
		return true
	}
	if o1 == 0 && pointOnClosedSegment(b1, a1, a2) {
		return true
	}
	if o2 == 0 && pointOnClosedSegment(b2, a1, a2) {
		return true
	}
	if o3 == 0 && pointOnClosedSegment(a1, b1, b2) {
		return true
	}
	if o4 == 0 && pointOnClosedSegment(a2, b1, b2) {
		return true
	}
	return false
}

func orientation(a, b, c *pb.Point) int {
	cross := (float64(b.Lat)-float64(a.Lat))*(float64(c.Lng)-float64(b.Lng)) -
		(float64(b.Lng)-float64(a.Lng))*(float64(c.Lat)-float64(b.Lat))
	if math.Abs(cross) <= snapTolerance {
		return 0
	}
	if cross > 0 {
		return 1
	}
	return -1
}

func pointOnClosedSegment(point, from, to *pb.Point) bool {
	ok, t := pointOnSegment(point, from, to)
	if !ok {
		return false
	}
	return t >= 0 && t <= 1
}
