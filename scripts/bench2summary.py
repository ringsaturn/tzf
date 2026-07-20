#!/usr/bin/env python3
"""Parse go benchmark + memory output and emit a GitHub Actions Step Summary markdown.

Output format mirrors tzf-rs benchmark_summary.md:
  Target | Dataset | Scenario | Median (µs) | Throughput (ops/s) | Memory (MiB)
"""

import re
import sys


# ---------------------------------------------------------------------------
# Parsing
# ---------------------------------------------------------------------------

def parse_bench(text):
    """Return list of dicts from -benchmem + hrtesting output."""
    pattern = re.compile(
        r"^(Benchmark\S+?)-\d+\s+"           # name + cpu count
        r"(\d+)\s+"                            # iterations
        r"([\d.]+)\s+ns/op"                    # mean ns/op
        r"(?:\s+([\d.]+)\s+ns/p50)?"           # hrtesting p50 (median)
        r"(?:\s+[\d.]+\s+ns/p90)?"             # p90 (skipped)
        r"(?:\s+([\d.]+)\s+ns/p99)?"           # hrtesting p99
        r"(?:\s+([\d.]+)\s+B/op)?"             # B/op
        r"(?:\s+(\d+)\s+allocs/op)?",          # allocs/op
        re.MULTILINE,
    )
    results = []
    for m in pattern.finditer(text):
        ns_op = float(m.group(3))
        p50 = float(m.group(4)) if m.group(4) else ns_op
        # hrtesting rounds to µs resolution; fall back to ns_op when p50=0
        if p50 == 0:
            p50 = ns_op
        p99 = float(m.group(5)) if m.group(5) else ns_op
        if p99 == 0:
            p99 = ns_op
        results.append({
            "name": m.group(1),
            "iters": int(m.group(2)),
            "ns_op": ns_op,
            "ns_p50": p50,
            "ns_p99": p99,
            "b_op": float(m.group(6)) if m.group(6) else 0.0,
            "allocs_op": int(m.group(7)) if m.group(7) else 0,
        })
    return results


def parse_memory(text):
    """Return dict {finder_name: mib} from bench-memory output."""
    pattern = re.compile(r"^MEMORY\t(\S+)\t([-\d.]+)", re.MULTILINE)
    return {m.group(1): float(m.group(2)) for m in pattern.finditer(text)}


# ---------------------------------------------------------------------------
# Benchmark → table-row metadata
# ---------------------------------------------------------------------------

# (target_label, dataset_label, memory_key)
def bench_meta(bench_name):
    name = re.sub(r"^Benchmark", "", bench_name)

    # --- Target ---
    if name.startswith("NewFinderFromTZBReaderAt"):
        target = "TZBFinder ReaderAt"
        dataset = "topology-simplified .tzb"
        mem_key = "TZBFinderReaderAt"
    elif name.startswith("NewFinderFromTZB"):
        target = "TZBFinder"
        dataset = "topology-simplified .tzb"
        mem_key = "TZBFinder"
    elif name.startswith("TZBFinderReaderAt"):
        target = "TZBFinder ReaderAt"
        dataset = "topology-simplified .tzb"
        mem_key = "TZBFinderReaderAt"
    elif name.startswith("TZBFinder"):
        target = "TZBFinder"
        dataset = "topology-simplified .tzb"
        mem_key = "TZBFinder"
    elif name.startswith("DefaultFinder"):
        target = "DefaultFinder"
        dataset = "topology-simplified + preindex"
        mem_key = "DefaultFinder"
    elif name.startswith("FuzzyFinder"):
        target = "FuzzyFinder"
        dataset = "preindex"
        mem_key = "FuzzyFinder"
    elif "FullFinderWithoutPreindex" in name:
        target = "Finder"
        dataset = "full-precision"
        mem_key = "FullFinderWithoutPreindex"
    elif "FullFinder" in name:
        target = "FullFinder"
        dataset = "full-precision + preindex"
        mem_key = "FullFinder"
    elif "GridIndex_WithGrid" in name:
        target = "Finder"
        dataset = "topology-simplified + GridIndex"
        mem_key = "Finder"
    elif "GridIndex_NoGrid" in name:
        target = "Finder"
        dataset = "topology-simplified (no GridIndex)"
        mem_key = "FinderNoGrid"
    else:
        # Plain GetTimezoneName* — basic Finder
        target = "Finder"
        dataset = "topology-simplified"
        mem_key = "Finder"

    # --- Scenario ---
    if name.startswith("NewFinderFromTZB"):
        return target, dataset, mem_key, "construction"

    if "GetTimezoneNames" in name:
        method = "GetTimezoneNames"
    else:
        method = "GetTimezoneName"

    if "AtEdge" in name:
        location = "edge case"
    else:
        location = "random world cities"

    scenario = f"{location} · {method}"

    return target, dataset, mem_key, scenario


# ---------------------------------------------------------------------------
# Formatting helpers
# ---------------------------------------------------------------------------

def fmt_median(ns):
    return f"{ns:.1f}"


def fmt_throughput(ns_op):
    if ns_op <= 0:
        return "N/A"
    ops = 1e9 / ns_op
    return f"{ops/1_000:.1f}K"


def fmt_mib(mib):
    if mib < 0:
        return "N/A"
    return f"{mib:.2f}"


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    bench_file = sys.argv[1] if len(sys.argv) > 1 else "benchmark_result.txt"
    memory_file = sys.argv[2] if len(sys.argv) > 2 else "memory_result.txt"

    bench_text = ""
    memory_text = ""
    try:
        bench_text = open(bench_file).read()
    except FileNotFoundError:
        pass
    try:
        memory_text = open(memory_file).read()
    except FileNotFoundError:
        pass

    benchmarks = parse_bench(bench_text)
    memory = parse_memory(memory_text)

    # Build rows
    rows = []
    for b in benchmarks:
        meta = bench_meta(b["name"])
        if meta is None:
            continue
        target, dataset, mem_key, scenario = meta
        mib = memory.get(mem_key, -1.0)
        rows.append({
            "target": target,
            "dataset": dataset,
            "scenario": scenario,
            "median_us": fmt_median(b["ns_p50"]),
            "p99_us": fmt_median(b["ns_p99"]),
            "throughput": fmt_throughput(b["ns_op"]),
            "memory_mib": fmt_mib(mib),
        })

    if not rows:
        print("No benchmark data found.")
        return

    rows.sort(key=lambda r: (r["scenario"], r["target"]))

    lines = ["# Benchmark Summary\n"]
    lines.append("| Target | Dataset | Scenario | Median (ns) | p99 (ns) | Approx throughput (ops/s) | Memory (MiB) |")
    lines.append("| --- | --- | --- | ---: | ---: | ---: | ---: |")
    for r in rows:
        lines.append(
            f"| {r['target']} | {r['dataset']} | {r['scenario']}"
            f" | {r['median_us']} | {r['p99_us']} | {r['throughput']} | {r['memory_mib']} |"
        )

    print("\n".join(lines))


if __name__ == "__main__":
    main()
