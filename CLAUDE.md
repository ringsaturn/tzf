# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TZF is a high-performance timezone finder library for Go that determines the timezone for given latitude and longitude coordinates. The project is designed for geospatial services like weather forecast APIs where fast timezone lookups are critical.

## Development Commands

```bash
make fmt        # gofmt all packages
make test       # golangci-lint + go test -v with coverage
make cover      # test + open coverage HTML
make bench      # run benchmarks
make pb         # regenerate protobuf Go bindings via buf
```

Run a single test:
```bash
go test -v -run TestName ./internal/topology/...
```

Key tools required: `golangci-lint`, `buf` (proto generation), `go-licenses`.

## Core Architecture

### Finder Types (`tzf.go`, `tzf_fuzzy.go`, `tzf_default_finder.go`)

Three finder implementations share the interface in `f.go`:

| Finder | Mechanism | Memory | Speed |
|--------|-----------|--------|-------|
| `Finder` | Polygon point-in-polygon + RTree | ~100MB lite / ~1GB full | moderate |
| `FuzzyFinder` | Pre-indexed map tiles | ~1.78MB | fastest |
| `DefaultFinder` | FuzzyFinder first, Finder fallback (±0.02°) | ~60MB after GC | fast |

### Data Pipeline

```
Raw GeoJSON (timezone-boundary-builder)
  └─ cmd/geojson2tzpb       → Timezones .bin         (~92MB, full precision)
       └─ cmd/reducetzpb    → .topology.bin           (~13MB, topology-aware D-P simplified)
            └─ cmd/deduplicatetzpb → .topo.bin        (TopoTimezones, shared-edge dedup)
                 └─ cmd/compresstopotzpb → .compress.topo.bin  (polyline-encoded coords)
       └─ cmd/compresstzpb  → .compress.bin           (~4.5MB, polyline coords)
       └─ cmd/preindextzpb  → .preindex.bin           (~2MB, tile pre-index)
```

Data files live in `github.com/ringsaturn/tzf-rel` (full) and `tzf-rel-lite` (lite, embedded via Go modules). Versions must match between Finder and FuzzyFinder.

### Protobuf Schema (`pb/tzf/v1/tzinfo.proto`)

Key message families:
- **`Timezones` / `Timezone` / `Polygon` / `Point`** — flat polygon format used by all finders
- **`CompressedTimezones`** — polyline-encoded coordinates (existing lite format)
- **`TopoTimezones` / `SharedEdge` / `RingSegment`** — shared-edge deduplication format; rings are sequences of segment references pointing into a global edge library
- **`CompressedTopoTimezones`** — `TopoTimezones` with polyline-encoded point sequences

Regenerate after proto changes: `make pb` (runs `buf generate`).

### `internal/topology` Package

The topology-aware simplification engine. Key files:

- **`topology.go`** — `DoWithStats(input, epsilon)` is the main entry point. Pipeline: normalize coordinates → fix winding order → remove zero-length edges → snap T-junction vertices → collect rings + edge/vertex indices → mark shared edges → mark fixed vertices → simplify each ring using Douglas-Peucker with a shared-segment cache → validate fallbacks.
- **`dedup.go`** — `BuildTopoTimezones` / `DecodeTopoTimezones`: converts flat `Timezones` into the `TopoTimezones` shared-edge format. Uses `markFixedVerticesForDedup` (stricter than the simplification variant) to split rings at shared/non-shared boundaries.
- **`validate.go`** — `Validate` / `MustValidateForReduction`: geometry checks (winding, closure, self-intersection, zero-length edges). `ReductionValidateOptions` disables same-direction shared edge checks for disputed-territory data.

**Critical invariants:**
- `normalizeWindings` must be called *before* `snapVertices` and topology analysis so adjacent rings traverse shared boundaries in opposite directions.
- `removeZeroLengthEdges` must run before `collectRings`; source data can contain rings where adjacent (or wrap-around) points are identical, which breaks shared-edge detection.
- `markFixedVertices` (simplification) only fixes 3+-ring junction vertices. `markFixedVerticesForDedup` also fixes shared↔non-shared transitions.
- Enclave rings (hole whose shape = another timezone's exterior) are detected by `isEntirelyShared`; both partner rings rotate to the lexicographically smallest vertex (`findCanonicalStart`) before entering the shared-segment cache, ensuring identical simplification results.
- Simplified rings failing `hasSelfIntersection` (≤100 pts, O(n²)) or `ringHasZeroLengthEdge` fall back to the original unmodified input ring via `getOriginalRing`.

### `reduce` Package

- `reduce.go`: `DoTopologyAwareWithStats` wraps `topology.DoWithStats` + `MustValidateForReduction`.
- `compress.go`: polyline encode/decode for `Timezones` → `CompressedTimezones`.
- `compress_topo.go`: polyline encode/decode for `TopoTimezones` → `CompressedTopoTimezones`; edge ID references pass through unchanged.

### CLI Tools (`cmd/`)

| Tool | Input | Output | Purpose |
|------|-------|--------|---------|
| `reducetzpb` | `.bin` (Timezones) | `.topology.bin` | Topology-aware D-P simplification |
| `deduplicatetzpb` | `.bin` (Timezones) | `.topo.bin` | Shared-edge deduplication |
| `compresstopotzpb` | `.topo.bin` | `.compress.topo.bin` | Polyline compress topo format |
| `compresstzpb` | `.bin` | `.compress.bin` | Polyline compress flat format |
| `preindextzpb` | `.bin` | `.preindex.bin` | Tile pre-indexing |
| `geojson2tzpb` | GeoJSON | `.bin` | GeoJSON → protobuf |

## Known Data Quirks

- **Antimeridian (-180°/+180°):** `normalizeLng` (-180→+180) is used only for topology key matching (`newPointKey`/`newEdgeKey`), never to mutate geometric coordinates. Mixing signs in the same ring corrupts `signedArea` and winding detection.
- **Disputed territories:** Some timezone pairs share edges in the same direction (e.g. Israel/Palestine). `ReductionValidateOptions` disables `CheckSameDirectionSharedEdges` to allow this.
- **Source data zero-length edges:** Certain rings (e.g. Macau border-crossing building outline) have duplicate adjacent vertices in the upstream data; these must be stripped before topology analysis.
- **Fallback rings:** ~200 rings simplify to degenerate or self-intersecting shapes and revert to original geometry. They are mostly tiny island outlines and building-footprint enclaves.
