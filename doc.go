/*
Package tzf converts (lng, lat) coordinates to timezone names.

Inspired by timezonefinder https://github.com/jannikmi/timezonefinder,
a fast Python package for finding the timezone of any point on earth offline.

# Overview

v2 is protobuf-free. Boundary data ships as TZF embedded binary files
(.tzb, the compact transport profile) and TZF memory images (.tzm, the
query-time profile) from https://github.com/ringsaturn/tzf-dist, embedded in
this module's dependencies with go:embed. Every constructor returns [F]; the
mechanism behind it is not part of the API:

	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		panic(err)
	}
	// Coordinates are in longitude-latitude order.
	fmt.Println(finder.GetTimezoneName(116.3883, 39.9289))

# Constructors

Three pre-defined finders over embedded tzf-dist data:

  - [NewDefaultFinder]: the recommended general-purpose finder. Reads the
    lite .tzm memory image, where polygon storage aliases the embedded bytes,
    so the geometry stays in read-only data and the retained heap is 12.8 MiB
    (Apple M3 Max, 2026c dataset).
  - [NewEmbeddedFinder]: for embedded and memory-constrained targets. The
    lite .tzb file is queried in place, with under 1 KB of heap beyond the
    file bytes. A preindex miss costs about 6.9 µs against 542 ns for
    [NewDefaultFinder] (same machine and dataset).
  - [NewFullFinder]: full-precision geometry, expanded at load time. Use it
    when a query may land within ~111 m of a border and the exact answer
    matters.

Two bring-your-own-bytes constructors, for data read from disk, an object
store, or your own go:embed:

  - [NewFinderFromTZB]: always expands the file's geometry; data is released
    after loading.
  - [NewFinderFromTZM]: always aliases the file in place; data must stay
    live and unmodified for the finder's lifetime.

Construction is expensive relative to a query. Build one finder and reuse it
(a package-level variable is fine); every finder is safe for concurrent use.

# Queries

GetTimezoneName is fuzzy-first: when the file carries a FUZZY preindex
section, which every tzf-dist artifact does, a tile lookup resolves most
queries without point-in-polygon work and falls back to exact ray casting
near borders. GetTimezoneNames runs the polygon scan in every finder and is
the call to use when a point may belong to more than one timezone; results
are sorted lexicographically. A point on a shared border belongs to every
touching polygon.

# GeoJSON export

[F] covers the four query methods only, so [GeoJSONer] is a separate
interface: assert it on the value a constructor returned. Every finder built
here satisfies it.

	boundaries := finder.(tzf.GeoJSONer).GetGeoJSON()

# Experimental surface

Package [github.com/ringsaturn/tzf/v2/x] holds surface that is useful in
production but exempt from this module's semantic-versioning promise. It
currently covers in-place querying over an io.ReaderAt: a file, an mmap'd
region, an embedded flash adapter, or a bytes.Reader over caller-owned bytes.
A minor version bump may break it; the root package's API breaks only at a
major version.
*/
package tzf
