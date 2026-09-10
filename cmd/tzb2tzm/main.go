// Command tzb2tzm converts an E-profile .tzb file into the M-profile .tzm
// memory image via the pb-free transcoder; the output is byte-identical to
// encoding the .tzm from the source dataset directly.
//
// Intended as a build- or deploy-time step (for example a Dockerfile RUN):
// ship only the compact .tzb, produce the .tzm where it is consumed, then
// mmap it so the polygon storage stays in the page cache and is shared
// across processes — the pattern for memory-constrained or cgroup-quota
// deployments. Never distribute the .tzm itself.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/v2/internal/embedbin"
)

func main() {
	output := flag.String("o", "", "output path (default: input name with .tzm)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: tzb2tzm [-o output] input.tzb")
		os.Exit(2)
	}
	inputPath := flag.Arg(0)
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		fail("read input", err)
	}
	reader, err := embedbin.Open(raw)
	if err != nil {
		fail("open .tzb", err)
	}
	data, err := reader.TranscodeM()
	if err != nil {
		fail("transcode", err)
	}
	if _, err := embedbin.Open(data); err != nil {
		fail("validate output", err)
	}
	dest := *output
	if dest == "" {
		dest = strings.TrimSuffix(inputPath, ".tzb") + ".tzm"
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		fail("write output", err)
	}
	fmt.Fprintf(os.Stderr, "input:  bytes=%d timezones=%d fuzzy=%v\n",
		len(raw), reader.TimezoneCount(), reader.HasFuzzy())
	fmt.Fprintf(os.Stderr, "output: bytes=%d profile=m\n", len(data))
	fmt.Println(dest)
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "tzb2tzm: %s: %v\n", action, err)
	os.Exit(1)
}
