// Command borderchange measures boundary movement caused by topology-aware
// simplification.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	border "github.com/ringsaturn/tzf/internal/borderchange"
	"github.com/ringsaturn/tzf/internal/topology"
	"google.golang.org/protobuf/proto"
)

func main() {
	epsilon := flag.Float64("epsilon", 0.001, "Douglas-Peucker tolerance in degrees")
	tolerance := flag.Float64("certification-tolerance-m", 0.5, "maximum numerical certification gap in meters")
	topPairs := flag.Int("top-pairs", 20, "number of timezone pairs to print")
	workers := flag.Int("workers", 0, "analysis workers, 0 means GOMAXPROCS")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/cmd/borderchange -epsilon 0.001 INPUT.bin")
		os.Exit(2)
	}

	started := time.Now()
	raw, err := os.ReadFile(flag.Arg(0))
	must(err)
	input := &pb.Timezones{}
	must(proto.Unmarshal(raw, input))
	raw = nil
	version := input.Version

	log.Printf("borderchange: dataset loaded in %s", time.Since(started).Round(time.Millisecond))
	simplifyStarted := time.Now()
	simplified, baseline, stats := topology.DoWithStatsAndBaseline(input, *epsilon)
	log.Printf("borderchange: simplification finished in %s", time.Since(simplifyStarted).Round(time.Millisecond))
	// Drop the raw dataset before analysis; only baseline and simplified are
	// needed from here on, so the GC can reclaim the largest copy.
	input = nil
	_ = raw
	report, err := border.Analyze(baseline, simplified, border.Options{CertificationToleranceM: *tolerance, Workers: *workers, Progress: true})
	must(err)
	printMarkdown(report, flag.Arg(0), version, *epsilon, stats, *topPairs, time.Since(started))
}

func printMarkdown(report *border.Report, inputPath, version string, epsilon float64, stats topology.Stats, topPairs int, elapsed time.Duration) {
	fmt.Println("## Evaluation result")
	fmt.Println()
	fmt.Printf("- Input: `%s`\n", inputPath)
	fmt.Printf("- Dataset version: `%s`\n", version)
	fmt.Printf("- Douglas-Peucker epsilon: `%.6f degrees`\n", epsilon)
	fmt.Printf("- Unique source arcs: `%d`\n", report.UniqueArcs)
	fmt.Printf("- Changed arcs: `%d`\n", report.ChangedArcs)
	fmt.Printf("- Original unique boundary length: `%.3f km`\n", report.OriginalLengthKM)
	fmt.Printf("- Changed boundary length: `%.3f km`\n", report.ChangedLengthKM)
	fmt.Printf("- Error strip area: `%.6f km2`\n", report.ErrorAreaKM2)
	fmt.Printf("- Maximum single strip area: `%.6f km2`\n", report.MaxStripAreaKM2)
	fmt.Printf("- Runtime: `%s`\n", elapsed.Round(time.Millisecond))
	fmt.Println()

	fmt.Println("### Boundary displacement")
	fmt.Println()
	fmt.Println("| Metric | Distance |")
	fmt.Println("|---|---:|")
	for _, q := range []struct {
		name  string
		value float64
	}{{"p50", .5}, {"p95", .95}, {"p99", .99}, {"p99.9", .999}} {
		fmt.Printf("| Length-weighted %s | %.3f m |\n", q.name, border.Quantile(&report.LengthDistances, q.value))
	}
	fmt.Printf("| Certified maximum | %.3f m |\n", report.MaxCertifiedM)
	fmt.Printf("| Certification upper tolerance | +%.3f m |\n", report.CertificationToleranceM)
	fmt.Println()
	fmt.Printf("Maximum location: `%.7f, %.7f`, timezone pair: `%s` / `%s`.\n",
		report.MaxLocation.Lng, report.MaxLocation.Lat, report.MaxLocation.TimezoneA, report.MaxLocation.TimezoneB)
	fmt.Println()

	fmt.Println("| Threshold | Boundary length above threshold |")
	fmt.Println("|---|---:|")
	for _, threshold := range []float64{10, 50, 100, 500} {
		fmt.Printf("| %.0f m | %.6f%% |\n", threshold, 100*border.ProportionAbove(&report.LengthDistances, threshold))
	}
	fmt.Println()

	fmt.Println("### Error strip width")
	fmt.Println()
	fmt.Println("| Metric | Width |")
	fmt.Println("|---|---:|")
	for _, q := range []struct {
		name  string
		value float64
	}{{"p50", .5}, {"p95", .95}, {"p99", .99}, {"p99.9", .999}} {
		fmt.Printf("| Area-weighted %s | %.3f m |\n", q.name, border.Quantile(&report.AreaWidths, q.value))
	}
	fmt.Println()
	for _, threshold := range []float64{10, 50, 100} {
		fmt.Printf("- Error area within %.0f m of source boundary: `%.6f%%`\n", threshold, 100*(1-border.ProportionAbove(&report.AreaWidths, threshold)))
	}
	fmt.Println()

	fmt.Println("### Largest timezone-pair error areas")
	fmt.Println()
	fmt.Println("| Timezone A | Timezone B | Area |")
	fmt.Println("|---|---|---:|")
	if topPairs > len(report.PairAreas) {
		topPairs = len(report.PairAreas)
	}
	for _, pair := range report.PairAreas[:topPairs] {
		fmt.Printf("| %s | %s | %.6f km2 |\n", pair.TimezoneA, pair.TimezoneB, pair.AreaKM2)
	}
	fmt.Println()
	fmt.Println("### Simplifier statistics")
	fmt.Println()
	fmt.Println("```text")
	fmt.Println(stats.String())
	fmt.Println("```")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
