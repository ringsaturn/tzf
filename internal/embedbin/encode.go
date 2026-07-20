package embedbin

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/geom"
	"github.com/ringsaturn/tzf/internal/gridindex"
	"github.com/ringsaturn/tzf/internal/polyline"
)

// EncodeOptions controls host-side .tzb generation.
type EncodeOptions struct {
	ChunkTarget   int
	AllowShortcut bool
}

type encChunk struct {
	points []geom.I32Point
	bbox   bbox
	off    uint32
}

type encGroup struct {
	points []geom.I32Point
	chunks []encChunk
	bbox   bbox
}

type encRing struct {
	opFirst    uint32
	pointCount uint32
	ops        []uint32
	bbox       bbox
}

type encPoly struct {
	ringFirst uint32
	ringCount uint16
	bbox      bbox
}

type encTZ struct {
	polyFirst uint32
	polyCount uint16
	bbox      bbox
}

type encoder struct {
	chunkTarget int
	groups      []encGroup
	rings       []encRing
	polys       []encPoly
	tzs         []encTZ
	ops         []uint32
	edgeGroups  map[int32]edgeGroup
}

type edgeGroup struct {
	index uint32
	empty bool
}

// Encode converts a compressed topology protobuf into the v1 embedded layout.
func Encode(input *pb.CompressedTopoTimezones, opts EncodeOptions) ([]byte, error) {
	if input == nil {
		return nil, fmt.Errorf("input: %w: nil", ErrMalformed)
	}
	if input.Method != pb.CompressMethod_COMPRESS_METHOD_POLYLINE {
		return nil, fmt.Errorf("input: %w: unsupported compression method %v", ErrMalformed, input.Method)
	}
	target := opts.ChunkTarget
	if target == 0 {
		target = defaultChunk
	}
	if target < 1 || target > math.MaxUint16 {
		return nil, fmt.Errorf("chunk target: %w: %d", ErrMalformed, target)
	}
	if len(input.Timezones) == 0 || len(input.Timezones) > math.MaxUint16 {
		return nil, fmt.Errorf("timezone count: %w: %d", ErrMalformed, len(input.Timezones))
	}
	if len(input.Version) > 16 || !utf8.ValidString(input.Version) || strings.IndexByte(input.Version, 0) >= 0 {
		return nil, fmt.Errorf("data version: %w", ErrMalformed)
	}

	e := encoder{chunkTarget: target, edgeGroups: make(map[int32]edgeGroup, len(input.SharedEdges))}
	for _, edge := range input.SharedEdges {
		if edge == nil || edge.Id < 0 {
			return nil, fmt.Errorf("shared edge: %w: invalid id", ErrMalformed)
		}
		if _, exists := e.edgeGroups[edge.Id]; exists {
			return nil, fmt.Errorf("shared edge: %w: duplicate id %d", ErrMalformed, edge.Id)
		}
		points, err := decodePolyline(edge.Points)
		if err != nil {
			return nil, fmt.Errorf("shared edge %d: %w", edge.Id, err)
		}
		points = cleanPoints(points)
		if len(points) < 2 {
			e.edgeGroups[edge.Id] = edgeGroup{empty: true}
			continue
		}
		idx, err := e.addGroup(points)
		if err != nil {
			return nil, fmt.Errorf("shared edge %d: %w", edge.Id, err)
		}
		e.edgeGroups[edge.Id] = edgeGroup{index: idx}
	}

	names := make([]string, len(input.Timezones))
	for i, tz := range input.Timezones {
		if tz == nil || tz.Name == "" || !utf8.ValidString(tz.Name) || strings.IndexByte(tz.Name, 0) >= 0 {
			return nil, fmt.Errorf("timezone %d name: %w", i, ErrMalformed)
		}
		if len(tz.Polygons) == 0 {
			return nil, fmt.Errorf("timezone %q: %w: no polygons", tz.Name, ErrMalformed)
		}
		names[i] = tz.Name
		first := len(e.polys)
		if uint64(first) > math.MaxUint32 {
			return nil, fmt.Errorf("POLYDIR: %w: index capacity", ErrMalformed)
		}
		tb := emptyBBox()
		for j, poly := range tz.Polygons {
			ep, err := e.addPolygon(poly)
			if err != nil {
				return nil, fmt.Errorf("timezone %q polygon %d: %w", tz.Name, j, err)
			}
			tb.union(ep.bbox)
		}
		count, err := checkedU16("timezone polygon count", len(e.polys)-first)
		if err != nil {
			return nil, err
		}
		e.tzs = append(e.tzs, encTZ{polyFirst: uint32(first), polyCount: count, bbox: tb})
	}

	grid := input.GridIndex
	if grid == nil {
		grid = gridindex.BuildFromCompressedTopoTimezones(input)
	}
	gridBytes, err := encodeGrid(grid, len(input.Timezones))
	if err != nil {
		return nil, err
	}
	return e.serialize(names, input.Version, uint32(target), opts.AllowShortcut, gridBytes)
}

func decodePolyline(data []byte) ([]geom.I32Point, error) {
	coords, err := polyline.DecodeCoordsInt32(data)
	if err != nil {
		return nil, fmt.Errorf("%w: polyline: %v", ErrMalformed, err)
	}
	points := make([]geom.I32Point, len(coords))
	for i, c := range coords {
		points[i] = geom.I32Point{X: c[0], Y: c[1]}
	}
	return points, nil
}

func (e *encoder) addGroup(points []geom.I32Point) (uint32, error) {
	cleaned := make([]geom.I32Point, 0, len(points))
	for _, p := range points {
		if len(cleaned) == 0 || !samePoint(cleaned[len(cleaned)-1], p) {
			cleaned = append(cleaned, p)
		}
	}
	points = cleaned
	if len(points) < 2 {
		return 0, fmt.Errorf("%w: group has %d points", ErrMalformed, len(points))
	}
	if uint64(len(points)) > math.MaxUint32 {
		return 0, fmt.Errorf("%w: group point count", ErrMalformed)
	}
	b := emptyBBox()
	for _, p := range points {
		if !pointInDomain(p) {
			return 0, fmt.Errorf("%w: coordinate outside storage domain", ErrMalformed)
		}
		b.add(p)
	}
	g := encGroup{points: slices.Clone(points), bbox: b}
	for start := 0; start < len(points); start += e.chunkTarget {
		end := min(start+e.chunkTarget, len(points))
		cb := emptyBBox()
		for _, p := range points[start:end] {
			cb.add(p)
		}
		if end < len(points) {
			cb.add(points[end])
		}
		g.chunks = append(g.chunks, encChunk{points: g.points[start:end], bbox: cb})
	}
	if len(g.chunks) > math.MaxUint16 {
		return 0, fmt.Errorf("%w: group chunk count", ErrMalformed)
	}
	if len(e.groups) >= math.MaxInt32 {
		return 0, fmt.Errorf("%w: group count", ErrMalformed)
	}
	idx := uint32(len(e.groups))
	e.groups = append(e.groups, g)
	return idx, nil
}

func (e *encoder) addRing(segments []*pb.CompressedRingSegment) (encRing, error) {
	if len(segments) == 0 {
		return encRing{}, fmt.Errorf("%w: empty ring", ErrMalformed)
	}
	var ops []uint32
	var run []geom.I32Point
	flush := func() error {
		if len(run) == 0 {
			return nil
		}
		cleaned := run[:0]
		for _, p := range run {
			if len(cleaned) == 0 || !samePoint(cleaned[len(cleaned)-1], p) {
				cleaned = append(cleaned, p)
			}
		}
		run = cleaned
		if len(run) < 2 {
			run = nil
			return nil
		}
		idx, err := e.addGroup(run)
		if err != nil {
			return err
		}
		ops = append(ops, idx)
		run = nil
		return nil
	}
	for _, segment := range segments {
		if segment == nil {
			return encRing{}, fmt.Errorf("%w: nil ring segment", ErrMalformed)
		}
		switch s := segment.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			if s.Inline == nil {
				return encRing{}, fmt.Errorf("%w: nil inline segment", ErrMalformed)
			}
			points, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return encRing{}, err
			}
			if len(points) < 2 {
				return encRing{}, fmt.Errorf("%w: inline segment has %d points", ErrMalformed, len(points))
			}
			if len(run) > 0 {
				if !samePoint(run[len(run)-1], points[0]) {
					return encRing{}, fmt.Errorf("%w: disconnected inline segments", ErrMalformed)
				}
				points = points[1:]
			}
			run = append(run, points...)
		case *pb.CompressedRingSegment_EdgeForward:
			if err := flush(); err != nil {
				return encRing{}, err
			}
			edge, ok := e.edgeGroups[s.EdgeForward]
			if !ok {
				return encRing{}, fmt.Errorf("%w: missing edge %d", ErrMalformed, s.EdgeForward)
			}
			if !edge.empty {
				ops = append(ops, edge.index)
			}
		case *pb.CompressedRingSegment_EdgeReversed:
			if err := flush(); err != nil {
				return encRing{}, err
			}
			edge, ok := e.edgeGroups[s.EdgeReversed]
			if !ok {
				return encRing{}, fmt.Errorf("%w: missing edge %d", ErrMalformed, s.EdgeReversed)
			}
			if !edge.empty {
				ops = append(ops, edge.index|0x80000000)
			}
		default:
			return encRing{}, fmt.Errorf("%w: empty ring segment content", ErrMalformed)
		}
	}
	if err := flush(); err != nil {
		return encRing{}, err
	}
	if len(ops) == 0 || len(ops) > math.MaxUint16 {
		return encRing{}, fmt.Errorf("%w: ring op count %d", ErrMalformed, len(ops))
	}
	r := encRing{ops: ops, bbox: emptyBBox()}
	if uint64(len(e.ops)) > math.MaxUint32 {
		return encRing{}, fmt.Errorf("%w: RINGOPS index capacity", ErrMalformed)
	}
	r.opFirst = uint32(len(e.ops))
	var points uint64
	for i, word := range ops {
		g := &e.groups[word&0x7fffffff]
		r.bbox.union(g.bbox)
		points += uint64(len(g.points))
		next := ops[(i+1)%len(ops)]
		entry, exit := groupEndpoints(g, word>>31 != 0)
		ng := &e.groups[next&0x7fffffff]
		nextEntry, _ := groupEndpoints(ng, next>>31 != 0)
		_ = entry
		if !samePoint(exit, nextEntry) {
			return encRing{}, fmt.Errorf("%w: disconnected cyclic ring junction", ErrMalformed)
		}
	}
	points -= uint64(len(ops))
	if points < 3 || points > math.MaxUint32 {
		return encRing{}, fmt.Errorf("%w: ring open point count %d", ErrMalformed, points)
	}
	r.pointCount = uint32(points)
	e.ops = append(e.ops, ops...)
	e.rings = append(e.rings, r)
	return r, nil
}

func groupEndpoints(g *encGroup, reversed bool) (entry, exit geom.I32Point) {
	entry, exit = g.points[0], g.points[len(g.points)-1]
	if reversed {
		entry, exit = exit, entry
	}
	return entry, exit
}

func (e *encoder) addPolygon(poly *pb.CompressedTopoPolygon) (encPoly, error) {
	if poly == nil {
		return encPoly{}, fmt.Errorf("%w: nil polygon", ErrMalformed)
	}
	first := len(e.rings)
	if uint64(first) > math.MaxUint32 {
		return encPoly{}, fmt.Errorf("%w: RINGDIR index capacity", ErrMalformed)
	}
	ext, err := e.addRing(poly.Exterior)
	if err != nil {
		return encPoly{}, err
	}
	for i, hole := range poly.Holes {
		if hole == nil || len(hole.Holes) != 0 {
			return encPoly{}, fmt.Errorf("%w: nested or nil hole %d", ErrMalformed, i)
		}
		if _, err := e.addRing(hole.Exterior); err != nil {
			return encPoly{}, fmt.Errorf("hole %d: %w", i, err)
		}
	}
	count, err := checkedU16("polygon ring count", len(e.rings)-first)
	if err != nil {
		return encPoly{}, err
	}
	p := encPoly{ringFirst: uint32(first), ringCount: count, bbox: ext.bbox}
	e.polys = append(e.polys, p)
	return p, nil
}

func encodeGrid(grid *pb.GridIndex, tzCount int) ([]byte, error) {
	if grid == nil || len(grid.Cells) == 0 {
		return nil, fmt.Errorf("grid: %w: empty", ErrMalformed)
	}
	type key struct{ lng, lat int }
	cells := make(map[key][]uint32, len(grid.Cells))
	minLng, maxLng := math.MaxInt, math.MinInt
	minLat, maxLat := math.MaxInt, math.MinInt
	for _, cell := range grid.Cells {
		if cell == nil {
			return nil, fmt.Errorf("grid: %w: nil cell", ErrMalformed)
		}
		k := key{int(cell.Lng), int(cell.Lat)}
		if k.lng < -181 || k.lng > 181 || k.lat < -91 || k.lat > 91 {
			return nil, fmt.Errorf("grid: %w: key outside domain", ErrMalformed)
		}
		if _, exists := cells[k]; exists {
			return nil, fmt.Errorf("grid: %w: duplicate cell", ErrMalformed)
		}
		if len(cell.TzIndices) > 15 {
			return nil, fmt.Errorf("grid: %w: candidate count %d", ErrMalformed, len(cell.TzIndices))
		}
		var prev uint32
		for i, idx := range cell.TzIndices {
			if idx >= uint32(tzCount) || (i > 0 && idx <= prev) {
				return nil, fmt.Errorf("grid: %w: invalid candidate order or index", ErrMalformed)
			}
			prev = idx
		}
		cells[k] = slices.Clone(cell.TzIndices)
		minLng, maxLng = min(minLng, k.lng), max(maxLng, k.lng)
		minLat, maxLat = min(minLat, k.lat), max(maxLat, k.lat)
	}
	lngCells, latCells := maxLng-minLng+1, maxLat-minLat+1
	if minLng < -181 || minLat < -91 || maxLng > 181 || maxLat > 91 ||
		lngCells > math.MaxUint16 || latCells > math.MaxUint16 {
		return nil, fmt.Errorf("grid: %w: invalid extent", ErrMalformed)
	}
	words := make([]uint32, lngCells*latCells)
	var candidates []uint16
	interned := make(map[string]uint32)
	for y := 0; y < latCells; y++ {
		for x := 0; x < lngCells; x++ {
			list := cells[key{lng: minLng + x, lat: minLat + y}]
			if len(list) == 0 {
				continue
			}
			keyBytes := make([]byte, len(list)*2)
			for i, idx := range list {
				binary.LittleEndian.PutUint16(keyBytes[i*2:], uint16(idx))
			}
			s := string(keyBytes)
			off, ok := interned[s]
			if !ok {
				if len(candidates) >= 1<<28 || len(candidates)+len(list) >= 1<<28 {
					return nil, fmt.Errorf("grid: %w: candidate pool capacity", ErrMalformed)
				}
				off = uint32(len(candidates))
				interned[s] = off
				for _, idx := range list {
					candidates = append(candidates, uint16(idx))
				}
			}
			words[y*lngCells+x] = uint32(len(list))<<28 | off
		}
	}
	out := make([]byte, 12+4*len(words)+2*len(candidates))
	binary.LittleEndian.PutUint16(out[0:], uint16(int16(minLng)))
	binary.LittleEndian.PutUint16(out[2:], uint16(int16(minLat)))
	binary.LittleEndian.PutUint16(out[4:], uint16(lngCells))
	binary.LittleEndian.PutUint16(out[6:], uint16(latCells))
	binary.LittleEndian.PutUint32(out[8:], uint32(len(candidates)))
	for i, word := range words {
		binary.LittleEndian.PutUint32(out[12+i*4:], word)
	}
	base := 12 + 4*len(words)
	for i, idx := range candidates {
		binary.LittleEndian.PutUint16(out[base+i*2:], idx)
	}
	return out, nil
}

func (e *encoder) serialize(names []string, version string, target uint32, allowShortcut bool, grid []byte) ([]byte, error) {
	nameBlobLen := 0
	for _, name := range names {
		if uint64(nameBlobLen)+uint64(len(name)) > math.MaxUint32 {
			return nil, fmt.Errorf("NAMES: %w: blob capacity", ErrMalformed)
		}
		nameBlobLen += len(name)
	}
	nameSectionLen := uint64(4) + 4*uint64(len(names)+1) + uint64(nameBlobLen)
	if nameSectionLen > math.MaxUint32 {
		return nil, fmt.Errorf("NAMES: %w: section capacity", ErrMalformed)
	}
	nameSec := make([]byte, int(nameSectionLen))
	binary.LittleEndian.PutUint32(nameSec, uint32(nameBlobLen))
	pos := 0
	blob := 4 + 4*(len(names)+1)
	for i, name := range names {
		binary.LittleEndian.PutUint32(nameSec[4+i*4:], uint32(pos))
		copy(nameSec[blob+pos:], name)
		pos += len(name)
	}
	binary.LittleEndian.PutUint32(nameSec[4+len(names)*4:], uint32(pos))

	tzSec := make([]byte, int(tzRecordLen)*len(e.tzs))
	for i, t := range e.tzs {
		o := i * int(tzRecordLen)
		binary.LittleEndian.PutUint32(tzSec[o:], t.polyFirst)
		binary.LittleEndian.PutUint16(tzSec[o+4:], t.polyCount)
		putBBox(tzSec, o+8, t.bbox)
	}
	polySec := make([]byte, int(polyRecordLen)*len(e.polys))
	for i, p := range e.polys {
		o := i * int(polyRecordLen)
		binary.LittleEndian.PutUint32(polySec[o:], p.ringFirst)
		binary.LittleEndian.PutUint16(polySec[o+4:], p.ringCount)
		putBBox(polySec, o+8, p.bbox)
	}
	ringSec := make([]byte, int(ringRecordLen)*len(e.rings))
	for i, r := range e.rings {
		o := i * int(ringRecordLen)
		binary.LittleEndian.PutUint32(ringSec[o:], r.opFirst)
		binary.LittleEndian.PutUint32(ringSec[o+4:], r.pointCount)
		binary.LittleEndian.PutUint16(ringSec[o+8:], uint16(len(r.ops)))
		putBBox(ringSec, o+12, r.bbox)
	}
	opSec := make([]byte, 4*len(e.ops))
	for i, op := range e.ops {
		binary.LittleEndian.PutUint32(opSec[i*4:], op)
	}

	var chunks []encChunk
	pointsSec := make([]byte, 0)
	groupSec := make([]byte, int(groupRecordLen)*len(e.groups))
	for i := range e.groups {
		g := &e.groups[i]
		o := i * int(groupRecordLen)
		if uint64(len(chunks)) > math.MaxUint32 {
			return nil, fmt.Errorf("CHUNKDIR: %w: index capacity", ErrMalformed)
		}
		binary.LittleEndian.PutUint32(groupSec[o:], uint32(len(chunks)))
		binary.LittleEndian.PutUint32(groupSec[o+4:], uint32(len(g.points)))
		binary.LittleEndian.PutUint16(groupSec[o+8:], uint16(len(g.chunks)))
		binary.LittleEndian.PutUint32(groupSec[o+12:], uint32(g.points[0].X))
		binary.LittleEndian.PutUint32(groupSec[o+16:], uint32(g.points[0].Y))
		last := g.points[len(g.points)-1]
		binary.LittleEndian.PutUint32(groupSec[o+20:], uint32(last.X))
		binary.LittleEndian.PutUint32(groupSec[o+24:], uint32(last.Y))
		putBBox(groupSec, o+28, g.bbox)
		for j := range g.chunks {
			c := g.chunks[j]
			if uint64(len(pointsSec)) > math.MaxUint32 {
				return nil, fmt.Errorf("POINTS: %w: offset capacity", ErrMalformed)
			}
			c.off = uint32(len(pointsSec))
			pointsSec = append(pointsSec, encodePoints(c.points)...)
			chunks = append(chunks, c)
		}
	}
	chunkSec := make([]byte, int(chunkRecordLen)*len(chunks))
	for i, c := range chunks {
		o := i * int(chunkRecordLen)
		binary.LittleEndian.PutUint32(chunkSec[o:], c.off)
		binary.LittleEndian.PutUint16(chunkSec[o+4:], uint16(len(c.points)))
		putBBox(chunkSec, o+8, c.bbox)
	}

	sections := []struct {
		typ  uint32
		data []byte
	}{
		{sectionNames, nameSec}, {sectionTZDir, tzSec}, {sectionPolyDir, polySec},
		{sectionRingDir, ringSec}, {sectionRingOps, opSec}, {sectionGroupDir, groupSec},
		{sectionChunkDir, chunkSec}, {sectionGrid, grid}, {sectionPoints, pointsSec},
	}
	offsets := make([]uint32, len(sections))
	cursor := align4(headerSize + uint64(len(sections))*sectionEntryLen)
	for i, section := range sections {
		off, err := checkedU32("section offset", cursor)
		if err != nil {
			return nil, err
		}
		offsets[i] = off
		cursor += uint64(len(section.data))
		if i != len(sections)-1 {
			cursor = align4(cursor)
		}
	}
	fileSize := cursor + footerSize
	size32, err := checkedU32("file size", fileSize)
	if err != nil {
		return nil, err
	}
	if fileSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("file size: %w: host int capacity", ErrMalformed)
	}
	out := make([]byte, int(fileSize))
	copy(out[0:4], "TZFB")
	out[4], out[5] = formatMajor, formatMinor
	binary.LittleEndian.PutUint16(out[6:], headerSize)
	flags := flagGrid
	if !allowShortcut {
		flags |= flagNoShortcut
	}
	binary.LittleEndian.PutUint32(out[8:], flags)
	binary.LittleEndian.PutUint32(out[12:], coordScale)
	binary.LittleEndian.PutUint32(out[16:], size32)
	binary.LittleEndian.PutUint32(out[20:], uint32(len(sections)))
	copy(out[24:40], version)
	binary.LittleEndian.PutUint32(out[40:], uint32(len(names)))
	binary.LittleEndian.PutUint32(out[44:], target)
	for i, section := range sections {
		o := headerSize + i*sectionEntryLen
		binary.LittleEndian.PutUint32(out[o:], section.typ)
		binary.LittleEndian.PutUint32(out[o+4:], offsets[i])
		binary.LittleEndian.PutUint32(out[o+8:], uint32(len(section.data)))
		copy(out[offsets[i]:], section.data)
	}
	binary.LittleEndian.PutUint32(out[len(out)-4:], crc32.ChecksumIEEE(out[:len(out)-4]))
	return out, nil
}
