// Command borderchange measures boundary movement caused by topology-aware
// simplification.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ringsaturn/tzf/convert"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	border "github.com/ringsaturn/tzf/internal/borderchange"
	"github.com/ringsaturn/tzf/internal/topology"
	"github.com/ringsaturn/tzf/reduce"
	"google.golang.org/protobuf/proto"
)

type comparisonDetails struct {
	sourcePath       string
	candidatePath    string
	sourceVersion    string
	candidateVersion string
	epsilon          *float64
	stats            *topology.Stats
	sourcePoints     int
	candidatePoints  int
}

func main() {
	epsilon := flag.Float64("epsilon", 0.001, "Douglas-Peucker tolerance in degrees")
	tolerance := flag.Float64("certification-tolerance-m", 0.5, "maximum numerical certification gap in meters")
	topPairs := flag.Int("top-pairs", 20, "number of timezone pairs to print")
	workers := flag.Int("workers", 0, "analysis workers, 0 means GOMAXPROCS")
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		fmt.Fprintln(os.Stderr, "usage: borderchange [flags] SOURCE [CANDIDATE]")
		fmt.Fprintln(os.Stderr, "  one input: generate the candidate with -epsilon")
		fmt.Fprintln(os.Stderr, "  two inputs: compare source GeoJSON or protobuf with a supplied candidate")
		os.Exit(2)
	}

	started := time.Now()
	source, err := loadDataset(flag.Arg(0))
	must(err)
	details := comparisonDetails{
		sourcePath:    flag.Arg(0),
		sourceVersion: source.Version,
	}

	var baseline, candidate *pb.Timezones
	if flag.NArg() == 1 {
		log.Printf("borderchange: dataset loaded in %s", time.Since(started).Round(time.Millisecond))
		simplifyStarted := time.Now()
		var stats topology.Stats
		candidate, baseline, stats = topology.DoWithStatsAndBaseline(source, *epsilon)
		log.Printf("borderchange: simplification finished in %s", time.Since(simplifyStarted).Round(time.Millisecond))
		details.candidatePath = "generated with topology-aware simplification"
		details.candidateVersion = candidate.Version
		details.epsilon = epsilon
		details.stats = &stats
	} else {
		candidate, err = loadDataset(flag.Arg(1))
		must(err)
		if source.Version != "" && candidate.Version != "" && source.Version != candidate.Version {
			must(fmt.Errorf("dataset versions differ: %q vs %q", source.Version, candidate.Version))
		}
		normalizeStarted := time.Now()
		baseline = topology.PrepareBaseline(source)
		log.Printf("borderchange: datasets loaded and source normalized in %s", time.Since(normalizeStarted).Round(time.Millisecond))
		details.candidatePath = flag.Arg(1)
		details.candidateVersion = candidate.Version
	}
	details.sourcePoints = countPoints(baseline)
	details.candidatePoints = countPoints(candidate)

	// Drop the source copy before analysis. The normalized baseline and candidate
	// contain all geometry needed from this point onward.
	source = nil
	report, err := border.Analyze(baseline, candidate, border.Options{CertificationToleranceM: *tolerance, Workers: *workers, Progress: true})
	must(err)
	printMarkdown(report, details, *topPairs, time.Since(started))
}

func loadDataset(path string) (*pb.Timezones, error) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return loadGeoJSONZip(path)
	case strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".geojson"):
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return loadGeoJSON(raw)
	case strings.HasSuffix(lower, ".compress.topo.bin"):
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		compressed := &pb.CompressedTopoTimezones{}
		if err := proto.Unmarshal(raw, compressed); err != nil {
			return nil, fmt.Errorf("decode compressed topology %q: %w", path, err)
		}
		return topology.DecodeTopoTimezones(reduce.DecompressTopoTimezones(compressed)), nil
	case strings.HasSuffix(lower, ".topo.bin"):
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		topo := &pb.TopoTimezones{}
		if err := proto.Unmarshal(raw, topo); err != nil {
			return nil, fmt.Errorf("decode topology %q: %w", path, err)
		}
		return topology.DecodeTopoTimezones(topo), nil
	default:
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		flat := &pb.Timezones{}
		if err := proto.Unmarshal(raw, flat); err != nil {
			return nil, fmt.Errorf("decode timezones %q: %w", path, err)
		}
		return flat, nil
	}
}

func loadGeoJSONZip(path string) (*pb.Timezones, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	var selected *zip.File
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !file.FileInfo().IsDir() && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".geojson")) {
			if selected != nil {
				return nil, fmt.Errorf("zip %q contains multiple GeoJSON candidates: %q and %q", path, selected.Name, file.Name)
			}
			selected = file
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("zip %q contains no JSON file", path)
	}

	file, err := selected.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return loadGeoJSON(raw)
}

func loadGeoJSON(raw []byte) (*pb.Timezones, error) {
	boundary := &convert.BoundaryFile{}
	if err := json.Unmarshal(raw, boundary); err != nil {
		return nil, fmt.Errorf("decode GeoJSON: %w", err)
	}
	return convert.DoWithVersion(boundary, "")
}

func countPoints(input *pb.Timezones) int {
	var points int
	for _, timezone := range input.Timezones {
		for _, polygon := range timezone.Polygons {
			points += len(polygon.Points)
			for _, hole := range polygon.Holes {
				points += len(hole.Points)
			}
		}
	}
	return points
}

func printMarkdown(report *border.Report, details comparisonDetails, topPairs int, elapsed time.Duration) {
	fmt.Println("## Evaluation result")
	fmt.Println()
	fmt.Printf("- Source input: `%s`\n", details.sourcePath)
	fmt.Printf("- Candidate input: `%s`\n", details.candidatePath)
	fmt.Printf("- Source dataset version: `%s`\n", displayVersion(details.sourceVersion))
	fmt.Printf("- Candidate dataset version: `%s`\n", displayVersion(details.candidateVersion))
	if details.epsilon != nil {
		fmt.Printf("- Douglas-Peucker epsilon: `%.6f degrees`\n", *details.epsilon)
	}
	fmt.Printf("- Source points after topology normalization: `%d`\n", details.sourcePoints)
	fmt.Printf("- Candidate points: `%d`\n", details.candidatePoints)
	if details.sourcePoints > 0 {
		fmt.Printf("- Point reduction: `%.3f%%`\n", 100*(1-float64(details.candidatePoints)/float64(details.sourcePoints)))
	}
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
	if details.stats != nil {
		fmt.Println("### Simplifier statistics")
		fmt.Println()
		fmt.Println("```text")
		fmt.Println(details.stats.String())
		fmt.Println("```")
	}
}

func displayVersion(version string) string {
	if version == "" {
		return "not encoded"
	}
	return version
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
