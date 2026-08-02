// Command topo2embed converts CompressedTopoTimezones protobuf data to .tzb.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"github.com/ringsaturn/tzf/internal/embedbin"
	"google.golang.org/protobuf/proto"
)

func main() {
	output := flag.String("o", "", "output .tzb path")
	chunk := flag.Int("chunk", 0, "target points per chunk (default 256)")
	allowShortcut := flag.Bool("allow-shortcut", false, "enable the single-candidate GRID shortcut")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: topo2embed [-o output.tzb] [-chunk 256] [-allow-shortcut] input.compress.topo.bin")
		os.Exit(2)
	}
	inputPath := flag.Arg(0)
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		fail("read input", err)
	}
	var input pb.CompressedTopoTimezones
	if err := proto.Unmarshal(raw, &input); err != nil {
		fail("decode CompressedTopoTimezones", err)
	}
	data, err := embedbin.Encode(&input, embedbin.EncodeOptions{
		ChunkTarget: *chunk, AllowShortcut: *allowShortcut,
	})
	if err != nil {
		fail("encode .tzb", err)
	}
	dest := *output
	if dest == "" {
		dest = strings.TrimSuffix(inputPath, ".compress.topo.bin") + ".tzb"
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		fail("write output", err)
	}
	fmt.Fprintf(os.Stderr, "input:  bytes=%d timezones=%d\n", len(raw), len(input.Timezones))
	fmt.Fprintf(os.Stderr, "output: bytes=%d chunk=%d shortcut=%v\n", len(data), effectiveChunk(*chunk), *allowShortcut)
	fmt.Println(dest)
}

func effectiveChunk(chunk int) int {
	if chunk == 0 {
		return 256
	}
	return chunk
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "topo2embed: %s: %v\n", action, err)
	os.Exit(1)
}
