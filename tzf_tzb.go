package tzf

import (
	"io"

	"github.com/ringsaturn/tzf/internal/embedbin"
)

// NewFinderFromTZB builds a Finder over a byte-backed TZF embedded binary
// file. The file is validated when opened and retained without expanding its
// geometry into Go polygon objects.
func NewFinderFromTZB(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	return newTZBFinder(reader)
}

// NewFinderFromTZBReaderAt builds a Finder that reads a TZF embedded binary
// file directly from an io.ReaderAt source. size is the exact file size.
//
// Queries use a fixed internal workspace and are safe for concurrent callers.
// ReaderAt access is serialized to preserve the zero-allocation query path.
func NewFinderFromTZBReaderAt(source io.ReaderAt, size int64) (F, error) {
	reader, err := embedbin.OpenReaderAt(source, size)
	if err != nil {
		return nil, err
	}
	return newTZBFinder(reader)
}

type tzbFinder struct {
	reader         *embedbin.Reader
	names          []string
	lookupCapacity int
}

var _ F = (*tzbFinder)(nil)

func newTZBFinder(reader *embedbin.Reader) (*tzbFinder, error) {
	names := make([]string, reader.TimezoneCount())
	for i := range names {
		name, err := reader.Name(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
	}
	return &tzbFinder{
		reader: reader, names: names, lookupCapacity: reader.LookupBufferSize(),
	}, nil
}

// GetTimezoneName returns the first matching timezone in source order.
//
// The F interface cannot expose a lazy structural read error. Such an error is
// treated as no match. Open-time validation and the CRC catch ordinary file
// corruption before queries begin.
func (f *tzbFinder) GetTimezoneName(lng, lat float64) string {
	idx, ok, err := f.reader.Lookup(lng, lat)
	if err != nil || !ok {
		return ""
	}
	return f.names[idx]
}

func (f *tzbFinder) GetTimezoneNames(lng, lat float64) ([]string, error) {
	indices, err := f.reader.LookupInto(lng, lat, make([]int32, 0, f.lookupCapacity))
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		return nil, nil
	}
	names := make([]string, len(indices))
	for i, idx := range indices {
		names[i] = f.names[idx]
	}
	return names, nil
}

func (f *tzbFinder) TimezoneNames() []string {
	return f.names
}

func (f *tzbFinder) DataVersion() string {
	return f.reader.DataVersion()
}
