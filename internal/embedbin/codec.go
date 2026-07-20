package embedbin

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/ringsaturn/tzf/internal/geom"
)

func appendVarint(dst []byte, v int32) []byte {
	u := uint32(v<<1) ^ uint32(v>>31)
	for u >= 0x80 {
		dst = append(dst, byte(u)|0x80)
		u >>= 7
	}
	return append(dst, byte(u))
}

func encodePoints(points []geom.I32Point) []byte {
	if len(points) == 0 {
		return nil
	}
	out := make([]byte, 0, len(points)*3)
	out = appendVarint(out, points[0].X)
	out = appendVarint(out, points[0].Y)
	prev := points[0]
	for _, p := range points[1:] {
		out = appendVarint(out, p.X-prev.X)
		out = appendVarint(out, p.Y-prev.Y)
		prev = p
	}
	return out
}

func putBBox(dst []byte, off int, b bbox) {
	binary.LittleEndian.PutUint32(dst[off:], uint32(b.minX))
	binary.LittleEndian.PutUint32(dst[off+4:], uint32(b.minY))
	binary.LittleEndian.PutUint32(dst[off+8:], uint32(b.maxX))
	binary.LittleEndian.PutUint32(dst[off+12:], uint32(b.maxY))
}

func getBBox(src []byte, off int) bbox {
	return bbox{
		minX: int32(binary.LittleEndian.Uint32(src[off:])),
		minY: int32(binary.LittleEndian.Uint32(src[off+4:])),
		maxX: int32(binary.LittleEndian.Uint32(src[off+8:])),
		maxY: int32(binary.LittleEndian.Uint32(src[off+12:])),
	}
}

func addDelta(prev, delta int32) (int32, error) {
	v := int64(prev) + int64(delta)
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("%w: coordinate overflow", ErrMalformed)
	}
	return int32(v), nil
}
