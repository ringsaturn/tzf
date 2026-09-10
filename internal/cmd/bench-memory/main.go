// bench-memory measures the retained heap of each finder type after initialization.
// Output format: MEMORY\t<finder>\t<MB>
package main

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/debug"

	tzfdist "github.com/ringsaturn/tzf-dist"

	tzf "github.com/ringsaturn/tzf/v2"
	"github.com/ringsaturn/tzf/v2/x"
)

func readHeap() uint64 {
	runtime.GC()
	debug.FreeOSMemory()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func report(name string, before, after uint64) {
	mb := float64(int64(after)-int64(before)) / (1024 * 1024)
	fmt.Printf("MEMORY\t%s\t%.1f\n", name, mb)
}

func main() {
	var before, after uint64

	// EmbeddedFinder (lite .tzb queried in place; the embedded bytes are the
	// storage, so retained heap is the reader's directories only)
	before = readHeap()
	{
		f, err := tzf.NewEmbeddedFinder()
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("EmbeddedFinder", before, after)
		runtime.KeepAlive(f)
	}

	// TZBFinderReaderAt (lite .tzb through an io.ReaderAt source)
	before = readHeap()
	{
		data := bytes.Clone(tzfdist.LiteTZB)
		f, err := x.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("TZBFinderReaderAt", before, after)
		runtime.KeepAlive(f)
	}

	// FinderFromTZB (lite .tzb expanded + fuzzy fast path; the source byte
	// slice is released after expansion)
	before = readHeap()
	{
		f, err := tzf.NewFinderFromTZB(bytes.Clone(tzfdist.LiteTZB))
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("FinderFromTZB", before, after)
		runtime.KeepAlive(f)
	}

	// DefaultFinder (lite .tzm memory image: fuzzy hash maps + in-place
	// polygon view aliasing the embedded bytes)
	before = readHeap()
	{
		f, err := tzf.NewDefaultFinder()
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("DefaultFinder", before, after)
		runtime.KeepAlive(f)
	}

	// FullFinder (full .tzb expanded + fuzzy fast path)
	before = readHeap()
	{
		f, err := tzf.NewFullFinder()
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("FullFinder", before, after)
		runtime.KeepAlive(f)
	}
}
