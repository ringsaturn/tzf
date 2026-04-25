# internal/topology

Topology-aware polygon simplification for timezone boundary data.

## Overview

Standard Douglas-Peucker simplification treats each polygon independently. When
two adjacent timezone polygons share a boundary, simplifying them separately
produces slightly different results for that boundary — creating gaps or overlaps
between zones. This package solves that by identifying shared boundaries before
simplification and ensuring both sides use the same simplified result.

## How it works

### 1. Coordinate normalisation

All coordinates are cast to `float32` to match the protobuf storage format.
Longitude −180° and +180° represent the same antimeridian line; the
`newPointKey` / `newEdgeKey` functions unify them for topology matching **only**
— the actual ring coordinates are never modified, which prevents antimeridian-
adjacent polygons from getting mixed-sign longitude coordinates that would
confuse area and winding calculations.

### 2. Vertex snapping

`snapVertices` detects T-junctions: cases where a vertex of one ring lies
exactly on an edge of another ring but is not yet a vertex there. It inserts
the missing vertex so that subsequent edge-hash matching can recognise the
shared boundary. An R-Tree spatial index limits the search to nearby vertices.
Only one snapping pass is performed; the source data is expected to be nearly
aligned already.

### 3. Shared edge detection

`collectRings` decomposes every ring into directed edges and builds two indexes:

- **edge index** — maps each canonical edge key (endpoint pair sorted by
  coordinate) to all rings that contain it.
- **vertex index** — maps each point key to the set of rings it appears in.

`markSharedEdges` then marks every edge that appears in exactly two different
rings with opposite directions as *shared*, recording which ring is the partner.

### 4. Fixed vertex marking

`markFixedVertices` identifies vertices that must not move during
simplification. A vertex is fixed only if it is a true topological node:

| Condition | Meaning |
|---|---|
| `prev.Shared != next.Shared` | Transition between shared and non-shared edges — this is a chain endpoint. |
| Both shared, different partners | Junction between two distinct shared chains. |
| `len(vertexIndex[v]) > 2` | Three or more rings meet here — a multi-way junction. |

Vertices that are interior to a single shared chain (both neighbours are shared
with the same partner) are intentionally **not** fixed. Marking them would
fragment the chain into many tiny segments and leave Douglas-Peucker with
nothing to compress.

### 5. Simplification

`simplifyRing` splits each ring at its fixed vertices into segments and
simplifies each segment:

- **No fixed vertices** — the whole ring is simplified as one closed path.
- **One fixed vertex** — the ring is rotated so the fixed vertex is first,
  then simplified as one open path.
- **Multiple fixed vertices** — the ring is split into segments between
  consecutive fixed vertices. Each segment is passed to `simplifySegment`.

`simplifySegment` checks whether a segment is fully shared (all edges belong
to the same partner ring). If so, a canonical key is computed from the segment
coordinates and looked up in `sharedCache`. The first ring to process the
segment stores the result; the partner ring retrieves it and reverses the
point order if necessary. This guarantees both sides of a shared boundary use
identical simplified coordinates.

Segments shorter than `minSimplifyPoints = 4` are passed through unchanged.

### 6. Winding normalisation

After simplification, `normalizeWindings` enforces the GeoJSON convention:
exterior rings are counter-clockwise, holes are clockwise.

### 7. Validation

`Validate` / `ValidateWithOptions` checks the output for:

- Rings with fewer than 4 points or fewer than 3 unique points
- Open rings (first and last point not equal)
- Rings with degenerate area (below `minRingArea = 1e-12`)
- Incorrect winding direction
- Shared edges appearing in the same direction in two different rings
  (disabled in `ReductionValidateOptions` to allow intentional overlaps in
  disputed-territory data)

`MustValidateForReduction` panics on any violation and is called automatically
by `reduce.DoTopologyAwareWithStats`.

## Entry points

```go
// Simplify and return the result.
output := topology.Do(input, epsilon)

// Simplify and return detailed statistics.
output, stats := topology.DoWithStats(input, epsilon)

// Validate output geometry.
if err := topology.Validate(output); err != nil { ... }
```

`Stats.String()` returns a multi-line summary suitable for logging:

```
topology_rings: total=2078 no_fixed=1476 one_fixed=16 multi_fixed=581 fallback=168
topology_points: input=8022588 snapped_inserted=100 fallback_points=5463 fixed_vertices=173757
topology_segments: total=175226 shared=2300(1.31%) skipped_short=170247(97.16%) ...
topology_segment_points: input=8197799 output=1258366 reduction=84.65%
topology_segment_length_buckets: le10=170732 le25=409 le50=424 le100=561 gt100=3100
```

## Known limitations

- **Short segments** — ~97% of segments in the global dataset have ≤ 4 points
  and are passed through without simplification. These are mostly short
  coastline edges and small island boundaries that cannot be reduced further at
  `epsilon = 0.001` without losing geographic fidelity.
- **Fallback rings** — rings that collapse to fewer than 3 unique points after
  simplification fall back to their original geometry (~168 rings in the global
  dataset). These are typically very small island polygons.
- **Antimeridian shared-chain cache misses** — edges crossing the antimeridian
  have coordinates at −180° on one side and +180° on the other, so they
  generate different segment signatures and skip the shared cache. Both sides
  are still simplified independently with the same epsilon and produce
  consistent results for straight antimeridian edges.
