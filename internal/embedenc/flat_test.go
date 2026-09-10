package embedenc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"hash/crc32"
	"slices"
	"testing"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

func openMFixture(t *testing.T, input *pb.CompressedTopoTimezones, opts EncodeOptions) ([]byte, *embedbin.Reader) {
	t.Helper()
	data, err := EncodeM(input, opts)
	if err != nil {
		t.Fatal(err)
	}
	r, err := embedbin.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	return data, r
}

func rechecksum(data []byte) {
	binary.LittleEndian.PutUint32(data[len(data)-4:], crc32.ChecksumIEEE(data[:len(data)-4]))
}

// findSectionEntry returns the table offset of the first entry with the given
// type, or -1.
func findSectionEntry(data []byte, typ uint32) int {
	count := int(binary.LittleEndian.Uint32(data[20:]))
	for i := range count {
		o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
		if binary.LittleEndian.Uint32(data[o:]) == typ {
			return o
		}
	}
	return -1
}

// TestFlatMatchesExpansion pins the M builder to the §5.1 expansion: the flat
// rings of an M file must be identical to Expand over an E file of the same
// source, including junction-duplicate removal on reversed shared edges.
func TestFlatMatchesExpansion(t *testing.T) {
	for name, input := range map[string]*pb.CompressedTopoTimezones{
		"shared-edges": sharedEdgeFixture(),
		"simple":       fixture("Etc/Test"),
	} {
		t.Run(name, func(t *testing.T) {
			_, er := openFixture(t, input, EncodeOptions{})
			expanded, err := er.Expand()
			if err != nil {
				t.Fatal(err)
			}
			_, mr := openMFixture(t, input, EncodeOptions{})
			view, err := mr.Flat()
			if err != nil {
				t.Fatal(err)
			}
			if view.Version != expanded.Version || !slices.Equal(view.Names, expanded.Names) {
				t.Fatalf("metadata = %q %v", view.Version, view.Names)
			}
			if len(view.Polygons) != len(expanded.Polygons) {
				t.Fatal("polygon set sizes differ")
			}
			for i := range view.Polygons {
				if len(view.Polygons[i]) != len(expanded.Polygons[i]) {
					t.Fatalf("timezone %d polygon count differs", i)
				}
				for j := range view.Polygons[i] {
					got, want := view.Polygons[i][j], expanded.Polygons[i][j]
					if !slices.Equal(got.Exterior, want.Exterior) {
						t.Fatalf("timezone %d polygon %d exterior differs", i, j)
					}
					if len(got.Holes) != len(want.Holes) {
						t.Fatalf("timezone %d polygon %d hole count differs", i, j)
					}
					for h := range got.Holes {
						if !slices.Equal(got.Holes[h], want.Holes[h]) {
							t.Fatalf("timezone %d polygon %d hole %d differs", i, j, h)
						}
					}
				}
			}
			if err := VerifyM(input, mr); err != nil {
				t.Fatalf("VerifyM: %v", err)
			}
		})
	}
}

func TestDenseGridMatchesGridMap(t *testing.T) {
	_, er := openFixture(t, sharedEdgeFixture(), EncodeOptions{})
	expanded, err := er.Expand()
	if err != nil {
		t.Fatal(err)
	}
	_, mr := openMFixture(t, sharedEdgeFixture(), EncodeOptions{})
	view, err := mr.Flat()
	if err != nil {
		t.Fatal(err)
	}
	if view.Grid == nil {
		t.Fatal("dense grid missing")
	}
	for lat := -90.0; lat <= 90.0; lat += 0.5 {
		for lng := -180.0; lng <= 180.0; lng += 0.5 {
			off, count := view.Grid.CellRange(lng, lat)
			var got []int32
			for i := range count {
				got = append(got, view.Grid.Candidate(off+i))
			}
			want := expanded.Grid[[2]int16{int16(floorInt(lng)), int16(floorInt(lat))}]
			if !slices.Equal(got, want) {
				t.Fatalf("dense grid at (%v,%v) = %v, want %v", lng, lat, got, want)
			}
		}
	}
	if off, count := view.Grid.CellRange(200, 0); off != 0 || count != 0 {
		t.Fatal("out-of-domain cell range not empty")
	}
}

func floorInt(v float64) int {
	i := int(v)
	if float64(i) > v {
		i--
	}
	return i
}

func TestProfileGuards(t *testing.T) {
	eData, eReader := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	mData, mReader := openMFixture(t, fixture("Etc/Test"), EncodeOptions{})

	if _, _, err := mReader.Lookup(5, 5); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("M Lookup error = %v", err)
	}
	if _, err := mReader.LookupInto(5, 5, make([]int32, 0, 1)); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("M LookupInto error = %v", err)
	}
	if _, err := mReader.Expand(); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("M Expand error = %v", err)
	}
	if _, err := eReader.Flat(); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("E Flat error = %v", err)
	}
	if err := VerifyM(fixture("Etc/Test"), eReader); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("VerifyM on E error = %v", err)
	}
	if !mReader.ProfileM() || eReader.ProfileM() {
		t.Fatal("profile reporting")
	}

	// Unknown profile bytes stay rejected.
	unknown := slices.Clone(eData)
	unknown[embedbin.ProfileOffset] = 2
	rechecksum(unknown)
	if _, err := embedbin.Open(unknown); err == nil {
		t.Fatal("embedbin.Open accepted unknown profile")
	}
	_ = mData
}

func TestFuzzyOnMProfile(t *testing.T) {
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{{Name: "Etc/Test", X: 8, Y: 7, Z: 4}},
	}
	_, r := openMFixture(t, fixture("Etc/Test"), EncodeOptions{Preindex: pre})
	if !r.HasFuzzy() {
		t.Fatal("FUZZY section missing")
	}
	single, multi, err := r.FuzzyMaps()
	if err != nil || len(single) != 1 || len(multi) != 0 {
		t.Fatalf("FuzzyMaps = %d,%d,%v", len(single), len(multi), err)
	}
}

func TestFlatReaderAtFallback(t *testing.T) {
	data, byteReader := openMFixture(t, sharedEdgeFixture(), EncodeOptions{})
	byteView, err := byteReader.Flat()
	if err != nil {
		t.Fatal(err)
	}
	ra, err := embedbin.OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	raView, err := ra.Flat()
	if err != nil {
		t.Fatal(err)
	}
	for i := range byteView.Polygons {
		for j := range byteView.Polygons[i] {
			if !slices.Equal(byteView.Polygons[i][j].Exterior, raView.Polygons[i][j].Exterior) {
				t.Fatalf("ReaderAt flat exterior differs at %d/%d", i, j)
			}
		}
	}
	off, count := raView.Grid.CellRange(2.5, 5.5)
	wantOff, wantCount := byteView.Grid.CellRange(2.5, 5.5)
	if off != wantOff || count != wantCount {
		t.Fatal("ReaderAt dense grid differs")
	}
}

func TestMRejectsStructuralCorruption(t *testing.T) {
	data, _ := openMFixture(t, fixture("Etc/Test"), EncodeOptions{})
	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{"chunk target nonzero", func(v []byte) {
			binary.LittleEndian.PutUint32(v[44:], 256)
			rechecksum(v)
		}},
		{"ring partition", func(v []byte) {
			o := findSectionEntry(v, embedbin.SectionFlatRingDir)
			off := binary.LittleEndian.Uint32(v[o+4:])
			binary.LittleEndian.PutUint32(v[off:], 1) // point_first of ring 0
			rechecksum(v)
		}},
		{"ring too short", func(v []byte) {
			o := findSectionEntry(v, embedbin.SectionFlatRingDir)
			off := binary.LittleEndian.Uint32(v[o+4:])
			binary.LittleEndian.PutUint32(v[off+4:], 2) // point_count < 3
			rechecksum(v)
		}},
		{"flatpoints truncated", func(v []byte) {
			o := findSectionEntry(v, embedbin.SectionFlatPoints)
			length := binary.LittleEndian.Uint32(v[o+8:])
			binary.LittleEndian.PutUint32(v[o+8:], length-8)
			rechecksum(v)
		}},
		{"E section in M file", func(v []byte) {
			o := findSectionEntry(v, embedbin.SectionGrid)
			binary.LittleEndian.PutUint32(v[o:], embedbin.SectionPoints)
			rechecksum(v)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := slices.Clone(data)
			tc.mutate(mutated)
			if _, err := embedbin.Open(mutated); err == nil {
				t.Fatal("embedbin.Open accepted structural corruption")
			}
		})
	}

	// The reverse direction: an M-only section type inside an E file.
	eData, _ := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	mutated := slices.Clone(eData)
	o := findSectionEntry(mutated, embedbin.SectionGrid)
	binary.LittleEndian.PutUint32(mutated[o:], embedbin.SectionFlatPoints)
	rechecksum(mutated)
	if _, err := embedbin.Open(mutated); err == nil {
		t.Fatal("embedbin.Open accepted M section in E file")
	}
}

// TestVerifyMSemanticTrustBoundary mirrors the E-profile deterministic
// encoding test: a coordinate flip is structurally valid (embedbin.Open accepts it)
// but must be caught by deep verification.
func TestVerifyMSemanticTrustBoundary(t *testing.T) {
	input := fixture("Etc/Test")
	a, err := EncodeM(input, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncodeM(input, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("M encoding is nondeterministic")
	}
	mutated := slices.Clone(a)
	o := findSectionEntry(mutated, embedbin.SectionFlatPoints)
	off := binary.LittleEndian.Uint32(mutated[o+4:])
	x := binary.LittleEndian.Uint32(mutated[off:])
	binary.LittleEndian.PutUint32(mutated[off:], x+1)
	rechecksum(mutated)
	r, err := embedbin.Open(mutated)
	if err != nil {
		t.Fatalf("semantic mutation was rejected structurally: %v", err)
	}
	if err := VerifyM(input, r); err == nil {
		t.Fatal("VerifyM accepted a mutated coordinate")
	}
}

func TestBundledMSemanticVerification(t *testing.T) {
	input := *loadDistTopo(t)
	data, r := openMFixture(t, &input, EncodeOptions{})
	if len(data) < 8_000_000 || len(data) > 16_000_000 {
		t.Fatalf("unexpected lite .tzm size %d", len(data))
	}
	if err := VerifyM(&input, r); err != nil {
		t.Fatal(err)
	}
}
