package embedenc

import (
	"encoding/binary"
	"fmt"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	"github.com/ringsaturn/tzf/v2/internal/gridindex"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/polyline"
)

// EncodeOptions controls host-side .tzb generation.
type EncodeOptions struct {
	ChunkTarget   int
	AllowShortcut bool
	// Preindex, when set, is embedded as the optional FUZZY section so a
	// single file can back both fuzzy and polygon lookups. Its Version must
	// equal the geometry input's Version.
	Preindex *pb.PreindexTimezones
}

type encChunk struct {
	points []geom.I32Point
	BBox   embedbin.BBox
	Off    uint32
}

type encGroup struct {
	points []geom.I32Point
	chunks []encChunk
	BBox   embedbin.BBox
}

type encRing struct {
	opFirst    uint32
	PointCount uint32
	ops        []uint32
	BBox       embedbin.BBox
}

type encPoly struct {
	ringFirst uint32
	ringCount uint16
	BBox      embedbin.BBox
}

type encTZ struct {
	polyFirst uint32
	polyCount uint16
	BBox      embedbin.BBox
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
	e, names, gridBytes, fuzzyBytes, target, err := build(input, opts)
	if err != nil {
		return nil, err
	}
	return e.serialize(names, input.Version, target, opts.AllowShortcut, gridBytes, fuzzyBytes)
}

// EncodeM converts a compressed topology protobuf into an M-profile (.tzm)
// file whose sections are the query-time structures (spec rev 1 §6): every
// ring stored as an open flat point run in FLATPOINTS, junction duplicates
// removed by the §5.1 expansion, directories identical to the E profile.
// ChunkTarget and AllowShortcut have no effect on the M output (chunking and
// the in-place shortcut flag are E-profile concepts).
func EncodeM(input *pb.CompressedTopoTimezones, opts EncodeOptions) ([]byte, error) {
	e, names, gridBytes, fuzzyBytes, _, err := build(input, opts)
	if err != nil {
		return nil, err
	}
	return e.serializeM(names, input.Version, gridBytes, fuzzyBytes)
}

// build runs the shared front half of both encoders: decode and clean the
// source topology into groups/rings/polygons/timezones plus the GRID and
// optional FUZZY section bytes.
func build(input *pb.CompressedTopoTimezones, opts EncodeOptions) (*encoder, []string, []byte, []byte, uint32, error) {
	fail := func(err error) (*encoder, []string, []byte, []byte, uint32, error) {
		return nil, nil, nil, nil, 0, err
	}
	if input == nil {
		return fail(fmt.Errorf("input: %w: nil", embedbin.ErrMalformed))
	}
	if input.Method != pb.CompressMethod_COMPRESS_METHOD_POLYLINE {
		return fail(fmt.Errorf("input: %w: unsupported compression method %v", embedbin.ErrMalformed, input.Method))
	}
	target := opts.ChunkTarget
	if target == 0 {
		target = embedbin.DefaultChunk
	}
	if target < 1 || target > math.MaxUint16 {
		return fail(fmt.Errorf("chunk target: %w: %d", embedbin.ErrMalformed, target))
	}
	if len(input.Timezones) == 0 || len(input.Timezones) > math.MaxUint16 {
		return fail(fmt.Errorf("timezone Count: %w: %d", embedbin.ErrMalformed, len(input.Timezones)))
	}
	if len(input.Version) > 16 || !utf8.ValidString(input.Version) || strings.IndexByte(input.Version, 0) >= 0 {
		return fail(fmt.Errorf("data version: %w", embedbin.ErrMalformed))
	}

	e := &encoder{chunkTarget: target, edgeGroups: make(map[int32]edgeGroup, len(input.SharedEdges))}
	for _, edge := range input.SharedEdges {
		if edge == nil || edge.Id < 0 {
			return fail(fmt.Errorf("shared edge: %w: invalid id", embedbin.ErrMalformed))
		}
		if _, exists := e.edgeGroups[edge.Id]; exists {
			return fail(fmt.Errorf("shared edge: %w: duplicate id %d", embedbin.ErrMalformed, edge.Id))
		}
		points, err := decodePolyline(edge.Points)
		if err != nil {
			return fail(fmt.Errorf("shared edge %d: %w", edge.Id, err))
		}
		points = cleanPoints(points)
		if len(points) < 2 {
			e.edgeGroups[edge.Id] = edgeGroup{empty: true}
			continue
		}
		idx, err := e.addGroup(points)
		if err != nil {
			return fail(fmt.Errorf("shared edge %d: %w", edge.Id, err))
		}
		e.edgeGroups[edge.Id] = edgeGroup{index: idx}
	}

	names := make([]string, len(input.Timezones))
	for i, tz := range input.Timezones {
		if tz == nil || tz.Name == "" || !utf8.ValidString(tz.Name) || strings.IndexByte(tz.Name, 0) >= 0 {
			return fail(fmt.Errorf("timezone %d name: %w", i, embedbin.ErrMalformed))
		}
		if len(tz.Polygons) == 0 {
			return fail(fmt.Errorf("timezone %q: %w: no polygons", tz.Name, embedbin.ErrMalformed))
		}
		names[i] = tz.Name
		first := len(e.polys)
		if uint64(first) > math.MaxUint32 {
			return fail(fmt.Errorf("POLYDIR: %w: index capacity", embedbin.ErrMalformed))
		}
		tb := embedbin.EmptyBBox()
		for j, poly := range tz.Polygons {
			ep, err := e.addPolygon(poly)
			if err != nil {
				return fail(fmt.Errorf("timezone %q polygon %d: %w", tz.Name, j, err))
			}
			tb.Union(ep.BBox)
		}
		count, err := embedbin.CheckedU16("timezone polygon count", len(e.polys)-first)
		if err != nil {
			return fail(err)
		}
		e.tzs = append(e.tzs, encTZ{polyFirst: uint32(first), polyCount: count, BBox: tb})
	}

	grid := input.GridIndex
	if grid == nil {
		grid = gridindex.BuildFromCompressedTopoTimezones(input)
	}
	gridBytes, err := encodeGrid(grid, len(input.Timezones))
	if err != nil {
		return fail(err)
	}
	var fuzzyBytes []byte
	if opts.Preindex != nil {
		fuzzyBytes, err = encodeFuzzy(opts.Preindex, names, input.Version)
		if err != nil {
			return fail(err)
		}
	}
	return e, names, gridBytes, fuzzyBytes, uint32(target), nil
}

func decodePolyline(data []byte) ([]geom.I32Point, error) {
	coords, err := polyline.DecodeCoordsInt32(data)
	if err != nil {
		return nil, fmt.Errorf("%w: polyline: %v", embedbin.ErrMalformed, err)
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
		if len(cleaned) == 0 || !embedbin.SamePoint(cleaned[len(cleaned)-1], p) {
			cleaned = append(cleaned, p)
		}
	}
	points = cleaned
	if len(points) < 2 {
		return 0, fmt.Errorf("%w: group has %d points", embedbin.ErrMalformed, len(points))
	}
	if uint64(len(points)) > math.MaxUint32 {
		return 0, fmt.Errorf("%w: group point count", embedbin.ErrMalformed)
	}
	b := embedbin.EmptyBBox()
	for _, p := range points {
		if !embedbin.PointInDomain(p) {
			return 0, fmt.Errorf("%w: coordinate outside storage domain", embedbin.ErrMalformed)
		}
		b.Add(p)
	}
	g := encGroup{points: slices.Clone(points), BBox: b}
	for start := 0; start < len(points); start += e.chunkTarget {
		end := min(start+e.chunkTarget, len(points))
		cb := embedbin.EmptyBBox()
		for _, p := range points[start:end] {
			cb.Add(p)
		}
		if end < len(points) {
			cb.Add(points[end])
		}
		g.chunks = append(g.chunks, encChunk{points: g.points[start:end], BBox: cb})
	}
	if len(g.chunks) > math.MaxUint16 {
		return 0, fmt.Errorf("%w: group chunk count", embedbin.ErrMalformed)
	}
	if len(e.groups) >= math.MaxInt32 {
		return 0, fmt.Errorf("%w: group count", embedbin.ErrMalformed)
	}
	idx := uint32(len(e.groups))
	e.groups = append(e.groups, g)
	return idx, nil
}

func (e *encoder) addRing(segments []*pb.CompressedRingSegment) (encRing, error) {
	if len(segments) == 0 {
		return encRing{}, fmt.Errorf("%w: empty ring", embedbin.ErrMalformed)
	}
	var ops []uint32
	var run []geom.I32Point
	flush := func() error {
		if len(run) == 0 {
			return nil
		}
		cleaned := run[:0]
		for _, p := range run {
			if len(cleaned) == 0 || !embedbin.SamePoint(cleaned[len(cleaned)-1], p) {
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
			return encRing{}, fmt.Errorf("%w: nil ring segment", embedbin.ErrMalformed)
		}
		switch s := segment.Content.(type) {
		case *pb.CompressedRingSegment_Inline:
			if s.Inline == nil {
				return encRing{}, fmt.Errorf("%w: nil inline segment", embedbin.ErrMalformed)
			}
			points, err := decodePolyline(s.Inline.Points)
			if err != nil {
				return encRing{}, err
			}
			if len(points) < 2 {
				return encRing{}, fmt.Errorf("%w: inline segment has %d points", embedbin.ErrMalformed, len(points))
			}
			if len(run) > 0 {
				if !embedbin.SamePoint(run[len(run)-1], points[0]) {
					return encRing{}, fmt.Errorf("%w: disconnected inline segments", embedbin.ErrMalformed)
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
				return encRing{}, fmt.Errorf("%w: missing edge %d", embedbin.ErrMalformed, s.EdgeForward)
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
				return encRing{}, fmt.Errorf("%w: missing edge %d", embedbin.ErrMalformed, s.EdgeReversed)
			}
			if !edge.empty {
				ops = append(ops, edge.index|0x80000000)
			}
		default:
			return encRing{}, fmt.Errorf("%w: empty ring segment content", embedbin.ErrMalformed)
		}
	}
	if err := flush(); err != nil {
		return encRing{}, err
	}
	if len(ops) == 0 || len(ops) > math.MaxUint16 {
		return encRing{}, fmt.Errorf("%w: ring op count %d", embedbin.ErrMalformed, len(ops))
	}
	r := encRing{ops: ops, BBox: embedbin.EmptyBBox()}
	if uint64(len(e.ops)) > math.MaxUint32 {
		return encRing{}, fmt.Errorf("%w: RINGOPS index capacity", embedbin.ErrMalformed)
	}
	r.opFirst = uint32(len(e.ops))
	var points uint64
	for i, word := range ops {
		g := &e.groups[word&0x7fffffff]
		r.BBox.Union(g.BBox)
		points += uint64(len(g.points))
		next := ops[(i+1)%len(ops)]
		entry, exit := groupEndpoints(g, word>>31 != 0)
		ng := &e.groups[next&0x7fffffff]
		nextEntry, _ := groupEndpoints(ng, next>>31 != 0)
		_ = entry
		if !embedbin.SamePoint(exit, nextEntry) {
			return encRing{}, fmt.Errorf("%w: disconnected cyclic ring junction", embedbin.ErrMalformed)
		}
	}
	points -= uint64(len(ops))
	if points < 3 || points > math.MaxUint32 {
		return encRing{}, fmt.Errorf("%w: ring open point count %d", embedbin.ErrMalformed, points)
	}
	r.PointCount = uint32(points)
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
		return encPoly{}, fmt.Errorf("%w: nil polygon", embedbin.ErrMalformed)
	}
	first := len(e.rings)
	if uint64(first) > math.MaxUint32 {
		return encPoly{}, fmt.Errorf("%w: RINGDIR index capacity", embedbin.ErrMalformed)
	}
	ext, err := e.addRing(poly.Exterior)
	if err != nil {
		return encPoly{}, err
	}
	for i, hole := range poly.Holes {
		if hole == nil || len(hole.Holes) != 0 {
			return encPoly{}, fmt.Errorf("%w: nested or nil hole %d", embedbin.ErrMalformed, i)
		}
		if _, err := e.addRing(hole.Exterior); err != nil {
			return encPoly{}, fmt.Errorf("hole %d: %w", i, err)
		}
	}
	count, err := embedbin.CheckedU16("polygon ring count", len(e.rings)-first)
	if err != nil {
		return encPoly{}, err
	}
	p := encPoly{ringFirst: uint32(first), ringCount: count, BBox: ext.BBox}
	e.polys = append(e.polys, p)
	return p, nil
}

func encodeGrid(grid *pb.GridIndex, tzCount int) ([]byte, error) {
	if grid == nil || len(grid.Cells) == 0 {
		return nil, fmt.Errorf("grid: %w: empty", embedbin.ErrMalformed)
	}
	type key struct{ lng, lat int }
	cells := make(map[key][]uint32, len(grid.Cells))
	minLng, maxLng := math.MaxInt, math.MinInt
	minLat, maxLat := math.MaxInt, math.MinInt
	for _, cell := range grid.Cells {
		if cell == nil {
			return nil, fmt.Errorf("grid: %w: nil cell", embedbin.ErrMalformed)
		}
		k := key{int(cell.Lng), int(cell.Lat)}
		if k.lng < -181 || k.lng > 181 || k.lat < -91 || k.lat > 91 {
			return nil, fmt.Errorf("grid: %w: key outside domain", embedbin.ErrMalformed)
		}
		if _, exists := cells[k]; exists {
			return nil, fmt.Errorf("grid: %w: duplicate cell", embedbin.ErrMalformed)
		}
		if len(cell.TzIndices) > 15 {
			return nil, fmt.Errorf("grid: %w: candidate count %d", embedbin.ErrMalformed, len(cell.TzIndices))
		}
		var prev uint32
		for i, idx := range cell.TzIndices {
			if idx >= uint32(tzCount) || (i > 0 && idx <= prev) {
				return nil, fmt.Errorf("grid: %w: invalid candidate order or index", embedbin.ErrMalformed)
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
		return nil, fmt.Errorf("grid: %w: invalid extent", embedbin.ErrMalformed)
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
					return nil, fmt.Errorf("grid: %w: candidate pool capacity", embedbin.ErrMalformed)
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

// buildNamesSection serializes the NAMES section.
func buildNamesSection(names []string) ([]byte, error) {
	nameBlobLen := 0
	for _, name := range names {
		if uint64(nameBlobLen)+uint64(len(name)) > math.MaxUint32 {
			return nil, fmt.Errorf("NAMES: %w: blob capacity", embedbin.ErrMalformed)
		}
		nameBlobLen += len(name)
	}
	nameSectionLen := uint64(4) + 4*uint64(len(names)+1) + uint64(nameBlobLen)
	if nameSectionLen > math.MaxUint32 {
		return nil, fmt.Errorf("NAMES: %w: section capacity", embedbin.ErrMalformed)
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
	return nameSec, nil
}

// buildTZSection serializes TZDIR; the record layout is shared by both profiles.
func (e *encoder) buildTZSection() []byte {
	tzSec := make([]byte, int(embedbin.TZRecordLen)*len(e.tzs))
	for i, t := range e.tzs {
		o := i * int(embedbin.TZRecordLen)
		binary.LittleEndian.PutUint32(tzSec[o:], t.polyFirst)
		binary.LittleEndian.PutUint16(tzSec[o+4:], t.polyCount)
		embedbin.PutBBox(tzSec, o+8, t.BBox)
	}
	return tzSec
}

// buildPolySection serializes POLYDIR; ring_first indexes RINGDIR in the E
// profile and FLATRINGDIR in the M profile, with identical record content.
func (e *encoder) buildPolySection() []byte {
	polySec := make([]byte, int(embedbin.PolyRecordLen)*len(e.polys))
	for i, p := range e.polys {
		o := i * int(embedbin.PolyRecordLen)
		binary.LittleEndian.PutUint32(polySec[o:], p.ringFirst)
		binary.LittleEndian.PutUint16(polySec[o+4:], p.ringCount)
		embedbin.PutBBox(polySec, o+8, p.BBox)
	}
	return polySec
}

func (e *encoder) serialize(names []string, version string, target uint32, allowShortcut bool, grid, fuzzy []byte) ([]byte, error) {
	nameSec, err := buildNamesSection(names)
	if err != nil {
		return nil, err
	}
	tzSec := e.buildTZSection()
	polySec := e.buildPolySection()
	ringSec := make([]byte, int(embedbin.RingRecordLen)*len(e.rings))
	for i, r := range e.rings {
		o := i * int(embedbin.RingRecordLen)
		binary.LittleEndian.PutUint32(ringSec[o:], r.opFirst)
		binary.LittleEndian.PutUint32(ringSec[o+4:], r.PointCount)
		binary.LittleEndian.PutUint16(ringSec[o+8:], uint16(len(r.ops)))
		embedbin.PutBBox(ringSec, o+12, r.BBox)
	}
	opSec := make([]byte, 4*len(e.ops))
	for i, op := range e.ops {
		binary.LittleEndian.PutUint32(opSec[i*4:], op)
	}

	var chunks []encChunk
	pointsSec := make([]byte, 0)
	groupSec := make([]byte, int(embedbin.GroupRecordLen)*len(e.groups))
	for i := range e.groups {
		g := &e.groups[i]
		o := i * int(embedbin.GroupRecordLen)
		if uint64(len(chunks)) > math.MaxUint32 {
			return nil, fmt.Errorf("CHUNKDIR: %w: index capacity", embedbin.ErrMalformed)
		}
		binary.LittleEndian.PutUint32(groupSec[o:], uint32(len(chunks)))
		binary.LittleEndian.PutUint32(groupSec[o+4:], uint32(len(g.points)))
		binary.LittleEndian.PutUint16(groupSec[o+8:], uint16(len(g.chunks)))
		binary.LittleEndian.PutUint32(groupSec[o+12:], uint32(g.points[0].X))
		binary.LittleEndian.PutUint32(groupSec[o+16:], uint32(g.points[0].Y))
		last := g.points[len(g.points)-1]
		binary.LittleEndian.PutUint32(groupSec[o+20:], uint32(last.X))
		binary.LittleEndian.PutUint32(groupSec[o+24:], uint32(last.Y))
		embedbin.PutBBox(groupSec, o+28, g.BBox)
		for j := range g.chunks {
			c := g.chunks[j]
			if uint64(len(pointsSec)) > math.MaxUint32 {
				return nil, fmt.Errorf("POINTS: %w: offset capacity", embedbin.ErrMalformed)
			}
			c.Off = uint32(len(pointsSec))
			pointsSec = append(pointsSec, embedbin.EncodePoints(c.points)...)
			chunks = append(chunks, c)
		}
	}
	chunkSec := make([]byte, int(embedbin.ChunkRecordLen)*len(chunks))
	for i, c := range chunks {
		o := i * int(embedbin.ChunkRecordLen)
		binary.LittleEndian.PutUint32(chunkSec[o:], c.Off)
		binary.LittleEndian.PutUint16(chunkSec[o+4:], uint16(len(c.points)))
		embedbin.PutBBox(chunkSec, o+8, c.BBox)
	}

	sections := []embedbin.OutSection{
		{Typ: embedbin.SectionNames, Data: nameSec, Align: 4}, {Typ: embedbin.SectionTZDir, Data: tzSec, Align: 4}, {Typ: embedbin.SectionPolyDir, Data: polySec, Align: 4},
		{Typ: embedbin.SectionRingDir, Data: ringSec, Align: 4}, {Typ: embedbin.SectionRingOps, Data: opSec, Align: 4}, {Typ: embedbin.SectionGroupDir, Data: groupSec, Align: 4},
		{Typ: embedbin.SectionChunkDir, Data: chunkSec, Align: 4}, {Typ: embedbin.SectionGrid, Data: grid, Align: 4},
	}
	if fuzzy != nil {
		// FUZZY sits before POINTS (metadata-first order) and is 8-byte
		// aligned so its keys array is castable on aligned targets.
		sections = append(sections, embedbin.OutSection{Typ: embedbin.SectionFuzzy, Data: fuzzy, Align: 8})
	}
	sections = append(sections, embedbin.OutSection{Typ: embedbin.SectionPoints, Data: pointsSec, Align: 4})
	flags := embedbin.FlagGrid
	if !allowShortcut {
		flags |= embedbin.FlagNoShortcut
	}
	return embedbin.AssembleFile(embedbin.ProfileE, flags, target, len(names), version, sections)
}

// ringPoints materializes one ring's open point run from the in-memory
// groups/ops, the encoder-side twin of embedbin.Reader.expandRing (spec rev 1 §5.1):
// every op after the first drops its duplicated junction entry point and the
// final closing point is dropped.
func (e *encoder) ringPoints(r *encRing) ([]geom.I32Point, error) {
	pts := make([]geom.I32Point, 0, uint64(r.PointCount)+1)
	for k, word := range r.ops {
		g := e.groups[word&0x7fffffff].points
		reversed := word>>31 != 0
		skip := k > 0
		if reversed {
			for i := len(g) - 1; i >= 0; i-- {
				if skip && i == len(g)-1 {
					continue
				}
				pts = append(pts, g[i])
			}
		} else {
			if skip {
				pts = append(pts, g[1:]...)
			} else {
				pts = append(pts, g...)
			}
		}
	}
	if uint64(len(pts)) != uint64(r.PointCount)+1 || !embedbin.SamePoint(pts[len(pts)-1], pts[0]) {
		return nil, fmt.Errorf("ring expansion: %w: point count or closing junction", embedbin.ErrMalformed)
	}
	return pts[: len(pts)-1 : len(pts)-1], nil
}

// serializeM writes the M-profile (.tzm) layout: FLATRINGDIR records over one
// contiguous FLATPOINTS pair array, with NAMES/TZDIR/POLYDIR/GRID/FUZZY
// byte-identical to the E profile. The chunk_target header field is 0 and the
// shortcut flag is not set: both govern only the E in-place reader.
func (e *encoder) serializeM(names []string, version string, grid, fuzzy []byte) ([]byte, error) {
	nameSec, err := buildNamesSection(names)
	if err != nil {
		return nil, err
	}
	tzSec := e.buildTZSection()
	polySec := e.buildPolySection()

	flatRingSec := make([]byte, int(embedbin.FlatRingRecordLen)*len(e.rings))
	var flatPoints []byte
	var pairTotal uint64
	for i := range e.rings {
		r := &e.rings[i]
		pts, err := e.ringPoints(r)
		if err != nil {
			return nil, err
		}
		first, err := embedbin.CheckedU32("FLATRINGDIR point_first", pairTotal)
		if err != nil {
			return nil, err
		}
		o := i * int(embedbin.FlatRingRecordLen)
		binary.LittleEndian.PutUint32(flatRingSec[o:], first)
		binary.LittleEndian.PutUint32(flatRingSec[o+4:], uint32(len(pts)))
		embedbin.PutBBox(flatRingSec, o+8, r.BBox)
		for _, p := range pts {
			var pair [8]byte
			binary.LittleEndian.PutUint32(pair[0:], uint32(p.X))
			binary.LittleEndian.PutUint32(pair[4:], uint32(p.Y))
			flatPoints = append(flatPoints, pair[:]...)
		}
		pairTotal += uint64(len(pts))
	}
	if _, err := embedbin.CheckedU32("FLATPOINTS pair count", pairTotal); err != nil {
		return nil, err
	}

	sections := []embedbin.OutSection{
		{Typ: embedbin.SectionNames, Data: nameSec, Align: 4}, {Typ: embedbin.SectionTZDir, Data: tzSec, Align: 4}, {Typ: embedbin.SectionPolyDir, Data: polySec, Align: 4},
		{Typ: embedbin.SectionFlatRingDir, Data: flatRingSec, Align: 4}, {Typ: embedbin.SectionGrid, Data: grid, Align: 4},
	}
	if fuzzy != nil {
		sections = append(sections, embedbin.OutSection{Typ: embedbin.SectionFuzzy, Data: fuzzy, Align: 8})
	}
	// FLATPOINTS is 8-byte aligned so a reader may alias it as a point slice
	// on aligned little-endian targets (spec rev 1 §6.2).
	sections = append(sections, embedbin.OutSection{Typ: embedbin.SectionFlatPoints, Data: flatPoints, Align: 8})
	return embedbin.AssembleFile(embedbin.ProfileM, embedbin.FlagGrid, 0, len(names), version, sections)
}
