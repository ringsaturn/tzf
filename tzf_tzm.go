package tzf

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ringsaturn/tzf/internal/embedbin"
	"github.com/ringsaturn/tzf/internal/geom"
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

// NewFinderFromTZM builds a [Finder] over a TZF memory-profile (.tzm) file,
// whose sections already are the query-time structures (spec rev 1 §6). On
// little-endian hosts every ring slice aliases the file's FLATPOINTS section
// and grid lookups run in place over the GRID section, so opening does close
// to zero decode work and the retained heap beyond the mapping is mostly the
// per-ring YStripes indices rebuilt at open. Queries run at full [Finder]
// speed and return the same results as [NewFinderFromCompressedTopo] over the
// source dataset.
//
// data must stay live and unmodified for the finder's lifetime: it is the
// polygon storage, not a decode source. Pass go:embed bytes or an mmap'd
// region; the latter is shared across processes by the page cache.
func NewFinderFromTZM(data []byte, opts ...OptionFunc) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	return newFinderFromTZMReader(reader, opts...)
}

func newFinderFromTZMReader(reader *embedbin.Reader, opts ...OptionFunc) (*Finder, error) {
	opt := &Option{}
	for _, optFunc := range opts {
		optFunc(opt)
	}
	view, err := reader.Flat()
	if err != nil {
		return nil, err
	}
	items := assembleI32Items(view.Names, view.Polygons)
	return &Finder{
		core:    &finderImpl[int32]{items: items, dense: view.Grid},
		names:   view.Names,
		opt:     opt,
		version: view.Version,
	}, nil
}

// NewDefaultFinderFromTZM builds a [DefaultFinder] from one .tzm file
// carrying both geometry and a FUZZY section: a [FuzzyFinder] rebuilt from
// FUZZY handles the fast path, with the in-place polygon view as fallback.
// Unlike [NewDefaultFinderFromTZB], the source bytes are retained — they are
// the polygon storage — so data must stay live and unmodified.
func NewDefaultFinderFromTZM(data []byte) (F, error) {
	reader, err := embedbin.Open(data)
	if err != nil {
		return nil, err
	}
	fuzzyFinder, err := newFuzzyFinderFromTZBReader(reader)
	if err != nil {
		return nil, err
	}
	finder, err := newFinderFromTZMReader(reader, SetDropPBTZ)
	if err != nil {
		return nil, err
	}
	return &DefaultFinder{fuzzyFinder: fuzzyFinder, finder: finder}, nil
}
