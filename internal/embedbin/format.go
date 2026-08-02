// Package embedbin implements the TZF embedded binary format.
package embedbin

import (
	"errors"
	"fmt"
	"math"

	"github.com/ringsaturn/tzf/internal/geom"
)

const (
	headerSize      = 64
	sectionEntryLen = 16
	footerSize      = 4
	formatMajor     = 1
	formatMinor     = 0
	coordScale      = uint32(100000)
	defaultChunk    = 256

	flagGrid       = uint32(1 << 0)
	flagNoShortcut = uint32(1 << 1)

	sectionNames    = uint32(1)
	sectionTZDir    = uint32(2)
	sectionPolyDir  = uint32(3)
	sectionRingDir  = uint32(4)
	sectionRingOps  = uint32(5)
	sectionGroupDir = uint32(6)
	sectionChunkDir = uint32(7)
	sectionGrid     = uint32(8)
	sectionPoints   = uint32(9)

	tzRecordLen    = uint32(24)
	polyRecordLen  = uint32(24)
	ringRecordLen  = uint32(28)
	groupRecordLen = uint32(44)
	chunkRecordLen = uint32(24)
)

var (
	ErrMalformed      = errors.New("embedbin: malformed file")
	ErrBufferTooSmall = errors.New("embedbin: destination buffer too small")
	ErrIndex          = errors.New("embedbin: timezone index out of range")
)

type bbox struct {
	minX int32
	minY int32
	maxX int32
	maxY int32
}

func emptyBBox() bbox {
	return bbox{minX: math.MaxInt32, minY: math.MaxInt32, maxX: math.MinInt32, maxY: math.MinInt32}
}

func (b *bbox) add(p geom.I32Point) {
	b.minX = min(b.minX, p.X)
	b.minY = min(b.minY, p.Y)
	b.maxX = max(b.maxX, p.X)
	b.maxY = max(b.maxY, p.Y)
}

func (b *bbox) union(o bbox) {
	b.minX = min(b.minX, o.minX)
	b.minY = min(b.minY, o.minY)
	b.maxX = max(b.maxX, o.maxX)
	b.maxY = max(b.maxY, o.maxY)
}

func (b bbox) ordered() bool {
	return b.minX <= b.maxX && b.minY <= b.maxY
}

func (b bbox) inDomain() bool {
	return b.ordered() &&
		b.minX >= -18000000 && b.maxX <= 18000000 &&
		b.minY >= -9000000 && b.maxY <= 9000000
}

func (b bbox) contains(x, y float64) bool {
	return x >= float64(b.minX) && x <= float64(b.maxX) &&
		y >= float64(b.minY) && y <= float64(b.maxY)
}

func (b bbox) rayRelevant(x, y float64) bool {
	return y >= float64(b.minY) && y <= float64(b.maxY) && float64(b.maxX) >= x
}

func (b bbox) containsBBox(o bbox) bool {
	return b.minX <= o.minX && b.minY <= o.minY && b.maxX >= o.maxX && b.maxY >= o.maxY
}

func pointInDomain(p geom.I32Point) bool {
	return p.X >= -18000000 && p.X <= 18000000 && p.Y >= -9000000 && p.Y <= 9000000
}

func align4(n uint64) uint64 {
	return (n + 3) &^ 3
}

func checkedU16(name string, n int) (uint16, error) {
	if n < 0 || n > math.MaxUint16 {
		return 0, fmt.Errorf("%s: %w: %d exceeds uint16", name, ErrMalformed, n)
	}
	return uint16(n), nil
}

func checkedU32(name string, n uint64) (uint32, error) {
	if n > math.MaxUint32 {
		return 0, fmt.Errorf("%s: %w: %d exceeds uint32", name, ErrMalformed, n)
	}
	return uint32(n), nil
}

func samePoint(a, b geom.I32Point) bool {
	return a.X == b.X && a.Y == b.Y
}
