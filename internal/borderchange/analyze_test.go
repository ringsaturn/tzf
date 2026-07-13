package borderchange

import (
	"math"
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
)

func TestAnalyzeTriangleDent(t *testing.T) {
	original := dataset([][2]float32{{0, 0}, {0.5, 0.001}, {1, 0}, {1, 1}, {0, 1}, {0, 0}})
	simplified := dataset([][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}})
	report, err := Analyze(original, simplified, Options{CertificationToleranceM: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if report.ChangedArcs != 1 {
		t.Fatalf("changed arcs: got %d want 1", report.ChangedArcs)
	}
	if math.Abs(report.MaxCertifiedM-111.195) > 0.5 {
		t.Fatalf("maximum: got %.3f want about 111.195", report.MaxCertifiedM)
	}
	if report.ErrorAreaKM2 < 6 || report.ErrorAreaKM2 > 6.3 {
		t.Fatalf("area: got %.6f km2", report.ErrorAreaKM2)
	}
}

func TestSharedArcIsDeduplicated(t *testing.T) {
	left := [][2]float32{{0, 0}, {1, 0}, {1.001, 0.5}, {1, 1}, {0, 1}, {0, 0}}
	right := [][2]float32{{1, 1}, {1.001, 0.5}, {1, 0}, {2, 0}, {2, 1}, {1, 1}}
	original := &pb.Timezones{Timezones: []*pb.Timezone{
		{Name: "A", Polygons: []*pb.Polygon{{Points: pbPoints(left)}}},
		{Name: "B", Polygons: []*pb.Polygon{{Points: pbPoints(right)}}},
	}}
	leftSimple := [][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}
	rightSimple := [][2]float32{{1, 1}, {1, 0}, {2, 0}, {2, 1}, {1, 1}}
	simplified := &pb.Timezones{Timezones: []*pb.Timezone{
		{Name: "A", Polygons: []*pb.Polygon{{Points: pbPoints(leftSimple)}}},
		{Name: "B", Polygons: []*pb.Polygon{{Points: pbPoints(rightSimple)}}},
	}}
	report, err := Analyze(original, simplified, Options{CertificationToleranceM: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.ChangedArcs != 1 {
		t.Fatalf("changed arcs: got %d want 1", report.ChangedArcs)
	}
	if len(report.PairAreas) != 1 || report.PairAreas[0].TimezoneA != "A" || report.PairAreas[0].TimezoneB != "B" {
		t.Fatalf("unexpected pairs: %#v", report.PairAreas)
	}
}

func TestDensifyPreservesLongLatitudeEdge(t *testing.T) {
	line := densifyGeographicPolyline([]point{{lng: 90, lat: -86}, {lng: 180, lat: -86}})
	distance := distancePointToPolyline(point{lng: 135, lat: -86}, line)
	if distance > 0.5 {
		t.Fatalf("distance from geographic midpoint: got %.3f m want <= 0.5 m", distance)
	}
}

func TestDensifyCrossesAntimeridian(t *testing.T) {
	line := densifyGeographicPolyline([]point{{lng: 179, lat: 10}, {lng: -179, lat: 10}})
	distance := distancePointToPolyline(point{lng: 180, lat: 10}, line)
	if distance > 0.5 {
		t.Fatalf("distance at antimeridian: got %.3f m want <= 0.5 m", distance)
	}
}

func dataset(coords [][2]float32) *pb.Timezones {
	return &pb.Timezones{Timezones: []*pb.Timezone{{Name: "Test/Zone", Polygons: []*pb.Polygon{{Points: pbPoints(coords)}}}}}
}

func pbPoints(coords [][2]float32) []*pb.Point {
	result := make([]*pb.Point, 0, len(coords))
	for _, p := range coords {
		result = append(result, &pb.Point{Lng: p[0], Lat: p[1]})
	}
	return result
}
