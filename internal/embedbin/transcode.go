package embedbin

import (
	"encoding/binary"
	"fmt"

	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// TranscodeM converts an E-profile (.tzb) file into the M-profile (.tzm)
// layout without any pipeline dependency: ring geometry is materialized by the
// §5.1 expansion and written as FLATRINGDIR/FLATPOINTS, while the
// profile-shared sections (NAMES, TZDIR, POLYDIR, GRID, FUZZY) are copied
// byte-for-byte. The output is identical to EncodeM over the E file's source
// dataset, so a consumer can ship the compact .tzb and produce the memory
// image locally (for example into an mmap-backed cache) instead of
// distributing the much larger .tzm.
func (r *Reader) TranscodeM() ([]byte, error) {
	if r.profile != profileE {
		return nil, ErrProfile
	}
	v := r.view()
	defer r.release(v)
	r = v

	groups := make([][]geom.I32Point, r.groupCount)
	for i := uint32(0); i < r.groupCount; i++ {
		points, err := r.decodeGroupAt(i)
		if err != nil {
			return nil, fmt.Errorf("transcode: %w", err)
		}
		groups[i] = points
	}
	group := func(i uint32) ([]geom.I32Point, error) { return groups[i], nil }

	flatRingSec := make([]byte, int(flatRingRecordLen)*int(r.ringCount))
	var flatPoints []byte
	var pairTotal uint64
	for i := uint32(0); i < r.ringCount; i++ {
		ring, err := r.RingAt(i)
		if err != nil {
			return nil, err
		}
		pts, err := r.expandRing(i, group)
		if err != nil {
			return nil, err
		}
		first, err := CheckedU32("FLATRINGDIR point_first", pairTotal)
		if err != nil {
			return nil, err
		}
		o := int(i) * int(flatRingRecordLen)
		binary.LittleEndian.PutUint32(flatRingSec[o:], first)
		binary.LittleEndian.PutUint32(flatRingSec[o+4:], uint32(len(pts)))
		PutBBox(flatRingSec, o+8, ring.Box)
		for _, p := range pts {
			var pair [8]byte
			binary.LittleEndian.PutUint32(pair[0:], uint32(p.X))
			binary.LittleEndian.PutUint32(pair[4:], uint32(p.Y))
			flatPoints = append(flatPoints, pair[:]...)
		}
		pairTotal += uint64(len(pts))
	}
	if _, err := CheckedU32("FLATPOINTS pair count", pairTotal); err != nil {
		return nil, err
	}

	copySection := func(typ uint32) ([]byte, error) {
		b, err := r.sectionBytes(typ)
		if err != nil {
			return nil, err
		}
		// sectionBytes aliases byte-backed readers; AssembleFile only reads
		// the slice, so no copy is needed here.
		return b, nil
	}
	nameSec, err := copySection(sectionNames)
	if err != nil {
		return nil, err
	}
	tzSec, err := copySection(sectionTZDir)
	if err != nil {
		return nil, err
	}
	polySec, err := copySection(sectionPolyDir)
	if err != nil {
		return nil, err
	}
	sections := []OutSection{
		{sectionNames, nameSec, 4}, {sectionTZDir, tzSec, 4}, {sectionPolyDir, polySec, 4},
		{sectionFlatRingDir, flatRingSec, 4},
	}
	flags := uint32(0)
	if r.grid.present {
		gridSec, err := copySection(sectionGrid)
		if err != nil {
			return nil, err
		}
		sections = append(sections, OutSection{sectionGrid, gridSec, 4})
		flags |= flagGrid
	}
	if r.fuzzy.present {
		fuzzySec, err := copySection(sectionFuzzy)
		if err != nil {
			return nil, err
		}
		sections = append(sections, OutSection{sectionFuzzy, fuzzySec, 8})
	}
	sections = append(sections, OutSection{sectionFlatPoints, flatPoints, 8})
	return AssembleFile(profileM, flags, 0, int(r.tzCount), r.version, sections)
}
