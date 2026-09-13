// Command topo2embed converts CompressedTopoTimezones pipeline data to the
// embedded binary container: profile e emits the chunked .tzb layout, profile
// m emits the flat memory-image .tzm layout.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/v2/internal/embedenc"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

func main() {
	output := flag.String("o", "", "output path (default: input name with .tzb/.tzm)")
	chunk := flag.Int("chunk", 0, "target points per chunk (default 64; E profile only)")
	allowShortcut := flag.Bool("allow-shortcut", false, "enable the single-candidate GRID shortcut (E profile only)")
	preindexPath := flag.String("preindex", "", "PreindexTimezones .bin to embed as the FUZZY section")
	profile := flag.String("profile", "e", "output profile: e (.tzb, embedded) or m (.tzm, memory image)")
	flag.Parse()
	if flag.NArg() != 1 || (*profile != "e" && *profile != "m") {
		fmt.Fprintln(os.Stderr, "usage: topo2embed [-o output] [-profile e|m] [-chunk 64] [-allow-shortcut] [-preindex preindex.bin] input.compress.topo.bin")
		os.Exit(2)
	}
	inputPath := flag.Arg(0)
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		fail("read input", err)
	}
	var input pb.CompressedTopoTimezones
	if err := pb.Unmarshal(raw, &input); err != nil {
		fail("decode CompressedTopoTimezones", err)
	}
	opts := embedenc.EncodeOptions{ChunkTarget: *chunk, AllowShortcut: *allowShortcut}
	if *preindexPath != "" {
		preRaw, err := os.ReadFile(*preindexPath)
		if err != nil {
			fail("read preindex", err)
		}
		preindex := &pb.PreindexTimezones{}
		if err := pb.Unmarshal(preRaw, preindex); err != nil {
			fail("decode PreindexTimezones", err)
		}
		opts.Preindex = preindex
	}
	var data []byte
	var err2 error
	ext := ".tzb"
	if *profile == "m" {
		data, err2 = embedenc.EncodeM(&input, opts)
		ext = ".tzm"
	} else {
		data, err2 = embedenc.Encode(&input, opts)
	}
	if err2 != nil {
		fail("encode "+ext, err2)
	}
	dest := *output
	if dest == "" {
		dest = strings.TrimSuffix(inputPath, ".compress.topo.bin") + ext
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		fail("write output", err)
	}
	fmt.Fprintf(os.Stderr, "input:  bytes=%d timezones=%d\n", len(raw), len(input.Timezones))
	if *profile == "m" {
		fmt.Fprintf(os.Stderr, "output: bytes=%d profile=m fuzzy=%v\n", len(data), opts.Preindex != nil)
	} else {
		fmt.Fprintf(os.Stderr, "output: bytes=%d profile=e chunk=%d shortcut=%v fuzzy=%v\n",
			len(data), effectiveChunk(*chunk), *allowShortcut, opts.Preindex != nil)
	}
	fmt.Println(dest)
}

func effectiveChunk(chunk int) int {
	if chunk == 0 {
		return 64
	}
	return chunk
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "topo2embed: %s: %v\n", action, err)
	os.Exit(1)
}
