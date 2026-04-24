package polyline

import (
	"math"
	"testing"
)

// reference examples from the Google Polyline spec:
// https://developers.google.com/maps/documentation/utilities/polylinealgorithm
// Note: the spec uses [lat, lng] order; the codec is order-agnostic.
var specCases = []struct {
	coords [][]float64
	enc    []byte
}{
	{
		coords: [][]float64{{38.5, -120.2}, {40.7, -120.95}, {43.252, -126.453}},
		enc:    []byte("_p~iF~ps|U_ulLnnqC_mqNvxq`@"),
	},
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	for _, tc := range specCases {
		got := EncodeCoords(tc.coords)
		if string(got) != string(tc.enc) {
			t.Errorf("EncodeCoords = %q, want %q", got, tc.enc)
		}

		decoded, rem, err := DecodeCoords(tc.enc)
		if err != nil {
			t.Fatalf("DecodeCoords error: %v", err)
		}
		if len(rem) != 0 {
			t.Errorf("unconsumed bytes: %q", rem)
		}
		if len(decoded) != len(tc.coords) {
			t.Fatalf("len(decoded)=%d, want %d", len(decoded), len(tc.coords))
		}
		const eps = 1e-5
		for i, c := range decoded {
			for j := range 2 {
				if math.Abs(c[j]-tc.coords[i][j]) > eps {
					t.Errorf("coord[%d][%d]: got %v, want %v", i, j, c[j], tc.coords[i][j])
				}
			}
		}
	}
}

func TestDecodeEmpty(t *testing.T) {
	coords, rem, err := DecodeCoords(nil)
	if err != nil || len(rem) != 0 || len(coords) != 0 {
		t.Errorf("DecodeCoords(nil) = %v, %v, %v; want nil, nil, nil", coords, rem, err)
	}
}

func TestEncodeEmpty(t *testing.T) {
	b := EncodeCoords(nil)
	if len(b) != 0 {
		t.Errorf("EncodeCoords(nil) = %q, want empty", b)
	}
}

func TestEncodeDecodeMany(t *testing.T) {
	// Exercise with a larger, high-precision coordinate set.
	orig := make([][]float64, 100)
	for i := range orig {
		orig[i] = []float64{
			float64(i)*0.12345 - 90,
			float64(i)*0.09876 - 45,
		}
	}
	encoded := EncodeCoords(orig)
	decoded, _, err := DecodeCoords(encoded)
	if err != nil {
		t.Fatal(err)
	}
	const eps = 1e-5
	for i, c := range decoded {
		for j := range 2 {
			if math.Abs(c[j]-orig[i][j]) > eps {
				t.Errorf("coord[%d][%d]: got %v, want %v", i, j, c[j], orig[i][j])
			}
		}
	}
}

func BenchmarkEncodeCoords(b *testing.B) {
	coords := make([][]float64, 500)
	for i := range coords {
		coords[i] = []float64{float64(i) * 0.001, float64(i) * 0.0005}
	}
	b.ResetTimer()
	for range b.N {
		EncodeCoords(coords)
	}
}

func BenchmarkDecodeCoords(b *testing.B) {
	coords := make([][]float64, 500)
	for i := range coords {
		coords[i] = []float64{float64(i) * 0.001, float64(i) * 0.0005}
	}
	enc := EncodeCoords(coords)
	b.ResetTimer()
	for range b.N {
		DecodeCoords(enc)
	}
}
