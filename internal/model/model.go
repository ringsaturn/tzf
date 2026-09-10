// Package model defines the pipeline's intermediate data structures: native
// Go mirrors of the retired tzf.v1 protobuf schema (field-for-field, so the
// pipeline code that consumed the generated bindings compiles against this
// package under the same alias). Intermediates serialize with encoding/gob
// behind a small magic header; they are private to the pipeline — the only
// distribution formats are `.tzb`/`.tzm`, and the frozen v1 line at the repo
// root keeps its own protobuf copy for history.
package model

// Point is a basic (lng, lat) coordinate. float32 matches the retired pb
// schema: the flat formats carried float32 precision, and the compressed
// formats never touch these fields at query time.
type Point struct {
	Lng float32
	Lat float32
}

// Polygon follows GeoJSON's shape: Points is the exterior ring, Holes are
// the interior rings.
type Polygon struct {
	Points []*Point
	Holes  []*Polygon
}

// Timezone is one timezone's polygons.
type Timezone struct {
	Polygons []*Polygon
	Name     string
}

// Timezones is the flat full-precision dataset.
type Timezones struct {
	Timezones []*Timezone
	Reduced   bool
	Version   string
}

// CompressMethod selects the coordinate compression codec.
type CompressMethod int32

const (
	CompressMethod_COMPRESS_METHOD_UNSPECIFIED CompressMethod = 0
	CompressMethod_COMPRESS_METHOD_POLYLINE    CompressMethod = 1
)

type CompressedPolygon struct {
	Points []byte
	Holes  []*CompressedPolygon
}

type CompressedTimezone struct {
	Data []*CompressedPolygon
	Name string
}

type CompressedTimezones struct {
	Method    CompressMethod
	Timezones []*CompressedTimezone
	Version   string
}

// PreindexTimezone is one OSM-style tile claimed by a timezone.
type PreindexTimezone struct {
	Name string
	X    int32
	Y    int32
	Z    int32
}

// PreindexTimezones is the tile preindex dump.
type PreindexTimezones struct {
	IdxZoom int32
	AggZoom int32
	Keys    []*PreindexTimezone
	Version string
}

// InlinePoints wraps a point sequence used inside a ring-segment union.
type InlinePoints struct {
	Points []*Point
}

// RingSegment is either inline points or a shared-edge reference; Content
// holds exactly one of RingSegment_Inline, RingSegment_EdgeForward,
// RingSegment_EdgeReversed (the retired pb oneof shape).
type RingSegment struct {
	Content isRingSegmentContent
}

type isRingSegmentContent interface{ isRingSegmentContent() }

type RingSegment_Inline struct {
	Inline *InlinePoints
}
type RingSegment_EdgeForward struct {
	EdgeForward int32
}
type RingSegment_EdgeReversed struct {
	EdgeReversed int32
}

func (*RingSegment_Inline) isRingSegmentContent()       {}
func (*RingSegment_EdgeForward) isRingSegmentContent()  {}
func (*RingSegment_EdgeReversed) isRingSegmentContent() {}

// TopoPolygon is a polygon whose exterior is a segment sequence; holes are
// nested TopoPolygons.
type TopoPolygon struct {
	Exterior []*RingSegment
	Holes    []*TopoPolygon
}

type TopoTimezone struct {
	Polygons []*TopoPolygon
	Name     string
}

// SharedEdge is one boundary edge stored once in the global edge library.
type SharedEdge struct {
	Id     int32
	Points []*Point
}

// TopoTimezones is the shared-edge deduplicated dataset.
type TopoTimezones struct {
	SharedEdges []*SharedEdge
	Timezones   []*TopoTimezone
	Version     string
}

// CompressedSharedEdge stores a shared edge polyline-encoded.
type CompressedSharedEdge struct {
	Id     int32
	Points []byte
}

type CompressedInlinePoints struct {
	Points []byte
}

// CompressedRingSegment mirrors RingSegment with polyline-encoded inline
// points; Content holds exactly one of CompressedRingSegment_Inline,
// CompressedRingSegment_EdgeForward, CompressedRingSegment_EdgeReversed.
type CompressedRingSegment struct {
	Content isCompressedRingSegmentContent
}

type isCompressedRingSegmentContent interface{ isCompressedRingSegmentContent() }

type CompressedRingSegment_Inline struct {
	Inline *CompressedInlinePoints
}
type CompressedRingSegment_EdgeForward struct {
	EdgeForward int32
}
type CompressedRingSegment_EdgeReversed struct {
	EdgeReversed int32
}

func (*CompressedRingSegment_Inline) isCompressedRingSegmentContent()       {}
func (*CompressedRingSegment_EdgeForward) isCompressedRingSegmentContent()  {}
func (*CompressedRingSegment_EdgeReversed) isCompressedRingSegmentContent() {}

type CompressedTopoPolygon struct {
	Exterior []*CompressedRingSegment
	Holes    []*CompressedTopoPolygon
}

type CompressedTopoTimezone struct {
	Polygons []*CompressedTopoPolygon
	Name     string
}

// CompressedTopoTimezones combines shared-edge deduplication with polyline
// compression; GridIndex is the optional embedded 1°×1° candidate index.
type CompressedTopoTimezones struct {
	Method      CompressMethod
	SharedEdges []*CompressedSharedEdge
	Timezones   []*CompressedTopoTimezone
	Version     string
	GridIndex   *GridIndex
}

// GridIndexCell records the timezone indices intersecting one 1°×1° cell.
type GridIndexCell struct {
	Lng       int32
	Lat       int32
	TzIndices []uint32
}

// GridIndex is the complete 1°×1° candidate-reduction index.
type GridIndex struct {
	Cells   []*GridIndexCell
	Version string
}
