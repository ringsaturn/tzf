// bench-memory measures the retained heap of each finder type after initialization.
// Output format: MEMORY\t<finder>\t<MB>
package main

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/ringsaturn/tzf"
	tzfdist "github.com/ringsaturn/tzf-dist"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
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

func buildTZB() []byte {
	input := &pb.CompressedTopoTimezones{}
	if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
		panic(err)
	}
	data, err := embedbin.Encode(input, embedbin.EncodeOptions{AllowShortcut: true})
	if err != nil {
		panic(err)
	}
	return data
}

func main() {
	var before, after uint64

	// FuzzyFinder
	before = readHeap()
	{
		input := &pb.PreindexTimezones{}
		if err := proto.Unmarshal(tzfdist.PreindexData, input); err != nil {
			panic(err)
		}
		f, err := tzf.NewFuzzyFinderFromPB(input)
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("FuzzyFinder", before, after)
		runtime.KeepAlive(f)
	}

	// Finder (lite, topology compress topo, with GridIndex)
	before = readHeap()
	{
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			panic(err)
		}
		f, err := tzf.NewFinderFromCompressedTopo(input)
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("Finder", before, after)
		runtime.KeepAlive(f)
	}

	// FinderNoGrid (lite, topology compress topo, GridIndex stripped)
	before = readHeap()
	{
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.TopologyCompressTopoData, input); err != nil {
			panic(err)
		}
		input.GridIndex = nil
		f, err := tzf.NewFinderFromCompressedTopo(input)
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("FinderNoGrid", before, after)
		runtime.KeepAlive(f)
	}

	// TZBFinder (lite .tzb retained as a byte slice)
	before = readHeap()
	{
		data := buildTZB()
		f, err := tzf.NewFinderFromTZB(data)
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("TZBFinder", before, after)
		runtime.KeepAlive(f)
	}

	// TZBFinderReaderAt (lite .tzb through an io.ReaderAt source)
	before = readHeap()
	{
		data := buildTZB()
		f, err := tzf.NewFinderFromTZBReaderAt(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("TZBFinderReaderAt", before, after)
		runtime.KeepAlive(f)
	}

	// DefaultFinder (FuzzyFinder + Finder combined, lite data)
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

	// FullFinderWithoutPreindex (Finder with full-precision data, no FuzzyFinder layer)
	before = readHeap()
	{
		input := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(tzfdist.CompressTopoData, input); err != nil {
			panic(err)
		}
		f, err := tzf.NewFinderFromCompressedTopo(input)
		if err != nil {
			panic(err)
		}
		after = readHeap()
		report("FullFinderWithoutPreindex", before, after)
		runtime.KeepAlive(f)
	}

	// FullFinder (DefaultFinder with full-precision data)
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
