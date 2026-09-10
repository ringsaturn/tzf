package embedenc

import (
	"bytes"
	"encoding/binary"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"hash/crc32"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
	"github.com/ringsaturn/tzf/v2/internal/polyline"
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

func openFixture(t *testing.T, input *pb.CompressedTopoTimezones, opts EncodeOptions) ([]byte, *embedbin.Reader) {
	t.Helper()
	data, err := Encode(input, opts)
	if err != nil {
		t.Fatal(err)
	}
	r, err := embedbin.Open(data)
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
		{0, 5, true},   // on the exterior edge: border belongs to the polygon
		{10, 10, true}, // exterior vertex
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

	ra, err := embedbin.OpenReaderAt(bytes.NewReader(data), int64(len(data)))
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
	}{
		{1, 1, true},  // between exterior and hole
		{5, 5, false}, // inside the hole
		{3, 5, true},  // on the hole edge: not excluded by the hole
		{0, 5, true},  // on the exterior edge: contained
	} {
		_, ok, err := r.Lookup(tc.x, tc.y)
		if err != nil || ok != tc.ok {
			t.Fatalf("Lookup(%v,%v) = %v,%v, want %v", tc.x, tc.y, ok, err, tc.ok)
		}
	}
}

func TestLookupIntoNameOrderAndCapacity(t *testing.T) {
	_, r := openFixture(t, fixture("Zed/Zone", "Alpha/Zone"), EncodeOptions{})
	if _, err := r.LookupInto(5, 5, make([]int32, 0, 1)); err != embedbin.ErrBufferTooSmall {
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
		o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
		if binary.LittleEndian.Uint32(mutated[o:]) == embedbin.SectionGrid {
			binary.LittleEndian.PutUint32(mutated[o:], 100)
			break
		}
	}
	flags := binary.LittleEndian.Uint32(mutated[8:])
	binary.LittleEndian.PutUint32(mutated[8:], flags&^embedbin.FlagGrid)
	binary.LittleEndian.PutUint32(mutated[len(mutated)-4:], crc32.ChecksumIEEE(mutated[:len(mutated)-4]))
	r, err := embedbin.Open(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LookupInto(5, 5, make([]int32, 0, 1)); err != embedbin.ErrBufferTooSmall {
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
		o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
		if binary.LittleEndian.Uint32(data[o:]) == embedbin.SectionPoints {
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
	r, err := embedbin.Open(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Lookup(5, 5); err == nil {
		t.Fatal("Lookup accepted trailing chunk byte")
	}
}

// loadDistTopo reads the full lite dataset intermediate from the path in
// TZF_PARITY_TOPO (a model-encoded *.gob produced by the pipeline or the
// bootstrap script); tests over the bundled dataset skip when it is unset.
func loadDistTopo(t testing.TB) *pb.CompressedTopoTimezones {
	t.Helper()
	path := os.Getenv("TZF_PARITY_TOPO")
	if path == "" {
		t.Skip("TZF_PARITY_TOPO not set; skipping bundled-dataset test")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	input := &pb.CompressedTopoTimezones{}
	if err := pb.Unmarshal(raw, input); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestBundledSemanticVerification(t *testing.T) {
	input := *loadDistTopo(t)
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
	ra, err := embedbin.OpenReaderAt(bytes.NewReader(data), int64(len(data)))
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
		o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
		if binary.LittleEndian.Uint32(mutated[o:]) == embedbin.SectionGroupDir {
			groupOff = binary.LittleEndian.Uint32(mutated[o+4:])
			break
		}
	}
	if groupOff == 0 {
		t.Fatal("GROUPDIR missing")
	}
	binary.LittleEndian.PutUint32(mutated[groupOff+20:], 100000)
	binary.LittleEndian.PutUint32(mutated[len(mutated)-4:], crc32.ChecksumIEEE(mutated[:len(mutated)-4]))
	r, err := embedbin.Open(mutated)
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
			v := pb.Clone(base)
			v.Method = pb.CompressMethod_COMPRESS_METHOD_UNSPECIFIED
			return v
		}(), EncodeOptions{}},
		{"empty name", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
			v.Timezones[0].Name = ""
			return v
		}(), EncodeOptions{}},
		{"NUL name", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
			v.Timezones[0].Name = "Bad\x00Name"
			return v
		}(), EncodeOptions{}},
		{"long version", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
			v.Version = "12345678901234567"
			return v
		}(), EncodeOptions{}},
		{"nested hole", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
			h := polygon([][2]float64{{2, 2}, {3, 2}, {3, 3}, {2, 2}})
			h.Holes = []*pb.CompressedTopoPolygon{polygon([][2]float64{{2.1, 2.1}, {2.2, 2.1}, {2.1, 2.2}, {2.1, 2.1}})}
			v.Timezones[0].Polygons[0].Holes = []*pb.CompressedTopoPolygon{h}
			return v
		}(), EncodeOptions{}},
		{"missing edge", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
			v.Timezones[0].Polygons[0].Exterior = []*pb.CompressedRingSegment{{
				Content: &pb.CompressedRingSegment_EdgeForward{EdgeForward: 99},
			}}
			return v
		}(), EncodeOptions{}},
		{"disconnected inline", func() *pb.CompressedTopoTimezones {
			v := pb.Clone(base)
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
			binary.LittleEndian.PutUint32(v[8:], binary.LittleEndian.Uint32(v[8:])&^embedbin.FlagGrid)
			rechecksum(v)
		}},
		{"duplicate section", func(v []byte) {
			count := int(binary.LittleEndian.Uint32(v[20:]))
			for i := 0; i < count; i++ {
				o := embedbin.HeaderSize + i*embedbin.SectionEntryLen
				if binary.LittleEndian.Uint32(v[o:]) == embedbin.SectionGrid {
					binary.LittleEndian.PutUint32(v[o:], embedbin.SectionNames)
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
			if _, err := embedbin.Open(mutated); err == nil {
				t.Fatal("embedbin.Open accepted structural corruption")
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
	withFuzzy, err := Encode(fixture("Etc/Test"), EncodeOptions{Preindex: &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "test",
		Keys: []*pb.PreindexTimezone{{Name: "Etc/Test", X: 8, Y: 7, Z: 4}},
	}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(withFuzzy, 5.0, 5.0)
	edges, err := Encode(sharedEdgeFixture(), EncodeOptions{})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(edges, 5.0, 5.0)
	mProfile, err := EncodeM(sharedEdgeFixture(), EncodeOptions{})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(mProfile, 5.0, 5.0)
	f.Fuzz(func(t *testing.T, data []byte, lng, lat float64) {
		r, err := embedbin.Open(data)
		if err != nil {
			return
		}
		_, _, _ = r.Lookup(lng, lat)
		_, _ = r.LookupInto(lng, lat, make([]int32, 0, r.TimezoneCount()))
		if r.HasFuzzy() {
			_, _, _ = r.FuzzyLookup(lng, lat)
			_, _ = r.FuzzyLookupAppend(make([]int32, 0, r.FuzzyLookupBufferSize()), lng, lat)
		}
		_, _ = r.Expand()
		if view, err := r.Flat(); err == nil {
			// Structurally valid M data must survive polygon assembly and
			// PIP over arbitrary coordinates without crashing.
			p := geom.Point{X: lng, Y: lat}
			for _, polys := range view.Polygons {
				for _, poly := range polys {
					pg := geom.NewI32Polygon(poly.Exterior, poly.Holes)
					_ = pg.ContainsPointAllowOnEdge(p)
				}
			}
			if view.Grid != nil {
				off, count := view.Grid.CellRange(lng, lat)
				for i := range count {
					_ = view.Grid.Candidate(off + i)
				}
			}
		}
	})
}
