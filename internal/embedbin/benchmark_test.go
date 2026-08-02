package embedbin

import (
	"io"
	"os"
	"strconv"
	"sync"
	"testing"

	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"google.golang.org/protobuf/proto"
)

var (
	benchmarkOnce sync.Once
	benchmarkData []byte
	benchmarkErr  error
)

func loadBenchmarkData() ([]byte, error) {
	benchmarkOnce.Do(func() {
		var input pb.CompressedTopoTimezones
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
			benchmarkErr = err
			return
		}
		benchmarkData, benchmarkErr = Encode(&input, EncodeOptions{})
	})
	return benchmarkData, benchmarkErr
}

func BenchmarkLookupTransport(b *testing.B) {
	data, err := loadBenchmarkData()
	if err != nil {
		b.Fatal(err)
	}
	queries := [][2]float64{
		{139.6917, 35.6895}, {-74.006, 40.7128}, {0, 51.5},
		{151.2093, -33.8688}, {-157.8583, 21.3069}, {179.9, 0},
	}
	run := func(b *testing.B, r *Reader) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q := queries[i%len(queries)]
			if _, _, err := r.Lookup(q[0], q[1]); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.Run("bytes", func(b *testing.B) {
		r, err := Open(data)
		if err != nil {
			b.Fatal(err)
		}
		run(b, r)
	})
	b.Run("file-reader-at", func(b *testing.B) {
		file, err := os.CreateTemp(b.TempDir(), "tzf-*.tzb")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			b.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			b.Fatal(err)
		}
		r, err := OpenReaderAt(file, int64(len(data)))
		if err != nil {
			b.Fatal(err)
		}
		run(b, r)
	})
	b.Run("sector-cache", func(b *testing.B) {
		source := &sectorReaderAt{data: data}
		r, err := OpenReaderAt(source, int64(len(data)))
		if err != nil {
			b.Fatal(err)
		}
		source.reads, source.bytes = 0, 0
		run(b, r)
		b.ReportMetric(float64(source.reads)/float64(b.N), "sector_reads/op")
		b.ReportMetric(float64(source.bytes)/float64(b.N), "source_bytes/op")
	})
}

func BenchmarkEncodeChunkTargets(b *testing.B) {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		b.Fatal(err)
	}
	for _, target := range []int{128, 256, 512} {
		b.Run(strconv.Itoa(target), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				data, err := Encode(&input, EncodeOptions{ChunkTarget: target})
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(data)), "file_bytes")
			}
		})
	}
}

func BenchmarkChunkTargetLookup(b *testing.B) {
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, &input); err != nil {
		b.Fatal(err)
	}
	queries := [][2]float64{
		{139.6917, 35.6895}, {-74.006, 40.7128}, {0, 51.5},
		{151.2093, -33.8688}, {-157.8583, 21.3069}, {179.9, 0},
	}
	for _, target := range []int{128, 256, 512} {
		data, err := Encode(&input, EncodeOptions{ChunkTarget: target})
		if err != nil {
			b.Fatal(err)
		}
		r, err := Open(data)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(strconv.Itoa(target), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(data)), "file_bytes")
			for i := 0; i < b.N; i++ {
				q := queries[i%len(queries)]
				if _, _, err := r.Lookup(q[0], q[1]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type sectorReaderAt struct {
	data     []byte
	cache    [4096]byte
	sector   int64
	valid    bool
	cacheLen int
	reads    int64
	bytes    int64
}

func (s *sectorReaderAt) ReadAt(dst []byte, off int64) (int, error) {
	total := 0
	for len(dst) > 0 && off < int64(len(s.data)) {
		sector := off / int64(len(s.cache))
		if !s.valid || sector != s.sector {
			start := sector * int64(len(s.cache))
			s.cacheLen = copy(s.cache[:], s.data[start:min(start+int64(len(s.cache)), int64(len(s.data)))])
			s.sector = sector
			s.valid = true
			s.reads++
			s.bytes += int64(s.cacheLen)
		}
		within := int(off - sector*int64(len(s.cache)))
		n := copy(dst, s.cache[within:s.cacheLen])
		dst = dst[n:]
		off += int64(n)
		total += n
	}
	if len(dst) != 0 {
		return total, io.EOF
	}
	return total, nil
}

var _ io.ReaderAt = (*sectorReaderAt)(nil)
