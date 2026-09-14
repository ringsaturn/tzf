package embedbin

import (
	"fmt"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// Byte-backed decode fast path. The streamCursor route pays a byteAt method
// call per stream byte — the price of serving io.ReaderAt sources through one
// code path — which dominates the expansion loader's cost (~4.7M stream bytes
// on the lite dataset). Byte-backed readers instead decode by direct slice
// indexing here; validation is byte-for-byte the streamCursor path's: minimal
// varints, 32-bit bounds, exact chunk termination, storage-domain and
// overflow checks.

// sliceVarint decodes one zigzag-LEB128 varint from buf at offset i and
// returns the value and the next offset. The one- and two-byte cases — ~98%
// of stored deltas (measured 66%/32% on full, 26%/72% on lite) — are kept
// small enough for the inliner; longer encodings and every error case take
// the slow path.
func sliceVarint(buf []byte, i int) (int32, int, error) {
	if i < len(buf) {
		if b := buf[i]; b < 0x80 {
			u := uint32(b)
			return int32((u >> 1) ^ uint32(-int32(u&1))), i + 1, nil
		} else if i+1 < len(buf) {
			if b1 := buf[i+1]; b1 < 0x80 && b1 != 0 {
				u := uint32(b&0x7f) | uint32(b1)<<7
				return int32((u >> 1) ^ uint32(-int32(u&1))), i + 2, nil
			}
		}
	}
	return sliceVarintSlow(buf, i)
}

func sliceVarintSlow(buf []byte, i int) (int32, int, error) {
	var u uint32
	for j := 0; j < 5; j++ {
		if i >= len(buf) {
			return 0, 0, fmt.Errorf("%w: truncated varint", ErrMalformed)
		}
		b := buf[i]
		i++
		if j == 4 && b&0xf0 != 0 {
			return 0, 0, fmt.Errorf("%w: varint exceeds 32 bits", ErrMalformed)
		}
		u |= uint32(b&0x7f) << (7 * j)
		if b&0x80 == 0 {
			if j > 0 && b == 0 {
				return 0, 0, fmt.Errorf("%w: nonminimal varint", ErrMalformed)
			}
			return int32((u >> 1) ^ uint32(-int32(u&1))), i, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: unterminated varint", ErrMalformed)
}

// appendChunkPoints decodes one chunk's full point run onto dst. Byte-backed
// readers take the direct slice route; ReaderAt sources fall back to
// DecodeChunkPoints (per-chunk allocation plus copy, as before).
func (r *Reader) appendChunkPoints(dst []geom.I32Point, index uint32, chunk ChunkRecord) ([]geom.I32Point, error) {
	if r.data == nil {
		part, err := r.DecodeChunkPoints(index, chunk)
		if err != nil {
			return dst, err
		}
		return append(dst, part...), nil
	}
	start, end, err := r.chunkRange(index, chunk, nil)
	if err != nil {
		return dst, err
	}
	buf := r.data[start:end]
	i := 0
	var prev geom.I32Point
	for k := 0; k < int(chunk.Count); k++ {
		var dx, dy int32
		dx, i, err = sliceVarint(buf, i)
		if err != nil {
			return dst, err
		}
		dy, i, err = sliceVarint(buf, i)
		if err != nil {
			return dst, err
		}
		if k == 0 {
			prev = geom.I32Point{X: dx, Y: dy}
		} else {
			x, err := addDelta(prev.X, dx)
			if err != nil {
				return dst, err
			}
			y, err := addDelta(prev.Y, dy)
			if err != nil {
				return dst, err
			}
			prev = geom.I32Point{X: x, Y: y}
		}
		if !PointInDomain(prev) {
			return dst, fmt.Errorf("%w: point domain", ErrMalformed)
		}
		dst = append(dst, prev)
	}
	if i != len(buf) {
		return dst, fmt.Errorf("%w: chunk termination", ErrMalformed)
	}
	return dst, nil
}
