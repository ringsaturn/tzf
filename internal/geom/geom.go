// Package geom provides zero-dependency 2-D geometry primitives and a
// YStripes-indexed point-in-polygon query optimised for timezone lookup.
//
// The YStripes index divides each polygon ring's Y-axis span into horizontal
// stripes. A PIP query needs only to scan the segments stored in the single
// stripe that contains the query latitude, reducing work from O(n) to roughly
// O(n/stripes) in the common case.
//
// The ray-casting algorithm is a direct Go port of github.com/tidwall/geojson
// (MIT), which is itself the canonical implementation used throughout this
// project.
package geom
