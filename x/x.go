// Package x holds experimental tzf surface: API that is useful in production
// but not ready for the compatibility promise the root package makes.
//
// # Stability
//
// This package is exempt from the module's semantic-versioning promise. In a
// vX.Y.Z release, a bump of Y may change or remove anything here; only Z
// bumps are guaranteed not to. The root package keeps the normal promise —
// breaking changes only at a major version. The convention, and the reason
// for the package name, follow golang.org/x/... — pin an exact version if you
// depend on this package and cannot absorb a break at a minor release.
package x

import (
	"io"

	"github.com/ringsaturn/tzf/v2"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/inplace"
)

// NewFinderFromTZBReaderAt builds a finder that reads a TZF embedded binary
// file (.tzb) directly from an io.ReaderAt source rather than from a byte
// slice. size is the exact file size.
//
// The file is validated when opened, so an os.File source is read through
// once at open; afterwards only the bytes a query touches are read. Queries
// use a fixed internal workspace and are safe for concurrent callers:
// ReaderAt access is serialized to preserve the allocation-free query path,
// so throughput does not scale with cores the way the byte-backed and
// expanded finders do.
//
// Semantics match [tzf.NewFinderFromTZB], including the FUZZY fast path when
// the file carries a FUZZY section, and the returned value also satisfies
// [tzf.GeoJSONer].
func NewFinderFromTZBReaderAt(source io.ReaderAt, size int64) (tzf.F, error) {
	reader, err := embedbin.OpenReaderAt(source, size)
	if err != nil {
		return nil, err
	}
	return inplace.New(reader)
}
