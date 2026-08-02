package embedbin

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"math"
	"slices"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/polyline"
	"google.golang.org/protobuf/proto"
)

func inlineRing(points ...[2]float64) []*pb.CompressedRingSegment {
	coords := make([][]float64, len(points))
	for i, p := range points {
		coords[i] = []float64{p[0], p[1]}
	}
	return []*pb.CompressedRingSegment{{
		Content: &pb.CompressedRingSegment_Inline{
			Inline: &pb.CompressedInlinePoints{Points: polyline.EncodeCoords(coords)},
		},
	}}
}

func polygon(ext [][2]float64, holes ...[][2]float64) *pb.CompressedTopoPolygon {
	p := &pb.CompressedTopoPolygon{Exterior: inlineRing(ext...)}
	for _, h := range holes {
		p.Holes = append(p.Holes, &pb.CompressedTopoPolygon{Exterior: inlineRing(h...)})
	}
	return p
}

func fixture(names ...string) *pb.CompressedTopoTimezones {
	square := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}
	out := &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: "test",
	}
	for _, name := range names {
		out.Timezones = append(out.Timezones, &pb.CompressedTopoTimezone{
			Name: name, Polygons: []*pb.CompressedTopoPolygon{polygon(square)},
		})
	}
	return out
}

func openFixture(t *testing.T, input *pb.CompressedTopoTimezones, opts EncodeOptions) ([]byte, *Reader) {
	t.Helper()
	data, err := Encode(input, opts)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	return data, r
}

func TestEncodeLookupAndReaderAt(t *testing.T) {
	data, r := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	for _, tc := range []struct {
		lng, lat float64
		ok       bool
	}{
		{5, 5, true},
		{0, 5, false},
		{10, 10, false},
		{-1, 5, false},
		{math.NaN(), 0, false},
		{math.Inf(1), 0, false},
		{181, 0, false},
	} {
		idx, ok, err := r.Lookup(tc.lng, tc.lat)
		if err != nil {
			t.Fatalf("Lookup(%v,%v): %v", tc.lng, tc.lat, err)
		}
		if ok != tc.ok || ok && idx != 0 {
			t.Fatalf("Lookup(%v,%v) = %d,%v, want ok=%v", tc.lng, tc.lat, idx, ok, tc.ok)
		}
	}
	name, err := r.Name(0)
	if err != nil || string(name) != "Etc/Test" {
		t.Fatalf("Name = %q, %v", name, err)
	}
	if r.DataVersion() != "test" || r.TimezoneCount() != 1 {
		t.Fatalf("metadata = %q,%d", r.DataVersion(), r.TimezoneCount())
	}

	ra, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	idx, ok, err := ra.Lookup(5, 5)
	if err != nil || !ok || idx != 0 {
		t.Fatalf("ReaderAt Lookup = %d,%v,%v", idx, ok, err)
	}
	appended, err := ra.AppendName([]byte("name="), 0)
	if err != nil || string(appended) != "name=Etc/Test" {
		t.Fatalf("AppendName = %q,%v", appended, err)
	}
}

func TestHoleBoundarySemantics(t *testing.T) {
	ext := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}
	hole := [][2]float64{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}}
	input := &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: "holes",
		Timezones: []*pb.CompressedTopoTimezone{{
			Name: "Hole/Test", Polygons: []*pb.CompressedTopoPolygon{polygon(ext, hole)},
		}},
	}
	_, r := openFixture(t, input, EncodeOptions{})
	for _, tc := range []struct {
		x, y float64
		ok   bool
	}{{1, 1, true}, {5, 5, false}, {3, 5, true}, {0, 5, false}} {
		_, ok, err := r.Lookup(tc.x, tc.y)
		if err != nil || ok != tc.ok {
			t.Fatalf("Lookup(%v,%v) = %v,%v, want %v", tc.x, tc.y, ok, err, tc.ok)
		}
	}
}

func TestLookupIntoNameOrderAndCapacity(t *testing.T) {
	_, r := openFixture(t, fixture("Zed/Zone", "Alpha/Zone"), EncodeOptions{})
	if _, err := r.LookupInto(5, 5, make([]int32, 0, 1)); err != ErrBufferTooSmall {
		t.Fatalf("small buffer error = %v", err)
	}
	got, err := r.LookupInto(5, 5, make([]int32, 0, 2))
	if err != nil || !slices.Equal(got, []int32{1, 0}) {
		t.Fatalf("LookupInto = %v,%v", got, err)
	}
}

func TestGridOptionalLinearFallback(t *testing.T) {
	data, _ := openFixture(t, fixture("Zed/Zone", "Alpha/Zone"), EncodeOptions{})
	mutated := slices.Clone(data)
	count := int(binary.LittleEndian.Uint32(mutated[20:]))
	for i := 0; i < count; i++ {
		o := headerSize + i*sectionEntryLen
		if binary.LittleEndian.Uint32(mutated[o:]) == sectionGrid {
			binary.LittleEndian.PutUint32(mutated[o:], 10)
			break
		}
	}
	flags := binary.LittleEndian.Uint32(mutated[8:])
	binary.LittleEndian.PutUint32(mutated[8:], flags&^flagGrid)
	binary.LittleEndian.PutUint32(mutated[len(mutated)-4:], crc32.ChecksumIEEE(mutated[:len(mutated)-4]))
	r, err := Open(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LookupInto(5, 5, make([]int32, 0, 1)); err != ErrBufferTooSmall {
		t.Fatalf("linear fallback capacity error = %v", err)
	}
	got, err := r.LookupInto(5, 5, make([]int32, 0, 2))
	if err != nil || !slices.Equal(got, []int32{1, 0}) {
		t.Fatalf("linear fallback = %v,%v", got, err)
	}
}

func TestShortcutFlag(t *testing.T) {
	triangle := [][2]float64{{0.1, 0.1}, {0.9, 0.1}, {0.1, 0.9}, {0.1, 0.1}}
	input := &pb.CompressedTopoTimezones{
		Method:  pb.CompressMethod_COMPRESS_METHOD_POLYLINE,
		Version: "shortcut",
		Timezones: []*pb.CompressedTopoTimezone{{
			Name: "Triangle", Polygons: []*pb.CompressedTopoPolygon{polygon(triangle)},
		}},
	}
	_, strict := openFixture(t, input, EncodeOptions{})
	if _, ok, err := strict.Lookup(0.8, 0.8); err != nil || ok {
		t.Fatalf("strict lookup = %v,%v", ok, err)
	}
	_, shortcut := openFixture(t, input, EncodeOptions{AllowShortcut: true})
	if _, ok, err := shortcut.Lookup(0.8, 0.8); err != nil || !ok {
		t.Fatalf("shortcut lookup = %v,%v", ok, err)
	}
}

func TestTrailingChunkBytesRejectedLazily(t *testing.T) {
	data, _ := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	pointEntry := -1
	count := int(binary.LittleEndian.Uint32(data[20:]))
	for i := 0; i < count; i++ {
		o := headerSize + i*sectionEntryLen
		if binary.LittleEndian.Uint32(data[o:]) == sectionPoints {
			pointEntry = o
			break
		}
	}
	if pointEntry < 0 {
		t.Fatal("POINTS entry missing")
	}
	mutated := make([]byte, len(data)+1)
	copy(mutated, data[:len(data)-4])
	mutated[len(data)-4] = 0
	binary.LittleEndian.PutUint32(mutated[16:], uint32(len(mutated)))
	length := binary.LittleEndian.Uint32(mutated[pointEntry+8:])
	binary.LittleEndian.PutUint32(mutated[pointEntry+8:], length+1)
	binary.LittleEndian.PutUint32(mutated[len(mutated)-4:], crc32.ChecksumIEEE(mutated[:len(mutated)-4]))
	r, err := Open(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Lookup(5, 5); err == nil {
		t.Fatal("Lookup accepted trailing chunk byte")
	}
}

func TestBundledSemanticVerification(t *testing.T) {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		t.Fatal(err)
	}
	data, r := openFixture(t, &input, EncodeOptions{AllowShortcut: true})
	if len(data) < 2_000_000 || len(data) > 4_000_000 {
		t.Fatalf("unexpected lite size %d", len(data))
	}
	if err := Verify(&input, r); err != nil {
		t.Fatal(err)
	}
}

func TestLookupAllocations(t *testing.T) {
	data, r := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	if allocs := testing.AllocsPerRun(100, func() {
		_, _, _ = r.Lookup(5, 5)
	}); allocs != 0 {
		t.Fatalf("byte Lookup allocations = %v", allocs)
	}
	dst := make([]int32, 0, r.TimezoneCount())
	if allocs := testing.AllocsPerRun(100, func() {
		_, _ = r.LookupInto(5, 5, dst)
	}); allocs != 0 {
		t.Fatalf("byte LookupInto allocations = %v", allocs)
	}
	ra, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_, _, _ = ra.Lookup(5, 5)
	}); allocs != 0 {
		t.Fatalf("ReaderAt Lookup allocations = %v", allocs)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_, _ = ra.LookupInto(5, 5, dst)
	}); allocs != 0 {
		t.Fatalf("ReaderAt LookupInto allocations = %v", allocs)
	}
}

func TestDeterministicEncodingAndSemanticTrustBoundary(t *testing.T) {
	input := fixture("Etc/Test")
	a, err := Encode(input, EncodeOptions{ChunkTarget: 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode(input, EncodeOptions{ChunkTarget: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("encoding is nondeterministic")
	}

	mutated := slices.Clone(a)
	groupOff := uint32(0)
	count := int(binary.LittleEndian.Uint32(mutated[20:]))
	for i := 0; i < count; i++ {
		o := headerSize + i*sectionEntryLen
		if binary.LittleEndian.Uint32(mutated[o:]) == sectionGroupDir {
			groupOff = binary.LittleEndian.Uint32(mutated[o+4:])
			break
		}
	}
	if groupOff == 0 {
		t.Fatal("GROUPDIR missing")
	}
	binary.LittleEndian.PutUint32(mutated[groupOff+20:], 100000)
	binary.LittleEndian.PutUint32(mutated[len(mutated)-4:], crc32.ChecksumIEEE(mutated[:len(mutated)-4]))
	r, err := Open(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Lookup(5, 5); err != nil {
		t.Fatalf("semantic mutation was unsafe: %v", err)
	}
	if err := Verify(input, r); err == nil {
		t.Fatal("deep verification accepted inconsistent GROUPDIR endpoint")
	}
}

func TestEncoderRejectsInvalidInput(t *testing.T) {
	base := fixture("Etc/Test")
	tests := []struct {
		name  string
		input *pb.CompressedTopoTimezones
		opts  EncodeOptions
	}{
		{"nil", nil, EncodeOptions{}},
		{"compression method", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Method = pb.CompressMethod_COMPRESS_METHOD_UNSPECIFIED
			return v
		}(), EncodeOptions{}},
		{"empty name", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Timezones[0].Name = ""
			return v
		}(), EncodeOptions{}},
		{"NUL name", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Timezones[0].Name = "Bad\x00Name"
			return v
		}(), EncodeOptions{}},
		{"long version", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Version = "12345678901234567"
			return v
		}(), EncodeOptions{}},
		{"nested hole", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			h := polygon([][2]float64{{2, 2}, {3, 2}, {3, 3}, {2, 2}})
			h.Holes = []*pb.CompressedTopoPolygon{polygon([][2]float64{{2.1, 2.1}, {2.2, 2.1}, {2.1, 2.2}, {2.1, 2.1}})}
			v.Timezones[0].Polygons[0].Holes = []*pb.CompressedTopoPolygon{h}
			return v
		}(), EncodeOptions{}},
		{"missing edge", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Timezones[0].Polygons[0].Exterior = []*pb.CompressedRingSegment{{
				Content: &pb.CompressedRingSegment_EdgeForward{EdgeForward: 99},
			}}
			return v
		}(), EncodeOptions{}},
		{"disconnected inline", func() *pb.CompressedTopoTimezones {
			v := proto.Clone(base).(*pb.CompressedTopoTimezones)
			v.Timezones[0].Polygons[0].Exterior = append(
				inlineRing([2]float64{0, 0}, [2]float64{1, 0}),
				inlineRing([2]float64{2, 0}, [2]float64{0, 0})...,
			)
			return v
		}(), EncodeOptions{}},
		{"chunk target low", base, EncodeOptions{ChunkTarget: -1}},
		{"chunk target high", base, EncodeOptions{ChunkTarget: math.MaxUint16 + 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Encode(tc.input, tc.opts); err == nil {
				t.Fatal("Encode accepted invalid input")
			}
		})
	}
}

func TestReaderRejectsStructuralCorruption(t *testing.T) {
	data, _ := openFixture(t, fixture("Etc/Test"), EncodeOptions{})
	rechecksum := func(data []byte) {
		binary.LittleEndian.PutUint32(data[len(data)-4:], crc32.ChecksumIEEE(data[:len(data)-4]))
	}
	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{"magic", func(v []byte) { v[0] = 'X'; rechecksum(v) }},
		{"file size", func(v []byte) { binary.LittleEndian.PutUint32(v[16:], uint32(len(v)-1)); rechecksum(v) }},
		{"CRC", func(v []byte) { v[len(v)-1] ^= 1 }},
		{"GRID flag", func(v []byte) {
			binary.LittleEndian.PutUint32(v[8:], binary.LittleEndian.Uint32(v[8:])&^flagGrid)
			rechecksum(v)
		}},
		{"duplicate section", func(v []byte) {
			count := int(binary.LittleEndian.Uint32(v[20:]))
			for i := 0; i < count; i++ {
				o := headerSize + i*sectionEntryLen
				if binary.LittleEndian.Uint32(v[o:]) == sectionGrid {
					binary.LittleEndian.PutUint32(v[o:], sectionNames)
					break
				}
			}
			rechecksum(v)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := slices.Clone(data)
			tc.mutate(mutated)
			if _, err := Open(mutated); err == nil {
				t.Fatal("Open accepted structural corruption")
			}
		})
	}
}

func FuzzOpenAndLookup(f *testing.F) {
	data, err := Encode(fixture("Etc/Test"), EncodeOptions{})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data, 5.0, 5.0)
	f.Fuzz(func(t *testing.T, data []byte, lng, lat float64) {
		r, err := Open(data)
		if err != nil {
			return
		}
		_, _, _ = r.Lookup(lng, lat)
		_, _ = r.LookupInto(lng, lat, make([]int32, 0, r.TimezoneCount()))
	})
}
