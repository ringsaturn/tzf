package embedbin

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"sync"
	"unicode/utf8"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

type section struct {
	Off uint32
	Len uint32
}

type readWorkspace struct {
	record     [64]byte
	cache      [512]byte
	cacheOff   uint64
	cacheLen   int
	cacheValid bool
}

// Reader performs direct lookups over a validated .tzb or .tzm source.
type Reader struct {
	data        []byte
	readerAt    io.ReaderAt
	size        uint64
	profile     byte
	flags       uint32
	tzCount     uint32
	chunkTarget uint32
	version     string
	sections    [sectionSlots]section
	polyCount   uint32
	// ringCount counts RINGDIR records in the E profile and FLATRINGDIR
	// records in the M profile; POLYDIR bounds-checks against it either way.
	ringCount uint32
	opCount   uint32
	// flatPairCount is the FLATPOINTS pair count (M profile only).
	flatPairCount uint32
	groupCount    uint32
	chunkCount    uint32
	grid          gridInfo
	fuzzy         fuzzyInfo
	mu            sync.Mutex
	work          readWorkspace
}

type gridInfo struct {
	present    bool
	lngMin     int16
	latMin     int16
	lngCells   uint16
	latCells   uint16
	candCount  uint32
	cellCount  uint32
	candidates uint64
}

type TZRecord struct {
	First uint32
	Count uint16
	Box   BBox
}

type PolyRecord struct {
	First uint32
	Count uint16
	Box   BBox
}

type RingRecord struct {
	First      uint32
	PointCount uint32
	Count      uint16
	Box        BBox
}

type GroupRecord struct {
	First      uint32
	PointCount uint32
	Count      uint16
	Entry      geom.I32Point
	Exit       geom.I32Point
	Box        BBox
}

type ChunkRecord struct {
	Off   uint32
	Count uint16
	Box   BBox
}

// Open validates and opens a byte-backed .tzb file.
func Open(data []byte) (*Reader, error) {
	r := &Reader{data: data, size: uint64(len(data))}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

// OpenReaderAt validates and opens size bytes from source.
func OpenReaderAt(source io.ReaderAt, size int64) (*Reader, error) {
	if source == nil || size < 0 {
		return nil, fmt.Errorf("%w: invalid ReaderAt source", ErrMalformed)
	}
	r := &Reader{readerAt: source, size: uint64(size)}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reader) open() error {
	if r.size < headerSize+footerSize || r.size > math.MaxUint32 {
		return fmt.Errorf("%w: invalid file size", ErrMalformed)
	}
	var h [headerSize]byte
	if err := r.readRaw(h[:], 0); err != nil {
		return err
	}
	if string(h[0:4]) != "TZFB" || h[4] != formatMajor {
		return fmt.Errorf("%w: magic or format major", ErrMalformed)
	}
	r.profile = h[profileOffset]
	if r.profile != profileE && r.profile != profileM {
		return fmt.Errorf("%w: unsupported profile %d", ErrMalformed, r.profile)
	}
	if binary.LittleEndian.Uint16(h[6:]) != headerSize ||
		binary.LittleEndian.Uint32(h[12:]) != coordScale ||
		uint64(binary.LittleEndian.Uint32(h[16:])) != r.size {
		return fmt.Errorf("%w: header fields", ErrMalformed)
	}
	r.flags = binary.LittleEndian.Uint32(h[8:])
	r.tzCount = binary.LittleEndian.Uint32(h[40:])
	r.chunkTarget = binary.LittleEndian.Uint32(h[44:])
	if r.tzCount == 0 || r.tzCount > math.MaxUint16 {
		return fmt.Errorf("%w: header counts", ErrMalformed)
	}
	// chunk_target is an E-profile field; M files carry 0 there.
	if r.profile == profileE && (r.chunkTarget == 0 || r.chunkTarget > math.MaxUint16) ||
		r.profile == profileM && r.chunkTarget != 0 {
		return fmt.Errorf("%w: header chunk target", ErrMalformed)
	}
	versionEnd := 16
	for i, b := range h[24:40] {
		if b == 0 {
			versionEnd = i
			for _, tail := range h[24+i : 40] {
				if tail != 0 {
					return fmt.Errorf("%w: data version padding", ErrMalformed)
				}
			}
			break
		}
	}
	if !utf8.Valid(h[24 : 24+versionEnd]) {
		return fmt.Errorf("%w: data version UTF-8", ErrMalformed)
	}
	r.version = string(h[24 : 24+versionEnd])

	sectionCount := binary.LittleEndian.Uint32(h[20:])
	tableEnd := uint64(headerSize) + uint64(sectionCount)*sectionEntryLen
	if tableEnd > r.size-footerSize {
		return fmt.Errorf("%w: section table bounds", ErrMalformed)
	}
	if err := r.verifyCRC(); err != nil {
		return err
	}
	var seen [sectionSlots]bool
	for i := uint32(0); i < sectionCount; i++ {
		entry, err := r.sectionEntry(i)
		if err != nil {
			return err
		}
		if entry.Off%4 != 0 {
			return fmt.Errorf("%w: unaligned section", ErrMalformed)
		}
		end := uint64(entry.Off) + uint64(entry.Len)
		if uint64(entry.Off) < tableEnd || end > r.size-footerSize {
			return fmt.Errorf("%w: section bounds", ErrMalformed)
		}
		var raw [4]byte
		if err := r.readRaw(raw[:], uint64(headerSize)+uint64(i)*sectionEntryLen); err != nil {
			return err
		}
		typ := binary.LittleEndian.Uint32(raw[:])
		if typ >= sectionNames && typ < sectionSlots {
			if seen[typ] {
				return fmt.Errorf("%w: duplicate known section %d", ErrMalformed, typ)
			}
			if !profileAllowsSection(r.profile, typ) {
				return fmt.Errorf("%w: section %d not valid in profile %d", ErrMalformed, typ, r.profile)
			}
			seen[typ] = true
			r.sections[typ] = entry
		}
		for j := uint32(0); j < i; j++ {
			other, err := r.sectionEntry(j)
			if err != nil {
				return err
			}
			if rangesOverlap(entry, other) {
				return fmt.Errorf("%w: overlapping sections", ErrMalformed)
			}
		}
	}
	// Mandatory sections are per-profile (spec rev 1 §6.1).
	mandatory := []uint32{sectionNames, sectionTZDir, sectionPolyDir, sectionRingDir, sectionRingOps, sectionGroupDir, sectionChunkDir, sectionPoints}
	if r.profile == profileM {
		mandatory = []uint32{sectionNames, sectionTZDir, sectionPolyDir, sectionFlatRingDir, sectionFlatPoints}
	}
	for _, typ := range mandatory {
		if !seen[typ] {
			return fmt.Errorf("%w: missing section %d", ErrMalformed, typ)
		}
	}
	hasGrid := seen[sectionGrid]
	if hasGrid != (r.flags&flagGrid != 0) {
		return fmt.Errorf("%w: GRID flag mismatch", ErrMalformed)
	}
	if r.sections[sectionTZDir].Len != r.tzCount*tzRecordLen ||
		r.sections[sectionPolyDir].Len%polyRecordLen != 0 {
		return fmt.Errorf("%w: directory section length", ErrMalformed)
	}
	r.polyCount = r.sections[sectionPolyDir].Len / polyRecordLen
	if r.profile == profileM {
		if err := r.openFlatSections(); err != nil {
			return err
		}
	} else {
		if r.sections[sectionRingDir].Len%ringRecordLen != 0 ||
			r.sections[sectionRingOps].Len%4 != 0 ||
			r.sections[sectionGroupDir].Len%groupRecordLen != 0 ||
			r.sections[sectionChunkDir].Len%chunkRecordLen != 0 {
			return fmt.Errorf("%w: directory section length", ErrMalformed)
		}
		r.ringCount = r.sections[sectionRingDir].Len / ringRecordLen
		r.opCount = r.sections[sectionRingOps].Len / 4
		r.groupCount = r.sections[sectionGroupDir].Len / groupRecordLen
		r.chunkCount = r.sections[sectionChunkDir].Len / chunkRecordLen
		if r.opCount == 0 || r.groupCount == 0 || r.chunkCount == 0 {
			return fmt.Errorf("%w: empty directory", ErrMalformed)
		}
	}
	if r.polyCount == 0 || r.ringCount == 0 {
		return fmt.Errorf("%w: empty directory", ErrMalformed)
	}
	if err := r.validateNames(); err != nil {
		return err
	}
	if hasGrid {
		if err := r.validateGrid(); err != nil {
			return err
		}
	}
	if seen[sectionFuzzy] {
		if err := r.validateFuzzy(); err != nil {
			return err
		}
	}
	if r.profile == profileE {
		if err := r.validateChunkOffsets(); err != nil {
			return err
		}
	}
	return nil
}

// profileAllowsSection reports whether a known section type may appear in a
// file of the given profile (spec rev 1 §6.1). YSTRIPES is accepted in M
// files but not consumed by this reader (spec §6.4: readers without stripe
// loading rebuild them at open).
func profileAllowsSection(profile byte, typ uint32) bool {
	switch typ {
	case sectionFlatPoints, sectionFlatRingDir, sectionYStripes:
		return profile == profileM
	case sectionRingDir, sectionRingOps, sectionGroupDir, sectionChunkDir, sectionPoints:
		return profile == profileE
	default:
		return true
	}
}

// openFlatSections validates the M-profile FLATRINGDIR/FLATPOINTS structure:
// alignment, exact pair arithmetic, and ring ranges partitioning FLATPOINTS
// in order.
func (r *Reader) openFlatSections() error {
	points := r.sections[sectionFlatPoints]
	if points.Off%8 != 0 || points.Len%8 != 0 {
		return fmt.Errorf("%w: FLATPOINTS alignment or length", ErrMalformed)
	}
	r.flatPairCount = points.Len / 8
	if r.sections[sectionFlatRingDir].Len%flatRingRecordLen != 0 {
		return fmt.Errorf("%w: FLATRINGDIR section length", ErrMalformed)
	}
	r.ringCount = r.sections[sectionFlatRingDir].Len / flatRingRecordLen
	var next uint64
	for i := uint32(0); i < r.ringCount; i++ {
		rec, err := r.FlatRingAt(i)
		if err != nil {
			return err
		}
		if uint64(rec.First) != next {
			return fmt.Errorf("%w: FLATRINGDIR rings do not partition FLATPOINTS", ErrMalformed)
		}
		next += uint64(rec.Count)
	}
	if next != uint64(r.flatPairCount) {
		return fmt.Errorf("%w: FLATPOINTS trailing pairs", ErrMalformed)
	}
	return nil
}

type FlatRingRecord struct {
	First uint32
	Count uint32
	Box   BBox
}

// flatRingAt reads one FLATRINGDIR record (M profile).
func (r *Reader) FlatRingAt(index uint32) (FlatRingRecord, error) {
	raw, err := r.readRecord(sectionFlatRingDir, index, r.ringCount, flatRingRecordLen)
	if err != nil {
		return FlatRingRecord{}, err
	}
	v := FlatRingRecord{
		First: binary.LittleEndian.Uint32(raw),
		Count: binary.LittleEndian.Uint32(raw[4:]),
		Box:   GetBBox(raw, 8),
	}
	if v.Count < 3 || uint64(v.First)+uint64(v.Count) > uint64(r.flatPairCount) || !v.Box.inDomain() {
		return FlatRingRecord{}, fmt.Errorf("%w: FLATRINGDIR record", ErrMalformed)
	}
	return v, nil
}

func (r *Reader) verifyCRC() error {
	var footer [4]byte
	if err := r.readRaw(footer[:], r.size-footerSize); err != nil {
		return err
	}
	want := binary.LittleEndian.Uint32(footer[:])
	if r.data != nil {
		if crc32.ChecksumIEEE(r.data[:len(r.data)-4]) != want {
			return fmt.Errorf("%w: CRC32", ErrMalformed)
		}
		return nil
	}
	hash := crc32.NewIEEE()
	buf := make([]byte, 32*1024)
	for off := uint64(0); off < r.size-footerSize; {
		n := int(min(uint64(len(buf)), r.size-footerSize-off))
		if err := r.readRaw(buf[:n], off); err != nil {
			return err
		}
		_, _ = hash.Write(buf[:n])
		off += uint64(n)
	}
	if hash.Sum32() != want {
		return fmt.Errorf("%w: CRC32", ErrMalformed)
	}
	return nil
}

func (r *Reader) sectionEntry(i uint32) (section, error) {
	var raw [sectionEntryLen]byte
	if err := r.readRaw(raw[:], uint64(headerSize)+uint64(i)*sectionEntryLen); err != nil {
		return section{}, err
	}
	return section{Off: binary.LittleEndian.Uint32(raw[4:]), Len: binary.LittleEndian.Uint32(raw[8:])}, nil
}

func rangesOverlap(a, b section) bool {
	return uint64(a.Off) < uint64(b.Off)+uint64(b.Len) && uint64(b.Off) < uint64(a.Off)+uint64(a.Len)
}

func (r *Reader) readRaw(dst []byte, off uint64) error {
	if off > r.size || uint64(len(dst)) > r.size-off {
		return fmt.Errorf("%w: read bounds", ErrMalformed)
	}
	if r.data != nil {
		copy(dst, r.data[off:off+uint64(len(dst))])
		return nil
	}
	n, err := r.readerAt.ReadAt(dst, int64(off))
	if err != nil && (err != io.EOF || n != len(dst)) {
		return fmt.Errorf("%w: read: %v", ErrMalformed, err)
	}
	if n != len(dst) {
		return fmt.Errorf("%w: short read", ErrMalformed)
	}
	return nil
}

func (r *Reader) readSmall(off uint64, n int) ([]byte, error) {
	if n > len(r.work.record) {
		return nil, fmt.Errorf("%w: internal record size", ErrMalformed)
	}
	dst := r.work.record[:n]
	if err := r.readRaw(dst, off); err != nil {
		return nil, err
	}
	return dst, nil
}

func (r *Reader) validateNames() error {
	s := r.sections[sectionNames]
	prefix := uint64(4) + 4*uint64(r.tzCount+1)
	if uint64(s.Len) < prefix {
		return fmt.Errorf("%w: NAMES length", ErrMalformed)
	}
	raw, err := r.readSmall(uint64(s.Off), 4)
	if err != nil {
		return err
	}
	blobLen := binary.LittleEndian.Uint32(raw)
	if prefix+uint64(blobLen) != uint64(s.Len) {
		return fmt.Errorf("%w: NAMES blob length", ErrMalformed)
	}
	var prev uint32
	for i := uint32(0); i <= r.tzCount; i++ {
		raw, err = r.readSmall(uint64(s.Off)+4+uint64(i)*4, 4)
		if err != nil {
			return err
		}
		off := binary.LittleEndian.Uint32(raw)
		if off < prev || off > blobLen || (i == r.tzCount && off != blobLen) {
			return fmt.Errorf("%w: NAMES offsets", ErrMalformed)
		}
		if i > 0 && off == prev {
			return fmt.Errorf("%w: empty timezone name", ErrMalformed)
		}
		prev = off
	}
	for i := int32(0); i < int32(r.tzCount); i++ {
		start, end, err := r.nameBounds(i)
		if err != nil {
			return err
		}
		n := int(end - start)
		if r.data != nil {
			name := r.data[start:end]
			if !utf8.Valid(name) {
				return fmt.Errorf("%w: invalid name UTF-8", ErrMalformed)
			}
			for _, b := range name {
				if b == 0 {
					return fmt.Errorf("%w: NUL in name", ErrMalformed)
				}
			}
			continue
		}
		name := make([]byte, n)
		if err := r.readRaw(name, start); err != nil {
			return err
		}
		if !utf8.Valid(name) {
			return fmt.Errorf("%w: invalid name UTF-8", ErrMalformed)
		}
		for _, b := range name {
			if b == 0 {
				return fmt.Errorf("%w: NUL in name", ErrMalformed)
			}
		}
	}
	return nil
}

func (r *Reader) validateGrid() error {
	s := r.sections[sectionGrid]
	if s.Len < 12 {
		return fmt.Errorf("%w: GRID length", ErrMalformed)
	}
	raw, err := r.readSmall(uint64(s.Off), 12)
	if err != nil {
		return err
	}
	g := gridInfo{
		present:   true,
		lngMin:    int16(binary.LittleEndian.Uint16(raw[0:])),
		latMin:    int16(binary.LittleEndian.Uint16(raw[2:])),
		lngCells:  binary.LittleEndian.Uint16(raw[4:]),
		latCells:  binary.LittleEndian.Uint16(raw[6:]),
		candCount: binary.LittleEndian.Uint32(raw[8:]),
	}
	if g.lngCells == 0 || g.latCells == 0 || g.lngMin < -181 || g.lngMin > 180 ||
		g.latMin < -91 || g.latMin > 90 ||
		int(g.lngMin)+int(g.lngCells)-1 > 181 ||
		int(g.latMin)+int(g.latCells)-1 > 91 ||
		g.candCount >= 1<<28 {
		return fmt.Errorf("%w: GRID dimensions", ErrMalformed)
	}
	cells := uint64(g.lngCells) * uint64(g.latCells)
	expect := uint64(12) + cells*4 + uint64(g.candCount)*2
	if expect != uint64(s.Len) || cells > math.MaxUint32 {
		return fmt.Errorf("%w: GRID section size", ErrMalformed)
	}
	g.cellCount = uint32(cells)
	g.candidates = uint64(s.Off) + 12 + cells*4
	r.grid = g
	for i := uint32(0); i < g.cellCount; i++ {
		raw, err := r.readSmall(uint64(s.Off)+12+uint64(i)*4, 4)
		if err != nil {
			return err
		}
		word := binary.LittleEndian.Uint32(raw)
		count, off := word>>28, word&0x0fffffff
		if uint64(off)+uint64(count) > uint64(g.candCount) {
			return fmt.Errorf("%w: GRID candidate range", ErrMalformed)
		}
		for j := uint32(0); j < count; j++ {
			raw, err = r.readSmall(g.candidates+uint64(off+j)*2, 2)
			if err != nil {
				return err
			}
			if uint32(binary.LittleEndian.Uint16(raw)) >= r.tzCount {
				return fmt.Errorf("%w: GRID candidate index", ErrMalformed)
			}
		}
	}
	return nil
}

func (r *Reader) validateChunkOffsets() error {
	var prev uint32
	for i := uint32(0); i < r.chunkCount; i++ {
		c, err := r.ChunkAt(i)
		if err != nil {
			return err
		}
		if c.Count == 0 || c.Off >= r.sections[sectionPoints].Len || (i > 0 && c.Off <= prev) {
			return fmt.Errorf("%w: chunk offset or count", ErrMalformed)
		}
		prev = c.Off
	}
	return nil
}

func (r *Reader) readRecord(sectionType uint32, index, count, width uint32) ([]byte, error) {
	if index >= count {
		return nil, fmt.Errorf("%w: directory index", ErrMalformed)
	}
	s := r.sections[sectionType]
	return r.readSmall(uint64(s.Off)+uint64(index)*uint64(width), int(width))
}

func (r *Reader) TZAt(index uint32) (TZRecord, error) {
	raw, err := r.readRecord(sectionTZDir, index, r.tzCount, tzRecordLen)
	if err != nil {
		return TZRecord{}, err
	}
	v := TZRecord{First: binary.LittleEndian.Uint32(raw), Count: binary.LittleEndian.Uint16(raw[4:]), Box: GetBBox(raw, 8)}
	if v.Count == 0 || uint64(v.First)+uint64(v.Count) > uint64(r.polyCount) || !v.Box.inDomain() {
		return TZRecord{}, fmt.Errorf("%w: TZDIR record", ErrMalformed)
	}
	return v, nil
}

func (r *Reader) PolyAt(index uint32) (PolyRecord, error) {
	raw, err := r.readRecord(sectionPolyDir, index, r.polyCount, polyRecordLen)
	if err != nil {
		return PolyRecord{}, err
	}
	v := PolyRecord{First: binary.LittleEndian.Uint32(raw), Count: binary.LittleEndian.Uint16(raw[4:]), Box: GetBBox(raw, 8)}
	if v.Count == 0 || uint64(v.First)+uint64(v.Count) > uint64(r.ringCount) || !v.Box.inDomain() {
		return PolyRecord{}, fmt.Errorf("%w: POLYDIR record", ErrMalformed)
	}
	return v, nil
}

func (r *Reader) RingAt(index uint32) (RingRecord, error) {
	raw, err := r.readRecord(sectionRingDir, index, r.ringCount, ringRecordLen)
	if err != nil {
		return RingRecord{}, err
	}
	v := RingRecord{
		First: binary.LittleEndian.Uint32(raw), PointCount: binary.LittleEndian.Uint32(raw[4:]),
		Count: binary.LittleEndian.Uint16(raw[8:]), Box: GetBBox(raw, 12),
	}
	if v.Count == 0 || v.PointCount < 3 || uint64(v.First)+uint64(v.Count) > uint64(r.opCount) || !v.Box.inDomain() {
		return RingRecord{}, fmt.Errorf("%w: RINGDIR record", ErrMalformed)
	}
	return v, nil
}

func (r *Reader) OpAt(index uint32) (uint32, error) {
	raw, err := r.readRecord(sectionRingOps, index, r.opCount, 4)
	if err != nil {
		return 0, err
	}
	word := binary.LittleEndian.Uint32(raw)
	if word&0x7fffffff >= r.groupCount {
		return 0, fmt.Errorf("%w: RINGOPS group index", ErrMalformed)
	}
	return word, nil
}

func (r *Reader) GroupAt(index uint32) (GroupRecord, error) {
	raw, err := r.readRecord(sectionGroupDir, index, r.groupCount, groupRecordLen)
	if err != nil {
		return GroupRecord{}, err
	}
	v := GroupRecord{
		First: binary.LittleEndian.Uint32(raw), PointCount: binary.LittleEndian.Uint32(raw[4:]),
		Count: binary.LittleEndian.Uint16(raw[8:]),
		Entry: geom.I32Point{X: int32(binary.LittleEndian.Uint32(raw[12:])), Y: int32(binary.LittleEndian.Uint32(raw[16:]))},
		Exit:  geom.I32Point{X: int32(binary.LittleEndian.Uint32(raw[20:])), Y: int32(binary.LittleEndian.Uint32(raw[24:]))},
		Box:   GetBBox(raw, 28),
	}
	if v.Count == 0 || v.PointCount < 2 || uint64(v.First)+uint64(v.Count) > uint64(r.chunkCount) ||
		!v.Box.inDomain() || !PointInDomain(v.Entry) || !PointInDomain(v.Exit) {
		return GroupRecord{}, fmt.Errorf("%w: GROUPDIR record", ErrMalformed)
	}
	var total uint64
	for i := uint32(0); i < uint32(v.Count); i++ {
		c, err := r.ChunkAt(v.First + i)
		if err != nil {
			return GroupRecord{}, err
		}
		total += uint64(c.Count)
	}
	if total != uint64(v.PointCount) {
		return GroupRecord{}, fmt.Errorf("%w: group point count", ErrMalformed)
	}
	return v, nil
}

func (r *Reader) ChunkAt(index uint32) (ChunkRecord, error) {
	raw, err := r.readRecord(sectionChunkDir, index, r.chunkCount, chunkRecordLen)
	if err != nil {
		return ChunkRecord{}, err
	}
	v := ChunkRecord{Off: binary.LittleEndian.Uint32(raw), Count: binary.LittleEndian.Uint16(raw[4:]), Box: GetBBox(raw, 8)}
	if v.Count == 0 || v.Off >= r.sections[sectionPoints].Len || !v.Box.inDomain() {
		return ChunkRecord{}, fmt.Errorf("%w: CHUNKDIR record", ErrMalformed)
	}
	return v, nil
}

// DataVersion returns the dataset version stored in the header.
func (r *Reader) DataVersion() string { return r.version }

// TimezoneCount returns the number of timezone records.
func (r *Reader) TimezoneCount() int { return int(r.tzCount) }

// LookupBufferSize returns a sufficient caller-buffer capacity for LookupInto.
func (r *Reader) LookupBufferSize() int {
	if r.grid.present {
		return 15
	}
	return int(r.tzCount)
}

// ShortcutEnabled reports whether single-candidate GRID lookups may skip PIP.
func (r *Reader) ShortcutEnabled() bool { return r.flags&flagNoShortcut == 0 }

// ProfileM reports whether the file is an M-profile (.tzm) memory image.
func (r *Reader) ProfileM() bool { return r.profile == profileM }

// Lookup returns the first containing timezone index in source order.
// E profile only; M files are queried through Flat.
func (r *Reader) Lookup(lng, lat float64) (int32, bool, error) {
	if r.profile != profileE {
		return 0, false, ErrProfile
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false
	return r.lookup(lng, lat)
}

func (r *Reader) lookup(lng, lat float64) (int32, bool, error) {
	count, off, grid, err := r.candidates(lng, lat)
	if err != nil || count == 0 {
		return 0, false, err
	}
	if grid && count == 1 && r.flags&flagNoShortcut == 0 &&
		lng > -179 && lng < 179 && lat > -89 && lat < 89 {
		idx, err := r.candidateAt(off)
		return int32(idx), err == nil, err
	}
	x, y := lng*float64(coordScale), lat*float64(coordScale)
	for i := uint32(0); i < count; i++ {
		var idx uint32
		if grid {
			idx, err = r.candidateAt(off + i)
		} else {
			idx = i
		}
		if err != nil {
			return 0, false, err
		}
		ok, err := r.timezoneContains(idx, x, y)
		if err != nil {
			return 0, false, err
		}
		if ok {
			return int32(idx), true, nil
		}
	}
	return 0, false, nil
}

// LookupInto appends all matching indices to dst and sorts them by name.
// E profile only; M files are queried through Flat.
func (r *Reader) LookupInto(lng, lat float64, dst []int32) ([]int32, error) {
	if r.profile != profileE {
		return dst[:0], ErrProfile
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.work.cacheValid = false
	count, off, grid, err := r.candidates(lng, lat)
	if err != nil {
		return dst[:0], err
	}
	if cap(dst) < int(count) {
		return dst[:0], ErrBufferTooSmall
	}
	dst = dst[:0]
	x, y := lng*float64(coordScale), lat*float64(coordScale)
	for i := uint32(0); i < count; i++ {
		idx := i
		if grid {
			idx, err = r.candidateAt(off + i)
			if err != nil {
				return dst, err
			}
		}
		ok, err := r.timezoneContains(idx, x, y)
		if err != nil {
			return dst, err
		}
		if ok {
			dst = append(dst, int32(idx))
		}
	}
	for i := 1; i < len(dst); i++ {
		v := dst[i]
		j := i
		for j > 0 {
			cmp, err := r.compareNames(v, dst[j-1])
			if err != nil {
				return dst, err
			}
			if cmp >= 0 {
				break
			}
			dst[j] = dst[j-1]
			j--
		}
		dst[j] = v
	}
	return dst, nil
}

func (r *Reader) candidates(lng, lat float64) (count, off uint32, grid bool, err error) {
	if math.IsNaN(lng) || math.IsNaN(lat) || math.IsInf(lng, 0) || math.IsInf(lat, 0) ||
		lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return 0, 0, r.grid.present, nil
	}
	if !r.grid.present {
		return r.tzCount, 0, false, nil
	}
	cx := int(math.Floor(lng)) - int(r.grid.lngMin)
	cy := int(math.Floor(lat)) - int(r.grid.latMin)
	if cx < 0 || cy < 0 || cx >= int(r.grid.lngCells) || cy >= int(r.grid.latCells) {
		return 0, 0, true, nil
	}
	cell := uint64(cy)*uint64(r.grid.lngCells) + uint64(cx)
	raw, err := r.readSmall(uint64(r.sections[sectionGrid].Off)+12+cell*4, 4)
	if err != nil {
		return 0, 0, true, err
	}
	word := binary.LittleEndian.Uint32(raw)
	return word >> 28, word & 0x0fffffff, true, nil
}

func (r *Reader) candidateAt(off uint32) (uint32, error) {
	if off >= r.grid.candCount {
		return 0, fmt.Errorf("%w: candidate offset", ErrMalformed)
	}
	raw, err := r.readSmall(r.grid.candidates+uint64(off)*2, 2)
	if err != nil {
		return 0, err
	}
	idx := uint32(binary.LittleEndian.Uint16(raw))
	if idx >= r.tzCount {
		return 0, fmt.Errorf("%w: candidate index", ErrMalformed)
	}
	return idx, nil
}

func (r *Reader) timezoneContains(index uint32, x, y float64) (bool, error) {
	t, err := r.TZAt(index)
	if err != nil || !t.Box.Contains(x, y) {
		return false, err
	}
	for i := uint32(0); i < uint32(t.Count); i++ {
		p, err := r.PolyAt(t.First + i)
		if err != nil {
			return false, err
		}
		if !p.Box.Contains(x, y) {
			continue
		}
		// Exterior rings allow on-edge containment and hole rings do not,
		// matching geom.PolygonOf.ContainsPointAllowOnEdge: a border query
		// belongs to every polygon touching it, and a point on a hole's
		// boundary stays inside the polygon.
		inside, err := r.ringContains(p.First, x, y, true)
		if err != nil || !inside {
			if err != nil {
				return false, err
			}
			continue
		}
		excluded := false
		for h := uint32(1); h < uint32(p.Count); h++ {
			hr, err := r.RingAt(p.First + h)
			if err != nil {
				return false, err
			}
			if !hr.Box.Contains(x, y) {
				continue
			}
			inHole, err := r.ringContains(p.First+h, x, y, false)
			if err != nil {
				return false, err
			}
			if inHole {
				excluded = true
				break
			}
		}
		if !excluded {
			return true, nil
		}
	}
	return false, nil
}

// ringContains reports whether the ring contains (x, y). A point on any ring
// segment returns allowOnEdge, mirroring geom.ringContainsPoint.
func (r *Reader) ringContains(index uint32, x, y float64, allowOnEdge bool) (bool, error) {
	ring, err := r.RingAt(index)
	if err != nil || !ring.Box.Contains(x, y) {
		return false, err
	}
	p := geom.Point{X: x, Y: y}
	inside := false
	var firstEntry, previousExit geom.I32Point
	var sum uint64
	for i := uint32(0); i < uint32(ring.Count); i++ {
		word, err := r.OpAt(ring.First + i)
		if err != nil {
			return false, err
		}
		group, err := r.GroupAt(word & 0x7fffffff)
		if err != nil {
			return false, err
		}
		sum += uint64(group.PointCount)
		entry, exit := group.Entry, group.Exit
		if word>>31 != 0 {
			entry, exit = exit, entry
		}
		if i == 0 {
			firstEntry = entry
		} else if !SamePoint(previousExit, entry) {
			cross, on := geom.RaycastSeg(toPoint(previousExit), toPoint(entry), p)
			if on {
				return allowOnEdge, nil
			}
			if cross {
				inside = !inside
			}
		}
		previousExit = exit
		if group.Box.rayRelevant(x, y) {
			on, err := r.scanGroup(group, p, &inside)
			if err != nil {
				return false, err
			}
			if on {
				return allowOnEdge, nil
			}
		}
	}
	if sum < uint64(ring.Count) || sum-uint64(ring.Count) != uint64(ring.PointCount) {
		return false, fmt.Errorf("%w: ring point count", ErrMalformed)
	}
	if !SamePoint(previousExit, firstEntry) {
		cross, on := geom.RaycastSeg(toPoint(previousExit), toPoint(firstEntry), p)
		if on {
			return allowOnEdge, nil
		}
		if cross {
			inside = !inside
		}
	}
	return inside, nil
}

func toPoint(p geom.I32Point) geom.Point {
	return geom.Point{X: float64(p.X), Y: float64(p.Y)}
}

func (r *Reader) scanGroup(group GroupRecord, p geom.Point, inside *bool) (bool, error) {
	for i := uint32(0); i < uint32(group.Count); i++ {
		chunkIndex := group.First + i
		chunk, err := r.ChunkAt(chunkIndex)
		if err != nil {
			return false, err
		}
		if !chunk.Box.rayRelevant(p.X, p.Y) {
			continue
		}
		last, err := r.scanChunk(chunkIndex, chunk, p, inside)
		if err != nil {
			return false, err
		}
		if last.on {
			return true, nil
		}
		if i+1 < uint32(group.Count) {
			next, err := r.ChunkAt(chunkIndex + 1)
			if err != nil {
				return false, err
			}
			first, err := r.FirstChunkPoint(chunkIndex+1, next)
			if err != nil {
				return false, err
			}
			cross, on := geom.RaycastSeg(toPoint(last.point), toPoint(first), p)
			if on {
				return true, nil
			}
			if cross {
				*inside = !*inside
			}
		}
	}
	return false, nil
}

type scanResult struct {
	point geom.I32Point
	on    bool
}

func (r *Reader) scanChunk(index uint32, chunk ChunkRecord, p geom.Point, inside *bool) (scanResult, error) {
	start, end, err := r.chunkRange(index, chunk)
	if err != nil {
		return scanResult{}, err
	}
	cursor := streamCursor{r: r, pos: start, end: end}
	x, err := cursor.varint()
	if err != nil {
		return scanResult{}, err
	}
	y, err := cursor.varint()
	if err != nil {
		return scanResult{}, err
	}
	prev := geom.I32Point{X: x, Y: y}
	if !PointInDomain(prev) {
		return scanResult{}, fmt.Errorf("%w: chunk coordinate domain", ErrMalformed)
	}
	onSegment := false
	for i := uint16(1); i < chunk.Count; i++ {
		dx, err := cursor.varint()
		if err != nil {
			return scanResult{}, err
		}
		dy, err := cursor.varint()
		if err != nil {
			return scanResult{}, err
		}
		nx, err := addDelta(prev.X, dx)
		if err != nil {
			return scanResult{}, err
		}
		ny, err := addDelta(prev.Y, dy)
		if err != nil {
			return scanResult{}, err
		}
		next := geom.I32Point{X: nx, Y: ny}
		if !PointInDomain(next) {
			return scanResult{}, fmt.Errorf("%w: chunk coordinate domain", ErrMalformed)
		}
		if !onSegment {
			cross, on := geom.RaycastSeg(toPoint(prev), toPoint(next), p)
			if on {
				onSegment = true
			} else if cross {
				*inside = !*inside
			}
		}
		prev = next
	}
	if cursor.pos != end {
		return scanResult{}, fmt.Errorf("%w: trailing chunk bytes", ErrMalformed)
	}
	return scanResult{point: prev, on: onSegment}, nil
}

func (r *Reader) FirstChunkPoint(index uint32, chunk ChunkRecord) (geom.I32Point, error) {
	start, end, err := r.chunkRange(index, chunk)
	if err != nil {
		return geom.I32Point{}, err
	}
	cursor := streamCursor{r: r, pos: start, end: end}
	x, err := cursor.varint()
	if err != nil {
		return geom.I32Point{}, err
	}
	y, err := cursor.varint()
	if err != nil {
		return geom.I32Point{}, err
	}
	p := geom.I32Point{X: x, Y: y}
	if !PointInDomain(p) {
		return geom.I32Point{}, fmt.Errorf("%w: chunk coordinate domain", ErrMalformed)
	}
	return p, nil
}

func (r *Reader) chunkRange(index uint32, chunk ChunkRecord) (uint64, uint64, error) {
	start := uint64(r.sections[sectionPoints].Off) + uint64(chunk.Off)
	end := uint64(r.sections[sectionPoints].Off) + uint64(r.sections[sectionPoints].Len)
	if index+1 < r.chunkCount {
		next, err := r.ChunkAt(index + 1)
		if err != nil {
			return 0, 0, err
		}
		end = uint64(r.sections[sectionPoints].Off) + uint64(next.Off)
	}
	if start >= end {
		return 0, 0, fmt.Errorf("%w: chunk byte range", ErrMalformed)
	}
	return start, end, nil
}

type streamCursor struct {
	r   *Reader
	pos uint64
	end uint64
}

func (c *streamCursor) varint() (int32, error) {
	var u uint32
	for i := 0; i < 5; i++ {
		if c.pos >= c.end {
			return 0, fmt.Errorf("%w: truncated varint", ErrMalformed)
		}
		b, err := c.r.byteAt(c.pos)
		if err != nil {
			return 0, err
		}
		c.pos++
		if i == 4 && b&0xf0 != 0 {
			return 0, fmt.Errorf("%w: varint exceeds 32 bits", ErrMalformed)
		}
		u |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			if i > 0 && b == 0 {
				return 0, fmt.Errorf("%w: nonminimal varint", ErrMalformed)
			}
			return int32((u >> 1) ^ uint32(-int32(u&1))), nil
		}
	}
	return 0, fmt.Errorf("%w: unterminated varint", ErrMalformed)
}

func (r *Reader) byteAt(off uint64) (byte, error) {
	if off >= r.size {
		return 0, fmt.Errorf("%w: byte read", ErrMalformed)
	}
	if r.data != nil {
		return r.data[off], nil
	}
	w := &r.work
	if !w.cacheValid || off < w.cacheOff || off >= w.cacheOff+uint64(w.cacheLen) {
		w.cacheOff = off
		w.cacheLen = int(min(uint64(len(w.cache)), r.size-off))
		if w.cacheLen == 0 {
			return 0, fmt.Errorf("%w: byte read", ErrMalformed)
		}
		if err := r.readRaw(w.cache[:w.cacheLen], off); err != nil {
			return 0, err
		}
		w.cacheValid = true
	}
	return w.cache[off-w.cacheOff], nil
}

func (r *Reader) nameBounds(idx int32) (uint64, uint64, error) {
	if idx < 0 || uint32(idx) >= r.tzCount {
		return 0, 0, ErrIndex
	}
	s := r.sections[sectionNames]
	raw, err := r.readSmall(uint64(s.Off)+4+uint64(idx)*4, 8)
	if err != nil {
		return 0, 0, err
	}
	a := binary.LittleEndian.Uint32(raw)
	b := binary.LittleEndian.Uint32(raw[4:])
	base := uint64(s.Off) + 4 + uint64(r.tzCount+1)*4
	return base + uint64(a), base + uint64(b), nil
}

// Name returns a timezone name. Byte-backed readers return a zero-copy slice.
func (r *Reader) Name(idx int32) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start, end, err := r.nameBounds(idx)
	if err != nil {
		return nil, err
	}
	if r.data != nil {
		return r.data[start:end], nil
	}
	out := make([]byte, int(end-start))
	if err := r.readRaw(out, start); err != nil {
		return nil, err
	}
	return out, nil
}

// AppendName appends a timezone name into caller-provided storage.
func (r *Reader) AppendName(dst []byte, idx int32) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start, end, err := r.nameBounds(idx)
	if err != nil {
		return dst, err
	}
	n := int(end - start)
	old := len(dst)
	if n > cap(dst)-len(dst) {
		dst = append(dst, make([]byte, n)...)
	} else {
		dst = dst[:old+n]
	}
	if r.data != nil {
		copy(dst[old:], r.data[start:end])
		return dst, nil
	}
	if err := r.readRaw(dst[old:], start); err != nil {
		return dst[:old], err
	}
	return dst, nil
}

func (r *Reader) compareNames(a, b int32) (int, error) {
	as, ae, err := r.nameBounds(a)
	if err != nil {
		return 0, err
	}
	bs, be, err := r.nameBounds(b)
	if err != nil {
		return 0, err
	}
	n := min(ae-as, be-bs)
	for i := uint64(0); i < n; i++ {
		ab, err := r.byteAt(as + i)
		if err != nil {
			return 0, err
		}
		bb, err := r.byteAt(bs + i)
		if err != nil {
			return 0, err
		}
		if ab < bb {
			return -1, nil
		}
		if ab > bb {
			return 1, nil
		}
	}
	if ae-as < be-bs {
		return -1, nil
	}
	if ae-as > be-bs {
		return 1, nil
	}
	return 0, nil
}

func (r *Reader) DecodeChunkPoints(index uint32, chunk ChunkRecord) ([]geom.I32Point, error) {
	start, end, err := r.chunkRange(index, chunk)
	if err != nil {
		return nil, err
	}
	cursor := streamCursor{r: r, pos: start, end: end}
	x, err := cursor.varint()
	if err != nil {
		return nil, err
	}
	y, err := cursor.varint()
	if err != nil {
		return nil, err
	}
	points := make([]geom.I32Point, 0, chunk.Count)
	prev := geom.I32Point{X: x, Y: y}
	points = append(points, prev)
	for i := uint16(1); i < chunk.Count; i++ {
		dx, err := cursor.varint()
		if err != nil {
			return nil, err
		}
		dy, err := cursor.varint()
		if err != nil {
			return nil, err
		}
		nx, err := addDelta(prev.X, dx)
		if err != nil {
			return nil, err
		}
		ny, err := addDelta(prev.Y, dy)
		if err != nil {
			return nil, err
		}
		prev = geom.I32Point{X: nx, Y: ny}
		if !PointInDomain(prev) {
			return nil, fmt.Errorf("%w: point domain", ErrMalformed)
		}
		points = append(points, prev)
	}
	if cursor.pos != end {
		return nil, fmt.Errorf("%w: chunk termination", ErrMalformed)
	}
	return points, nil
}

func (r *Reader) NameBytesLocked(idx int32) ([]byte, error) {
	start, end, err := r.nameBounds(idx)
	if err != nil {
		return nil, err
	}
	out := make([]byte, int(end-start))
	if r.data != nil {
		copy(out, r.data[start:end])
		return out, nil
	}
	if err := r.readRaw(out, start); err != nil {
		return nil, err
	}
	return out, nil
}
