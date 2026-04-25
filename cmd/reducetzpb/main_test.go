package main

import (
	"bytes"
	"strings"
	"testing"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/topology"
)

func TestCollectStats(t *testing.T) {
	input := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "A",
				Polygons: []*pb.Polygon{
					{
						Points: []*pb.Point{
							{Lng: 0, Lat: 0},
							{Lng: 1, Lat: 0},
							{Lng: 1, Lat: 1},
							{Lng: 0, Lat: 0},
						},
						Holes: []*pb.Polygon{
							{
								Points: []*pb.Point{
									{Lng: 0.2, Lat: 0.2},
									{Lng: 0.4, Lat: 0.2},
									{Lng: 0.3, Lat: 0.3},
									{Lng: 0.2, Lat: 0.2},
								},
							},
						},
					},
				},
			},
		},
	}

	stats := collectStats(input)
	if stats.Timezones != 1 || stats.Polygons != 1 || stats.Holes != 1 || stats.Points != 8 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestPrintReport(t *testing.T) {
	before := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "A",
				Polygons: []*pb.Polygon{
					{
						Points: []*pb.Point{
							{Lng: 0, Lat: 0},
							{Lng: 1, Lat: 0},
							{Lng: 1, Lat: 1},
							{Lng: 0, Lat: 1},
							{Lng: 0, Lat: 0},
						},
					},
				},
			},
		},
	}
	after := &pb.Timezones{
		Version: "test",
		Timezones: []*pb.Timezone{
			{
				Name: "A",
				Polygons: []*pb.Polygon{
					{
						Points: []*pb.Point{
							{Lng: 0, Lat: 0},
							{Lng: 1, Lat: 0},
							{Lng: 1, Lat: 1},
							{Lng: 0, Lat: 0},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	printReport(&buf, before, after, true, 0.001, topology.Stats{}, false)
	out := buf.String()

	for _, want := range []string{
		"mode: topology",
		"epsilon: 0.001000",
		"points=5",
		"points=4",
		"points=20.00%",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q in %q", want, out)
		}
	}
}
