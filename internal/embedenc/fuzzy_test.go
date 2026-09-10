package embedenc

import (
	"encoding/binary"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"hash/crc32"
	"slices"
	"testing"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

func preindexKey(lng, lat float64, z int32, name string) *pb.PreindexTimezone {
	x, y, _ := geom.NewTileID(lng, lat, uint(z)).XYZ()
	return &pb.PreindexTimezone{Name: name, X: int32(x), Y: int32(y), Z: z}
}

func openFuzzyFixture(t *testing.T, pre *pb.PreindexTimezones, names ...string) ([]byte, *embedbin.Reader) {
	t.Helper()
	return openFixture(t, fixture(names...), EncodeOptions{Preindex: pre})
}

func TestFuzzyHitAtEachZoom(t *testing.T) {
	const lng, lat = 139.6917, 35.6895
	for z := int32(2); z <= 4; z++ {
		pre := &pb.PreindexTimezones{
			IdxZoom: 4, AggZoom: 2, Version: "test",
			Keys: []*pb.PreindexTimezone{preindexKey(lng, lat, z, "Etc/Test")},
		}
		_, r := openFuzzyFixture(t, pre, "Etc/Test")
		if !r.HasFuzzy() {
			t.Fatal("HasFuzzy = false")
		}
		idx, ok, err := r.FuzzyLookup(lng, lat)
		if err != nil || !ok || idx != 0 {
			t.Fatalf("z=%d FuzzyLookup = %d,%v,%v", z, idx, ok, err)
		}
		// A far-away point must miss at every zoom.
		if _, ok, err := r.FuzzyLookup(-lng, -lat); err != nil || ok {
			t.Fatalf("z=%d miss lookup = %v,%v", z, ok, err)
		}
		for _, bad := range [][2]float64{{181, 0}, {0, 91}, {-181, 0}} {
			if _, ok, err := r.FuzzyLookup(bad[0], bad[1]); err != nil || ok {
				t.Fatalf("z=%d invalid coord lookup = %v,%v", z, ok, err)
			}
		}
	}
}

func TestFuzzyAggPrecedenceAndMultiOrder(t *testing.T) {
	const lng, lat = 10.0, 10.0
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{
			// The idx-zoom tile straddles a boundary: Zed listed first, so
			// Zed wins single-result lookups and leads multi results.
			preindexKey(lng, lat, 4, "Zed/Zone"),
			preindexKey(lng, lat, 4, "Alpha/Zone"),
			// A coarser agg tile for the same point maps only to Alpha and
			// must win: the lookup walks zooms coarsest-first.
			preindexKey(lng, lat, 2, "Alpha/Zone"),
		},
	}
	_, r := openFuzzyFixture(t, pre, "Zed/Zone", "Alpha/Zone")
	idx, ok, err := r.FuzzyLookup(lng, lat)
	if err != nil || !ok || idx != 1 {
		t.Fatalf("agg precedence FuzzyLookup = %d,%v,%v (want Alpha=1)", idx, ok, err)
	}

	// Drop the agg tile: the multi idx tile is now the first hit.
	pre.Keys = pre.Keys[:2]
	_, r = openFuzzyFixture(t, pre, "Zed/Zone", "Alpha/Zone")
	idx, ok, err = r.FuzzyLookup(lng, lat)
	if err != nil || !ok || idx != 0 {
		t.Fatalf("multi first-listed FuzzyLookup = %d,%v,%v (want Zed=0)", idx, ok, err)
	}
	got, err := r.FuzzyLookupAppend(make([]int32, 0, r.FuzzyLookupBufferSize()), lng, lat)
	if err != nil || !slices.Equal(got, []int32{0, 1}) {
		t.Fatalf("multi order = %v,%v (want stored order [0 1])", got, err)
	}
}

func TestFuzzyEncoderRejectsInvalidInput(t *testing.T) {
	valid := func() *pb.PreindexTimezones {
		return &pb.PreindexTimezones{
			IdxZoom: 4, AggZoom: 2, Version: "test",
			Keys: []*pb.PreindexTimezone{preindexKey(0, 0, 4, "Etc/Test")},
		}
	}
	tests := []struct {
		name string
		pre  *pb.PreindexTimezones
	}{
		{"empty preindex", &pb.PreindexTimezones{IdxZoom: 4, AggZoom: 2, Version: "test"}},
		{"version mismatch", func() *pb.PreindexTimezones {
			v := valid()
			v.Version = "other"
			return v
		}()},
		{"absent name", func() *pb.PreindexTimezones {
			v := valid()
			v.Keys[0].Name = "No/Such"
			return v
		}()},
		{"zoom above idx", func() *pb.PreindexTimezones {
			v := valid()
			v.Keys[0].Z = 5
			return v
		}()},
		{"zoom below agg", func() *pb.PreindexTimezones {
			v := valid()
			v.Keys[0].Z = 1
			return v
		}()},
		{"agg above idx", func() *pb.PreindexTimezones {
			v := valid()
			v.AggZoom = 5
			return v
		}()},
		{"tile x out of range", func() *pb.PreindexTimezones {
			v := valid()
			v.Keys[0].X = -1
			return v
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Encode(fixture("Etc/Test"), EncodeOptions{Preindex: tc.pre}); err == nil {
				t.Fatal("Encode accepted invalid preindex")
			}
		})
	}
}

func fuzzySectionEntry(t *testing.T, data []byte) (off, length uint32, entry int) {
	t.Helper()
	count := int(binary.LittleEndian.Uint32(data[20:]))
	for i := 0; i < count; i++ {
		o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
		if binary.LittleEndian.Uint32(data[o:]) == embedbin.SectionFuzzy {
			return binary.LittleEndian.Uint32(data[o+4:]), binary.LittleEndian.Uint32(data[o+8:]), o
		}
	}
	t.Fatal("FUZZY section missing")
	return 0, 0, 0
}

func TestFuzzyReaderRejectsStructuralCorruption(t *testing.T) {
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{
			preindexKey(10, 10, 4, "Etc/Test"),
			preindexKey(120, 30, 4, "Etc/Test"),
		},
	}
	data, _ := openFuzzyFixture(t, pre, "Etc/Test")
	rechecksum := func(v []byte) {
		binary.LittleEndian.PutUint32(v[len(v)-4:], crc32.ChecksumIEEE(v[:len(v)-4]))
	}
	tests := []struct {
		name   string
		mutate func(v []byte)
	}{
		{"unknown profile", func(v []byte) { v[embedbin.ProfileOffset] = 2 }},
		{"unsorted keys", func(v []byte) {
			off, _, _ := fuzzySectionEntry(t, v)
			a := binary.LittleEndian.Uint64(v[off+16:])
			b := binary.LittleEndian.Uint64(v[off+24:])
			binary.LittleEndian.PutUint64(v[off+16:], b)
			binary.LittleEndian.PutUint64(v[off+24:], a)
		}},
		{"value index out of range", func(v []byte) {
			off, _, _ := fuzzySectionEntry(t, v)
			tileCount := binary.LittleEndian.Uint32(v[off+4:])
			binary.LittleEndian.PutUint16(v[off+16+8*tileCount:], 0x7fff)
		}},
		{"reserved nonzero", func(v []byte) {
			off, _, _ := fuzzySectionEntry(t, v)
			v[off+2] = 1
		}},
		{"length mismatch", func(v []byte) {
			off, _, _ := fuzzySectionEntry(t, v)
			binary.LittleEndian.PutUint32(v[off+4:], 1) // tile_count no longer matches length
		}},
		{"zoom order", func(v []byte) {
			off, _, _ := fuzzySectionEntry(t, v)
			v[off+1] = v[off] + 1 // agg_zoom > idx_zoom
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := slices.Clone(data)
			tc.mutate(mutated)
			rechecksum(mutated)
			if _, err := embedbin.Open(mutated); err == nil {
				t.Fatal("embedbin.Open accepted corrupted FUZZY data")
			}
		})
	}
}

func TestFuzzySectionAlignmentAndMinorVersion(t *testing.T) {
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{preindexKey(10, 10, 4, "Etc/Test")},
	}
	data, _ := openFuzzyFixture(t, pre, "Etc/Test")
	off, length, _ := fuzzySectionEntry(t, data)
	if off%8 != 0 {
		t.Fatalf("FUZZY offset %d not 8-byte aligned", off)
	}
	if length%4 != 0 {
		t.Fatalf("FUZZY length %d not 4-byte padded", length)
	}
	if data[4] != embedbin.FormatMajor || data[5] != embedbin.FormatMinor {
		t.Fatalf("format version = %d.%d", data[4], data[5])
	}
	if data[embedbin.ProfileOffset] != embedbin.ProfileE {
		t.Fatalf("profile byte = %d", data[embedbin.ProfileOffset])
	}
}

func TestFuzzyLookupLockFreeAllocations(t *testing.T) {
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{preindexKey(10, 10, 4, "Etc/Test")},
	}
	_, r := openFuzzyFixture(t, pre, "Etc/Test")
	if allocs := testing.AllocsPerRun(100, func() {
		_, _, _ = r.FuzzyLookup(10, 10)
	}); allocs != 0 {
		t.Fatalf("FuzzyLookup allocations = %v", allocs)
	}
	dst := make([]int32, 0, r.FuzzyLookupBufferSize())
	if allocs := testing.AllocsPerRun(100, func() {
		_, _ = r.FuzzyLookupAppend(dst, 10, 10)
	}); allocs != 0 {
		t.Fatalf("FuzzyLookupAppend allocations = %v", allocs)
	}
}

func TestFuzzyAbsentFromPlainFile(t *testing.T) {
	_, r := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	if r.HasFuzzy() {
		t.Fatal("plain file reports FUZZY")
	}
	if _, _, err := r.FuzzyLookup(10, 10); err != embedbin.ErrNoFuzzy {
		t.Fatalf("FuzzyLookup without section = %v", err)
	}
}
