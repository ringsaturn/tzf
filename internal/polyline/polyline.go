// Package polyline implements the Google Maps Encoded Polyline algorithm.
//
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm
//
// This package is a modified version of github.com/twpayne/go-polyline (BSD-2-Clause).
package polyline

import (
	"errors"
	"math"
	"strconv"
)

// Sentinel errors matching the upstream library for compatibility.
var (
	ErrDimensionalMismatch  = errors.New("polyline: dimensional mismatch")
	ErrEmpty                = errors.New("polyline: empty")
	ErrInvalidByte          = errors.New("polyline: invalid byte")
	ErrOverflow             = errors.New("polyline: overflow")
	ErrUnterminatedSequence = errors.New("polyline: unterminated sequence")
)

const (
	defaultDim   = 2
	defaultScale = 1e5
)

func round(x float64) int {
	if x < 0 {
		return int(-math.Floor(-x + 0.5))
	}
	return int(math.Floor(x + 0.5))
}

// EncodeUint appends the variable-length encoding of u to buf and returns
// the extended slice.
func EncodeUint(buf []byte, u uint) []byte {
	for u >= 32 {
		buf = append(buf, byte((u&31)+95))
		u >>= 5
	}
	buf = append(buf, byte(u+63))
	return buf
}

// EncodeInt appends the zig-zag + variable-length encoding of i to buf.
func EncodeInt(buf []byte, i int) []byte {
	var u uint
	if i < 0 {
		u = uint(^(i << 1))
	} else {
		u = uint(i << 1)
	}
	return EncodeUint(buf, u)
}

// DecodeUint reads one variable-length unsigned integer from buf.
// Returns the value, the remaining bytes, and any error.
func DecodeUint(buf []byte) (uint, []byte, error) {
	if len(buf) == 0 {
		return 0, nil, ErrEmpty
	}
	n := strconv.IntSize / 5
	if n > len(buf) {
		n = len(buf)
	}
	var u, shift uint
	for i := 0; i < n; i++ {
		b := buf[i]
		switch {
		case 95 <= b && b < 127:
			u += (uint(b) - 95) << shift
			shift += 5
		case 63 <= b && b < 95:
			u += (uint(b) - 63) << shift
			return u, buf[i+1:], nil
		default:
			return 0, nil, ErrInvalidByte
		}
	}
	if len(buf) <= strconv.IntSize/5 {
		return 0, nil, ErrUnterminatedSequence
	}
	max := byte(1<<(strconv.IntSize-5*(strconv.IntSize/5)) - 1)
	b := buf[n]
	switch {
	case 63 <= b && b <= 63+max:
		u += (uint(b) - 63) << shift
		return u, buf[n+1:], nil
	case b < 127:
		return 0, nil, ErrOverflow
	default:
		return 0, nil, ErrInvalidByte
	}
}

// DecodeInt reads one zig-zag encoded signed integer from buf.
func DecodeInt(buf []byte) (int, []byte, error) {
	u, buf, err := DecodeUint(buf)
	if err != nil {
		return 0, nil, err
	}
	if u&1 == 0 {
		return int(u >> 1), buf, nil
	}
	if u == math.MaxUint {
		return math.MinInt, buf, nil
	}
	return -int((u + 1) >> 1), buf, nil
}

// EncodeCoords returns the default-codec encoding of a 2-D coordinate array.
// Coordinates are [lng, lat] pairs; successive values are delta-encoded.
func EncodeCoords(coords [][]float64) []byte {
	buf := make([]byte, 0, len(coords)*4)
	last := [defaultDim]int{}
	for _, coord := range coords {
		for i := range defaultDim {
			ex := round(defaultScale * coord[i])
			buf = EncodeInt(buf, ex-last[i])
			last[i] = ex
		}
	}
	return buf
}

// DecodeCoordsInt32 decodes a default-codec 2-D coordinate array from buf
// into raw 1e5-scaled integer coordinates ([lng, lat] pairs), without the
// float64 division that DecodeCoords performs.
func DecodeCoordsInt32(buf []byte) ([][2]int32, error) {
	if len(buf) == 0 {
		return nil, nil
	}

	var coords [][2]int32
	last := [defaultDim]int{}
	for len(buf) > 0 {
		var coord [2]int32
		for i := range defaultDim {
			v, rest, err := DecodeInt(buf)
			if err != nil {
				return nil, err
			}
			buf = rest
			last[i] += v
			if last[i] < math.MinInt32 || last[i] > math.MaxInt32 {
				return nil, ErrOverflow
			}
			coord[i] = int32(last[i])
		}
		coords = append(coords, coord)
	}
	return coords, nil
}

// DecodeCoords decodes a default-codec 2-D coordinate array from buf.
// Returns the coordinates, remaining unconsumed bytes, and any error.
func DecodeCoords(buf []byte) ([][]float64, []byte, error) {
	if len(buf) == 0 {
		return nil, buf, nil
	}

	// Decode first coordinate.
	first := make([]float64, defaultDim)
	var err error
	for i := range defaultDim {
		var v int
		v, buf, err = DecodeInt(buf)
		if err != nil {
			return nil, nil, err
		}
		first[i] = float64(v) / defaultScale
	}
	coords := [][]float64{first}

	// Decode subsequent coordinates as deltas.
	for len(buf) > 0 {
		coord := make([]float64, defaultDim)
		prev := coords[len(coords)-1]
		for i := range defaultDim {
			var v int
			v, buf, err = DecodeInt(buf)
			if err != nil {
				return nil, nil, err
			}
			coord[i] = prev[i] + float64(v)/defaultScale
		}
		coords = append(coords, coord)
	}

	return coords, nil, nil
}
