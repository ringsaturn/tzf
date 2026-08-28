package tzf

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// assembleI32Items builds the finder items from expanded (or aliased) ring
// data. Assembly cost is dominated by the per-ring YStripes build and every
// timezone is independent, so the work fans out across the CPUs; used by both
// the .tzm loader and the .tzb expansion loader.
func assembleI32Items(names []string, polygons [][]embedbin.ExpandedPolygon) []*tzitem[int32] {
	items := make([]*tzitem[int32], len(polygons))
	var cursor atomic.Int64
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(polygons)) {
		wg.Go(func() {
			for {
				i := int(cursor.Add(1)) - 1
				if i >= len(polygons) {
					return
				}
				polys := polygons[i]
				newItem := &tzitem[int32]{name: names[i]}
				newItem.polys = make([]*geom.I32Polygon, len(polys))
				for j, poly := range polys {
					newItem.polys[j] = geom.NewI32Polygon(poly.Exterior, poly.Holes)
				}
				minp, maxp := newItem.getMinMax()
				newItem.min = minp
				newItem.max = maxp
				items[i] = newItem
			}
		})
	}
	wg.Wait()
	return items
}

// NewFinderFromTZM builds a finder over a TZF memory-profile (.tzm) file,
// whose sections already are the query-time structures (spec rev 1 §6). On
// little-endian hosts every ring slice aliases the file's FLATPOINTS section
// and grid lookups run in place over the GRID section, so opening does close
// to zero decode work and the retained heap beyond the mapping is mostly the
// per-ring YStripes indices rebuilt at open.
//
// When the file carries a FUZZY section, GetTimezoneName consults the
// preindex tiles first and falls back to the polygon view on a miss;
// GetTimezoneNames always uses the polygon view.
//
// data must stay live and unmodified for the finder's lifetime: it is the
// polygon storage, not a decode source. Pass go:embed bytes or an mmap'd
// region; the latter is shared across processes by the page cache. Aliasing
// requires 8-byte alignment of data; misaligned or big-endian hosts fall
// back to a one-time decoded copy — correct, but the memory benefit
// degrades.
func NewFinderFromTZM(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	f, err := newFinderFromTZMReader(reader)
	if err != nil {
		return nil, err
	}
	if !reader.HasFuzzy() {
		return f, nil
	}
	fuzzy, err := newFuzzyIndexFromReader(reader)
	if err != nil {
		return nil, err
	}
	return &defaultFinder{fuzzy: fuzzy, finder: f}, nil
}

func newFinderFromTZMReader(reader *embedbin.Reader) (*finder, error) {
	view, err := reader.Flat()
	if err != nil {
		return nil, err
	}
	items := assembleI32Items(view.Names, view.Polygons)
	return &finder{
		core:    &finderImpl[int32]{items: items, dense: view.Grid},
		names:   view.Names,
		version: view.Version,
	}, nil
}
