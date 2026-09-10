package embedbin

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// FUZZY section (type 10) layout, spec rev 1 §4:
//
//	u8  idx_zoom
//	u8  agg_zoom               // agg_zoom ≤ idx_zoom
//	u16 reserved (0)
//	u32 tile_count
//	u32 multi_group_count
//	u32 multi_value_count
//	u64 keys[tile_count]       // packed geom.TileID, strictly ascending
//	u16 values[tile_count]
//	u16 multi_dir[multi_group_count × 2]   // (first, count) pairs
//	u16 multi_values[multi_value_count]    // NAMES indices
//	(zero padding to 4-byte alignment)
//
// The section itself is 8-byte aligned so the keys array (section offset 16)
// stays castable on aligned targets; explicit little-endian loads remain the
// normative access method.

type fuzzyInfo struct {
	present         bool
	idxZoom         uint8
	aggZoom         uint8
	tileCount       uint32
	multiGroupCount uint32
	multiValueCount uint32
	keysOff         uint64
	valuesOff       uint64
	multiDirOff     uint64
	multiValuesOff  uint64
	maxGroupLen     uint32
}

// validateFuzzy checks the FUZZY section's structure at open time: exact
// length for the stored counts, ordered keys, and resolvable values. Semantic
// parity with the source preindex is the build pipeline's job (embedcompare).
func (r *Reader) validateFuzzy() error {
	s := r.sections[sectionFuzzy]
	if s.Off%8 != 0 {
		return fmt.Errorf("%w: FUZZY section alignment", ErrMalformed)
	}
	if uint64(s.Len) < fuzzyHeaderLen {
		return fmt.Errorf("%w: FUZZY length", ErrMalformed)
	}
	raw, err := r.readSmall(uint64(s.Off), int(fuzzyHeaderLen))
	if err != nil {
		return err
	}
	f := fuzzyInfo{
		present:         true,
		idxZoom:         raw[0],
		aggZoom:         raw[1],
		tileCount:       binary.LittleEndian.Uint32(raw[4:]),
		multiGroupCount: binary.LittleEndian.Uint32(raw[8:]),
		multiValueCount: binary.LittleEndian.Uint32(raw[12:]),
		maxGroupLen:     1,
	}
	if raw[2] != 0 || raw[3] != 0 || f.aggZoom > f.idxZoom || f.idxZoom > 28 || f.tileCount == 0 {
		return fmt.Errorf("%w: FUZZY header", ErrMalformed)
	}
	size := fuzzyHeaderLen + 8*uint64(f.tileCount) + 2*uint64(f.tileCount) +
		4*uint64(f.multiGroupCount) + 2*uint64(f.multiValueCount)
	if Align4(size) != uint64(s.Len) {
		return fmt.Errorf("%w: FUZZY section size", ErrMalformed)
	}
	f.keysOff = uint64(s.Off) + fuzzyHeaderLen
	f.valuesOff = f.keysOff + 8*uint64(f.tileCount)
	f.multiDirOff = f.valuesOff + 2*uint64(f.tileCount)
	f.multiValuesOff = f.multiDirOff + 4*uint64(f.multiGroupCount)
	for pad := size; pad < uint64(s.Len); pad++ {
		b, err := r.byteAt(uint64(s.Off) + pad)
		if err != nil {
			return err
		}
		if b != 0 {
			return fmt.Errorf("%w: FUZZY padding", ErrMalformed)
		}
	}
	r.fuzzy = f

	var prev uint64
	for i := uint32(0); i < f.tileCount; i++ {
		key, err := r.fuzzyKeyAt(i)
		if err != nil {
			return err
		}
		if i > 0 && key <= prev {
			return fmt.Errorf("%w: FUZZY keys not strictly ascending", ErrMalformed)
		}
		prev = key
		if z := uint8(key >> 56); z < f.aggZoom || z > f.idxZoom {
			return fmt.Errorf("%w: FUZZY key zoom", ErrMalformed)
		}
		value, err := r.fuzzyU16At(f.valuesOff + 2*uint64(i))
		if err != nil {
			return err
		}
		if value&fuzzyMulti == 0 {
			if uint32(value) >= r.tzCount {
				return fmt.Errorf("%w: FUZZY value index", ErrMalformed)
			}
			continue
		}
		if uint32(value&^fuzzyMulti) >= f.multiGroupCount {
			return fmt.Errorf("%w: FUZZY multi group index", ErrMalformed)
		}
	}
	for g := uint32(0); g < f.multiGroupCount; g++ {
		first, count, err := r.fuzzyGroupAt(g)
		if err != nil {
			return err
		}
		if count == 0 || uint32(first)+uint32(count) > f.multiValueCount {
			return fmt.Errorf("%w: FUZZY multi group range", ErrMalformed)
		}
		if uint32(count) > f.maxGroupLen {
			f.maxGroupLen = uint32(count)
		}
	}
	for i := uint32(0); i < f.multiValueCount; i++ {
		v, err := r.fuzzyU16At(f.multiValuesOff + 2*uint64(i))
		if err != nil {
			return err
		}
		if uint32(v) >= r.tzCount {
			return fmt.Errorf("%w: FUZZY multi value index", ErrMalformed)
		}
	}
	r.fuzzy = f
	return nil
}

// The fuzzy accessors read via r.data directly when byte-backed: the offsets
// were bounds-checked against the section at open time, so that path stays
// lock-free and allocation-free. On the io.ReaderAt backend a stack buffer
// passed through the interface would escape and allocate on every access, so
// those reads go through the shared decode workspace instead — callers must
// hold r.mu (the exported Fuzzy* entry points take it; open-time validation
// runs single-threaded), which matches the backend's documented
// queries-serialize-internally behavior.

func (r *Reader) fuzzyKeyAt(i uint32) (uint64, error) {
	off := r.fuzzy.keysOff + 8*uint64(i)
	if r.data != nil {
		return binary.LittleEndian.Uint64(r.data[off:]), nil
	}
	raw, err := r.readSmall(off, 8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(raw), nil
}

func (r *Reader) fuzzyU16At(off uint64) (uint16, error) {
	if r.data != nil {
		return binary.LittleEndian.Uint16(r.data[off:]), nil
	}
	raw, err := r.readSmall(off, 2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(raw), nil
}

func (r *Reader) fuzzyGroupAt(g uint32) (first, count uint16, err error) {
	off := r.fuzzy.multiDirOff + 4*uint64(g)
	if r.data != nil {
		return binary.LittleEndian.Uint16(r.data[off:]), binary.LittleEndian.Uint16(r.data[off+2:]), nil
	}
	raw, err := r.readSmall(off, 4)
	if err != nil {
		return 0, 0, err
	}
	return binary.LittleEndian.Uint16(raw), binary.LittleEndian.Uint16(raw[2:]), nil
}

// fuzzySearch binary-searches the sorted key array for target and returns its
// position. Because zoom occupies the key's high bits, a per-zoom probe is a
// single search.
func (r *Reader) fuzzySearch(target uint64) (uint32, bool, error) {
	lo, hi := uint32(0), r.fuzzy.tileCount
	for lo < hi {
		mid := lo + (hi-lo)/2
		key, err := r.fuzzyKeyAt(mid)
		if err != nil {
			return 0, false, err
		}
		if key < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < r.fuzzy.tileCount {
		key, err := r.fuzzyKeyAt(lo)
		if err != nil {
			return 0, false, err
		}
		if key == target {
			return lo, true, nil
		}
	}
	return 0, false, nil
}

// HasFuzzy reports whether the file carries a FUZZY section.
func (r *Reader) HasFuzzy() bool { return r.fuzzy.present }

// FuzzyLookupBufferSize returns a sufficient caller-buffer capacity for
// FuzzyLookupAppend: the largest multi-tile group in the section.
func (r *Reader) FuzzyLookupBufferSize() int { return int(r.fuzzy.maxGroupLen) }

// fuzzyProbe walks zoom levels coarsest-first and returns the first hit's
// value position: the tile-map lookup loop with the two hash maps replaced by
// one sorted array. Byte-backed probes are lock-free; on the ReaderAt backend
// the caller must hold r.mu (see the accessor note above).
func (r *Reader) fuzzyProbe(lng, lat float64) (uint16, bool, error) {
	if !r.fuzzy.present {
		return 0, false, ErrNoFuzzy
	}
	if math.IsNaN(lng) || math.IsNaN(lat) || math.IsInf(lng, 0) || math.IsInf(lat, 0) ||
		lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return 0, false, nil
	}
	tile := geom.NewTileID(lng, lat, uint(r.fuzzy.idxZoom))
	for z := r.fuzzy.aggZoom; z <= r.fuzzy.idxZoom; z++ {
		key := uint64(tile.Shift(r.fuzzy.idxZoom - z))
		pos, ok, err := r.fuzzySearch(key)
		if err != nil {
			return 0, false, err
		}
		if ok {
			value, err := r.fuzzyU16At(r.fuzzy.valuesOff + 2*uint64(pos))
			return value, true, err
		}
	}
	return 0, false, nil
}

// FuzzyLookup returns the FUZZY tile match for (lng, lat), if any. Multi-name
// tiles resolve to the group's first entry (first-listed wins), matching
// the FUZZY fast path inside the composed finders.
func (r *Reader) FuzzyLookup(lng, lat float64) (int32, bool, error) {
	if r.data == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	value, ok, err := r.fuzzyProbe(lng, lat)
	if err != nil || !ok {
		return 0, false, err
	}
	if value&fuzzyMulti == 0 {
		return int32(value), true, nil
	}
	first, _, err := r.fuzzyGroupAt(uint32(value &^ fuzzyMulti))
	if err != nil {
		return 0, false, err
	}
	v, err := r.fuzzyU16At(r.fuzzy.multiValuesOff + 2*uint64(first))
	if err != nil {
		return 0, false, err
	}
	return int32(v), true, nil
}

// FuzzyLookupAppend appends the matched tile's timezone indices to dst in
// stored order (the source preindex key order) and returns the extended
// slice; dst is returned unchanged when no tile matches.
func (r *Reader) FuzzyLookupAppend(dst []int32, lng, lat float64) ([]int32, error) {
	if r.data == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	value, ok, err := r.fuzzyProbe(lng, lat)
	if err != nil || !ok {
		return dst, err
	}
	if value&fuzzyMulti == 0 {
		return append(dst, int32(value)), nil
	}
	first, count, err := r.fuzzyGroupAt(uint32(value &^ fuzzyMulti))
	if err != nil {
		return dst, err
	}
	for i := uint32(0); i < uint32(count); i++ {
		v, err := r.fuzzyU16At(r.fuzzy.multiValuesOff + 2*uint64(uint32(first)+i))
		if err != nil {
			return dst, err
		}
		dst = append(dst, int32(v))
	}
	return dst, nil
}

// FuzzyZooms returns the FUZZY section's (idx_zoom, agg_zoom).
func (r *Reader) FuzzyZooms() (idxZoom, aggZoom int) {
	return int(r.fuzzy.idxZoom), int(r.fuzzy.aggZoom)
}

// FuzzyMaps materializes the FUZZY section into the two hash maps the in-RAM
// fuzzy finder queries (single-timezone tiles and boundary tiles), one pass
// over the sorted key array. Values are NAMES indices; multi groups keep
// their stored order. Costs ~2.4 MB heap on the bundled preindex but restores
// hash-lookup query latency versus the in-place binary search.
func (r *Reader) FuzzyMaps() (single map[geom.TileID]uint16, multi map[geom.TileID][]uint16, err error) {
	if !r.fuzzy.present {
		return nil, nil, ErrNoFuzzy
	}
	if r.data == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	single = make(map[geom.TileID]uint16, r.fuzzy.tileCount)
	multi = make(map[geom.TileID][]uint16)
	for i := uint32(0); i < r.fuzzy.tileCount; i++ {
		key, err := r.fuzzyKeyAt(i)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.fuzzyU16At(r.fuzzy.valuesOff + 2*uint64(i))
		if err != nil {
			return nil, nil, err
		}
		if value&fuzzyMulti == 0 {
			single[geom.TileID(key)] = value
			continue
		}
		first, count, err := r.fuzzyGroupAt(uint32(value &^ fuzzyMulti))
		if err != nil {
			return nil, nil, err
		}
		group := make([]uint16, count)
		for j := uint32(0); j < uint32(count); j++ {
			v, err := r.fuzzyU16At(r.fuzzy.multiValuesOff + 2*uint64(uint32(first)+j))
			if err != nil {
				return nil, nil, err
			}
			group[j] = v
		}
		multi[geom.TileID(key)] = group
	}
	return single, multi, nil
}
