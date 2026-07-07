package preindex

import "testing"

func TestExcludePreIndexIncludesAbkhazia(t *testing.T) {
	points := []struct {
		name string
		lng  float64
		lat  float64
	}{
		{"gagra", 40.267, 43.278},
		{"pitsunda", 40.342, 43.160},
		{"sukhumi", 41.0167, 43.0015},
		{"ochamchire", 41.468, 42.712},
		{"abkhazia-west", 40.1, 43.0},
		{"mismatch-regression", 40.08, 43.34},
	}

	for _, point := range points {
		t.Run(point.name, func(t *testing.T) {
			if !excludePreIndex(point.lng, point.lat) {
				t.Fatalf("expected %.4f,%.4f to be excluded from preindex", point.lng, point.lat)
			}
		})
	}
}
