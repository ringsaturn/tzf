# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TZF is a high-performance timezone finder library for Go that determines the timezone for given latitude and longitude coordinates. The project is designed for geospatial services like weather forecast APIs where fast timezone lookups are critical.

## Repository Layout (single module)

`go.mod` at the repo root declares `github.com/ringsaturn/tzf/v2` — the major
version bump happens in place, with no `v2/` directory (maintainer decision
2026-08-28; the v1 line lives on its own branch and tags). One module holds
both the runtime and the pipeline:

- Public API: the root package (five constructors + `F`/`GeoJSONer`) and
  `x/` (experimental, exempt from semver). Nothing else is exported.
- Runtime internals: `internal/{embedbin,geom,inplace,tzerr}`.
- Pipeline (all internal — the v1-era public packages `reduce`, `preindex`,
  `convert.Do` are now `internal/{reduce,preindex,convert}`; external users
  drive the pipeline through the CLIs): `internal/model` (native mirrors of
  the retired tzf.v1 schema + gob codec), `internal/{topology,gridindex,
  polyline,polyf,preindexexclude,borderchange,maps}`, `internal/embedenc`
  (embedbin encoder + `Verify`/`VerifyM`), `internal/pbref` (reference
  finders for parity), `internal/parity` (source-parity tests, env-gated).
- CLIs: `cmd/` (user-facing `tzf`, `full-demo`, `debug` and the pipeline
  stages `geojson2tzpb` … `topo2embed`, `tzb2tzm`), harnesses under
  `internal/cmd/` (`embedcompare`, `bench-memory`, …).

There is **no protobuf and no pb parser anywhere**; pipeline intermediates
are gob (`internal/model`, `TZFGOB1\n` magic) and **never distributed** —
`tzf-dist` ships only `.tzb`/`.tzm`. The `internal/embedbin` ↔
`internal/embedenc` seam is `internal/embedbin/encseam.go` (exported for the
encoder package, still module-internal).

**Bootstrap**: the runtime embeds come from the published
`github.com/ringsaturn/tzf-dist` module (first v2 release
`v0.0.2026-c-tzb1`), so `go test ./...` works from a plain clone. The
pipeline and parity tooling still need the sibling `../tzf-dist` checkout:
`./scripts/build-tzf-dist-dev.sh` downloads the upstream raw GeoJSON, runs
the full pipeline (gob intermediates stay in gitignored `tmp/tzf-dist-dev`
as parity fixtures for `make parity` and the env-gated parity tests), and
installs the three artifacts over the placeholders in `../tzf-dist` for
tzf-dist's own embed tests — as uncommitted worktree state only: real data
is never committed on tzf-dist main (it ships via tags on the data
branch); restore the placeholders with
`git -C ../tzf-dist checkout -- lite.tzb lite.tzm full.tzb` when done. CI
checks both repos out side by side for the same reason.

## Development Commands

```bash
make fmt          # gofmt
make test         # golangci-lint + go test -race (parity env wired to tmp/tzf-dist-dev)
make bench        # query benchmarks over the embedded artifacts
make bench-memory # retained-heap per finder
make parity       # full embedcompare gate (deep E+M verify + dense + boundary)
```

Run a single test:
```bash
go test -v -run TestName ./
TZF_PARITY_TOPO=... TZF_PARITY_PREINDEX=... go test -v -run TestName ./internal/parity/
```

Key tools required: `golangci-lint`.

## v2 Public Surface

Exactly five constructors, all returning `F`; no `Option`, no exported
concrete finder types (spec §2):

| Constructor | Mechanism | Memory | Query |
|---|---|---|---|
| `NewDefaultFinder()` | lite `.tzm` memory image: FUZZY fast path + in-place polygon view aliasing the embedded bytes | ~10MB heap + rodata | ~300ns |
| `NewEmbeddedFinder()` | lite `.tzb` queried in place | <1KB heap | p50 ~0.5µs, ~6µs PIP |
| `NewFullFinder()` | full `.tzb` expanded + FUZZY fast path | ~145MB | ~300ns |
| `NewFinderFromTZB(data)` | always expanded (+FUZZY fast path when present); data released | | |
| `NewFinderFromTZM(data)` | always aliases in place (+FUZZY when present); data retained | | |

- `GetTimezoneName` is fuzzy-first when the file carries FUZZY;
  `GetTimezoneNames` is polygon-only in every finder (the polygon-exact
  escape hatch).
- In-place queries over caller-owned bytes: wrap in `bytes.NewReader` and use
  `x.NewFinderFromTZBReaderAt` (the `x` package is exempt from the semver
  promise — a minor bump may break it).
- `GeoJSONer` (`GetTZGeoJSON`/`GetGeoJSON`) is asserted, not part of `F`:
  `f.(tzf.GeoJSONer)`. Every constructor's result satisfies it. Both
  methods return serialized GeoJSON bytes — the boundary-file types are
  internal (`internal/convert`), not public API (decision 2026-08-28).
- Internally: unexported `finder` (storage-generic `finderCore` →
  `finderImpl[int32]`), `fuzzyIndex` (FUZZY hash maps), `defaultFinder`
  (fuzzy + polygon composition), `inplace.Finder`. The reference
  implementation for parity lives in `internal/pbref`.

## v1 Architecture (historical — the v1 line lives on its own branch)

### Finder Types (`tzf.go`, `tzf_fuzzy.go`, `tzf_default_finder.go`)

Three finder implementations share the interface in `f.go`:

| Finder | Mechanism | Memory | Speed |
|--------|-----------|--------|-------|
| `Finder` | Polygon point-in-polygon + grid index | ~30MB lite / ~150MB full | moderate |
| `FuzzyFinder` | Pre-indexed map tiles | ~2.4MB | fastest |
| `DefaultFinder` | FuzzyFinder first, Finder fallback (±0.02°) | ~32MB | fast |

## v2 Runtime Internals

### Embedded Binary Format `.tzb` / `.tzm` (`internal/embedbin`, `tzf_tzb.go`, `tzf_tzm.go`)

Sectioned little-endian container (format 1.1): header with a profile byte,
CRC32 footer, optional dense `GRID` and `FUZZY` (type 10,
preindex tiles as one sorted TileID array) sections. Two profiles share the
container: **E** (`.tzb`, profile 0) is the chunked varint layout above;
**M** (`.tzm`, profile 1) replaces the chunk machinery with `FLATRINGDIR`
(type 13, 24-byte records) over one contiguous `FLATPOINTS` (type 12,
8-byte-aligned `(i32,i32)` pairs, junction dedup pre-expanded) so the file
*is* the query-time structure; `YSTRIPES` (type 14) is assigned but not
emitted. Mandatory sections are per-profile and cross-profile section types
are rejected. Built by `cmd/topo2embed` (`-profile e|m`, `-preindex`
embeds FUZZY); parity harness `internal/cmd/embedcompare` (`-tzm`
adds the M leg; requires `-preindex` on FUZZY-carrying files so the composed
finders can be checked; `embedenc.VerifyM` derives expected flat rings from
the source pb independently of the encoder; `pbref` is the pb-composed query
reference).

Load paths (all protobuf-free at runtime):

- `NewEmbeddedFinder` / `x.NewFinderFromTZBReaderAt` — in-place queries over
  the compressed file, <1KB heap, ~6µs/query PIP. When the file carries FUZZY,
  `GetTimezoneName` probes it first in place (p50 ~0.5µs);
  `GetTimezoneNames` stays polygon-only. Both build
  `internal/inplace.Finder`; they differ only in how the `embedbin.Reader`
  was opened. The byte-backed FUZZY probe is lock-free and zero-alloc; the
  ReaderAt backend routes those reads through the locked decode workspace
  (still zero-alloc — stack buffers would escape through the interface).
- `NewFinderFromTZB` — one-pass expansion into `finderImpl[int32]` plus the
  FUZZY hash maps when present (fuzzy-first composition), junction-duplicate
  vertices dropped. Item assembly is parallelized (shared `assembleI32Items`
  with the .tzm loader): ~5× faster load than the old pb path (lite 18.6 ms
  vs 96 ms; full 78.5 ms vs 288 ms).
- `embedbin.(*Reader).TranscodeM` — pb-free `.tzb` → `.tzm` conversion
  (expand rings, copy profile-shared sections byte-wise); output is
  byte-identical to `embedenc.EncodeM` over the same source. Ship the compact
  `.tzb`, build the memory image locally — never distribute `.tzm`
  (`cmd/tzb2tzm`).
- `NewFinderFromTZM` — M-profile memory image: ring slices alias FLATPOINTS
  in place (little-endian hosts; explicit-LE copy fallback elsewhere), GRID
  queried in place via `finderImpl.dense`, YStripes rebuilt at open in
  parallel (~5ms lite), FUZZY composed when present. Query ~312ns; heap
  beyond the retained mapping ~10MB (stripes + items). The source bytes must
  stay live and unmodified.

### GeoJSON Export (`f.go`, `internal/inplace`, `internal/convert`)

`GeoJSONer` (`GetTZGeoJSON(name)` / `GetGeoJSON()`) is a separate exported
interface, deliberately not part of `F` — constructors return `F`, so callers
assert the behavior: `finder.(tzf.GeoJSONer).GetGeoJSON()`. Every finder here
satisfies it (compile-time assertions live next to each type). The expanded
and `.tzm` finders export polygons they already hold; the in-place finder
decodes on demand through `embedbin.(*Reader).ExpandTimezone`, which pulls
only the shared-edge groups the requested timezone's rings reference. Output
is byte-identical between the two paths (`TestInPlaceGeoJSONMatchesExpanded`
checks the whole world). `GetGeoJSON` has no error return, so the in-place
implementation omits an undecodable timezone the way `GetTimezoneName`
treats a lazy read error as no match.

### `x` Package

`github.com/ringsaturn/tzf/v2/x` holds experimental surface, exempt from the
module's semver promise: **a minor-version bump may break it**
(golang.org/x-style, stated in the package doc). Today it holds exactly
`NewFinderFromTZBReaderAt`. It imports the v2 root package plus
`internal/embedbin` and `internal/inplace`; the v2 root package must never
import `x`. It is also the route for in-place queries over caller-owned
bytes (`bytes.NewReader`) — v2 has no `InPlace()` option (W4).

Boundary semantics everywhere match post-#216 `ContainsPointAllowOnEdge`:
a point on a shared border belongs to every touching polygon (exterior rings
allow on-edge, hole rings do not).

## Pipeline Internals

**Protobuf-free** (maintainer decision 2026-08-28): the pipeline's data
structures live in `internal/model` — native Go mirrors of the retired
`tzf.v1` proto schema, field-for-field, imported under the alias `pb` so the
pipeline code reads unchanged. Intermediates serialize with `encoding/gob`
behind a `TZFGOB1\n` magic header (`model.Marshal`/`Unmarshal`/`Clone`; the
ring-segment union members register under stable `tzf.*` names). The proto
schema survives only on the v1 branch; the data build runs directly from
the upstream raw GeoJSON — no translation step exists. Rebuilding from
GeoJSON was verified equivalent to the pb-era artifacts (full.tzb
byte-identical; lite differs only via preindex regeneration).

Pipeline packages: `internal/convert` (GeoJSON↔model), `internal/reduce`,
`internal/preindex`, `internal/topology`, `internal/gridindex`,
`internal/polyline`, `internal/polyf`, `internal/preindexexclude`,
`internal/borderchange`, `internal/embedenc` (the embedbin encoder +
`Verify`/`VerifyM`), `internal/pbref` (reference finders reproducing the
deleted v1 query semantics for parity), `internal/parity` (source-parity
tests, gated on `TZF_PARITY_TOPO` / `TZF_PARITY_PREINDEX` pointing at
`.gob` intermediates), and the pipeline CLIs under `cmd/`. The GeoJSON
types live in `internal/convert` alongside the GeoJSON ↔ model conversions.

### Data Pipeline

```
Raw GeoJSON (timezone-boundary-builder)
  └─ cmd/geojson2tzpb
       └─ combined-with-oceans.gob                (Timezones, full precision)
            │
            ├─ cmd/reducetzpb -topology=true
            │    └─ combined-with-oceans.topology.gob   (Timezones, topology-aware D-P simplified)
            │         ├─ cmd/deduplicatetzpb
            │         │    └─ combined-with-oceans.topology.topo.gob   (TopoTimezones)
            │         │         └─ cmd/compresstopotzpb
            │         │              └─ combined-with-oceans.topology.compress.topo.gob  ← lite source
            │         └─ cmd/preindextzpb
            │              └─ combined-with-oceans.topology.preindex.gob ← preindex source
            │
            └─ cmd/deduplicatetzpb
                 └─ combined-with-oceans.topo.gob        (TopoTimezones)
                      └─ cmd/compresstopotzpb
                           └─ combined-with-oceans.compress.topo.gob   ← full source
```

The gob intermediates then feed `topo2embed` (+`tzb2tzm`) to produce the v2
artifact set `github.com/ringsaturn/tzf-dist` will ship (W7): `lite.tzb`
(+FUZZY, backs `NewEmbeddedFinder`), `lite.tzm` (+FUZZY, backs
`NewDefaultFinder`), `full.tzb` (+FUZZY, backs `NewFullFinder`) — all
carrying one `data_version`. Once that set ships, tzf-dist stops publishing
the pb artifacts and the v1 data line is frozen (decision 2026-08-28).

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

### `internal/reduce` Package

- `reduce.go`: `DoTopologyAwareWithStats` wraps `topology.DoWithStats` + `MustValidateForReduction`.
- `compress.go`: polyline encode/decode for `Timezones` → `CompressedTimezones`.
- `compress_topo.go`: `CompressTopoTimezones` / `DecompressTopoTimezones` for `TopoTimezones` ↔ `CompressedTopoTimezones`; edge ID references pass through unchanged. `DecompressedPolylineBytesToPoints` decodes shared-edge bytes directly into `pb.Point` slices.

### `internal/geom` Package

Zero-external-dependency polygon geometry engine, replacing `tidwall/geojson`.

Core types are generic over the coordinate storage type `Coord`
(`~int32 | ~float64`): `PointOf[T]` / `RingOf[T]` / `PolygonOf[T]`, with
aliases `Point`/`Ring`/`Polygon` (float64, degree space) and
`I32Point`/`I32Ring`/`I32Polygon` (1e5-scaled int32, `I32Scale`). The type
parameter only governs storage; all arithmetic runs in float64 — queries scale
the point once (`scale` field: 1 or 1e5) and convert segment endpoints in
registers, so results are identical across storage types. int32/float64 have
different GC shapes, so both instantiations are fully monomorphised.

| File | Content |
|------|---------|
| `type.go` | `Coord` constraint; `PointOf[T]`, `Rect`; `I32Scale` |
| `ring.go` | `RingOf[T]`: open-ring representation, `ringBounds`, `ringAreaAndPerimeter` (Shoelace + perimeter, storage space) |
| `ystripes.go` | `yStripesIndex`: horizontal stripe PIP index in storage space; stripe count = max(32, ⌊n × circularity⌋); per-segment Y ranges recomputed from ring endpoints at query time (not stored); uint32 indices |
| `pip.go` | `raycastSeg` ray-casting (with `math.Nextafter` vertex deduplication); `ringContainsPoint[T]` |
| `polygon.go` | `PolygonOf[T]` (exterior + holes); `Poly` interface; `NewPolygon`/`NewI32Polygon`; `ContainsPoint`; `ContainsPoly` |

`Finder` builds `geom.PolygonOf` objects at load time; queries are allocation-free.

### `internal/polyf` Package

Generic point-in-polygon finder, replacing `github.com/ringsaturn/polyf` + `mitchellh/mapstructure`.

- `polyf.go`: `F[T]` (linear scan) and `RF[T]` (R-Tree–accelerated via `tidwall/rtree`) finders; `Item[T]` holds `*geom.Polygon` + value.
- `featurecollection.go`: `BoundaryFile[T]` GeoJSON FeatureCollection parser using `json.RawMessage`; no reflection.

Used by `preindex/exclude.go` and `convert/convert.go`.

### `internal/polyline` Package

Google Maps Encoded Polyline codec, replacing `github.com/twpayne/go-polyline`.

- `polyline.go`: `EncodeCoords` / `DecodeCoords` (delta + zig-zag, scale=1e5, 2D).

Used by `reduce/compress.go` and `reduce/compress_topo.go`.

### CLI Tools (`cmd/`)

| Tool | Input | Output | Purpose |
|------|-------|--------|---------|
| `geojson2tzpb` | GeoJSON | `.gob` (Timezones) | GeoJSON → pipeline model |
| `reducetzpb` | `.gob` (Timezones) | `.topology.gob` | Topology-aware D-P simplification |
| `deduplicatetzpb` | `.gob` (Timezones) | `.topo.gob` (TopoTimezones) | Shared-edge deduplication |
| `compresstopotzpb` | `.topo.gob` | `.compress.topo.gob` (CompressedTopoTimezones) | Polyline compress topo format |
| `compresstzpb` | `.gob` | `.compress.gob` (CompressedTimezones) | Polyline compress flat format |
| `preindextzpb` | `.topology.gob` | `.preindex.gob` | Tile pre-indexing |
| `topo2embed` | `.compress.topo.gob` (+`-preindex .preindex.gob`) | `.tzb` / `.tzm` | Embedded binary encoder (`-profile e\|m`) |
| `tzb2tzm` | `.tzb` | `.tzm` | pb-free transcode to the memory profile |
| `tzf` | — | — | CLI lookup over `NewDefaultFinder` |

The `*tzpb` command names are a v1-era holdover (there is no protobuf in the
pipeline any more); they are kept because the data-build workflow and
tzf-dist's `build.yml` invoke them by name.

## Known Data Quirks

- **Antimeridian (-180°/+180°):** `normalizeLng` (-180→+180) is used only for topology key matching (`newPointKey`/`newEdgeKey`), never to mutate geometric coordinates. Mixing signs in the same ring corrupts `signedArea` and winding detection.
- **Disputed territories:** Some timezone pairs share edges in the same direction (e.g. Israel/Palestine). `ReductionValidateOptions` disables `CheckSameDirectionSharedEdges` to allow this.
- **Source data zero-length edges:** Certain rings (e.g. Macau border-crossing building outline) have duplicate adjacent vertices in the upstream data; these must be stripped before topology analysis.
- **Fallback rings:** ~200 rings simplify to degenerate or self-intersecting shapes and revert to original geometry. They are mostly tiny island outlines and building-footprint enclaves.
