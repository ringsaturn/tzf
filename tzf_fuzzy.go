package tzf

import (
	"errors"
	"math"
	"slices"
	"sort"

	"github.com/ringsaturn/tzf/v2/internal/convert"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"github.com/ringsaturn/tzf/v2/internal/geom"
)

// fuzzyIndex answers queries from the preindex tiles of a file's FUZZY
// section, rebuilt as a flat tile grid. It is the fast path inside the
// composed finders and is not a public mechanism in v2 (spec §5.2): a query
// it cannot answer unambiguously falls through to the polygon finder.
//
// The root is a dense 2^rootZoom × 2^rootZoom array of nodes. Below it,
// every two zooms form one block level down to leafZoom, the finest zoom any
// tile has: a node whose area holds finer tiles points to a block of 4 × 4
// child nodes. A node is a names index (< blockBase), a block pointer
// (blockBase + block), or noTile. A lookup is one root read plus one read per
// block level, instead of a map probe per zoom.
//
// Answers are those of probing every zoom from aggZoom to idxZoom, coarsest
// first, with boundary tiles resolving to their first name: the grid is
// built finest tiles first, so a coarser tile overwrites any finer one under
// it.
type fuzzyIndex struct {
	idxZoom   uint8
	rootZoom  uint8
	leafZoom  uint8
	blockBase uint16
	root      []uint16
	blocks    []uint16
	names     []string

	// The tiles in file order (ascending keys), kept for the preindex
	// GeoJSON export: tile i names tileIdx[tileOff[i]:tileOff[i+1]].
	tiles   []geom.TileID
	tileOff []uint32
	tileIdx []uint16
}

const (
	// blockSide is the side of a block: each block level refines a node by
	// two zooms, into blockSide × blockSide children.
	blockSide  = 4
	blockCells = blockSide * blockSide
	// maxRootZoom is the finest root zoom: 4^8 = 65,536 nodes (128 KB).
	maxRootZoom = 8
	// noTile marks a node no tile covers.
	noTile = math.MaxUint16
	// buildNoTile is noTile in the wider nodes used while building, before
	// identical blocks are merged.
	buildNoTile = math.MaxUint32
)

// childSlot is the position inside a block of the child holding cell (x, y),
// where the cells are below zooms finer than the block's children.
func childSlot(x, y uint32, below uint8) int {
	return int(((y>>below)&(blockSide-1))*blockSide + (x>>below)&(blockSide-1))
}

// newFuzzyIndexFromReader rebuilds the preindex tile grid from a file's
// FUZZY section (~0.5 MB on the bundled preindex) plus the compact tile list
// the GeoJSON export reads. The source bytes are not retained. It fails when
// the file has no FUZZY section.
func newFuzzyIndexFromReader(reader *embedbin.Reader) (*fuzzyIndex, error) {
	if !reader.HasFuzzy() {
		return nil, embedbin.ErrNoFuzzy
	}
	names := make([]string, reader.TimezoneCount())
	for i := range names {
		name, err := reader.Name(int32(i))
		if err != nil {
			return nil, err
		}
		names[i] = string(name)
	}
	if len(names) >= noTile {
		return nil, errors.New("tzf: too many timezones for the FUZZY grid")
	}
	idx, agg := reader.FuzzyZooms()
	idxZoom, aggZoom := uint8(idx), uint8(agg)

	n := reader.FuzzyTileCount()
	f := &fuzzyIndex{
		idxZoom:   idxZoom,
		blockBase: uint16(len(names)),
		names:     names,
		tiles:     make([]geom.TileID, 0, n),
		tileOff:   make([]uint32, 1, n+1),
		// Nearly every tile names one timezone.
		tileIdx: make([]uint16, 0, n+n/16),
	}
	err := reader.FuzzyEntries(func(key geom.TileID, indices []uint16) {
		f.tiles = append(f.tiles, key)
		f.tileIdx = append(f.tileIdx, indices...)
		f.tileOff = append(f.tileOff, uint32(len(f.tileIdx)))
	})
	if err != nil {
		return nil, err
	}
	if err := f.buildGrid(aggZoom); err != nil {
		return nil, err
	}
	return f, nil
}

// buildGrid builds root and blocks from the tile list. Only zooms in
// aggZoom..idxZoom are ever probed, so only those tiles go in.
func (f *fuzzyIndex) buildGrid(aggZoom uint8) error {
	// The keys are ascending and the zoom lives in their high bits, so the
	// tiles run coarsest zoom first and the finest probed zoom is the last
	// in range.
	f.leafZoom = aggZoom
	for i := len(f.tiles) - 1; i >= 0; i-- {
		if _, _, z := f.tiles[i].XYZ(); z <= f.idxZoom {
			f.leafZoom = max(aggZoom, z)
			break
		}
	}
	// Block levels sit every two zooms above the leaf.
	f.rootZoom = f.leafZoom
	if f.leafZoom > maxRootZoom {
		f.rootZoom = maxRootZoom - (f.leafZoom-maxRootZoom)%2
	}

	base := uint32(f.blockBase)
	side := 1 << f.rootZoom
	root := make([]uint32, side*side)
	for i := range root {
		root[i] = buildNoTile
	}
	// Tiles below the root zoom: the last ones in key order. Fewer blocks
	// than those tiles get made, as most blocks hold several of them.
	finer := len(f.tiles) - sort.Search(len(f.tiles), func(i int) bool {
		_, _, z := f.tiles[i].XYZ()
		return z > f.rootZoom
	})
	blocks := make([]uint32, 0, finer/2*blockCells)
	// Finest first: a coarser tile then overwrites any finer one under it.
	for i := len(f.tiles) - 1; i >= 0; i-- {
		x, y, z := f.tiles[i].XYZ()
		if z < aggZoom || z > f.idxZoom {
			continue
		}
		value := uint32(f.tileIdx[f.tileOff[i]])
		if z <= f.rootZoom {
			span := uint32(1) << (f.rootZoom - z)
			for ry := y * span; ry < (y+1)*span; ry++ {
				row := int(ry) * side
				for rx := x * span; rx < (x+1)*span; rx++ {
					root[row+int(rx)] = value
				}
			}
			continue
		}
		// The block level holding z: the shallowest level at or below it,
		// the tile's top-left cell there, and the cells it spans.
		level := f.rootZoom + (z-f.rootZoom+1)/2*2
		span := uint32(1) << (level - z)
		lx, ly := x*span, y*span
		shift := level - f.rootZoom
		nodes, pos := root, int(ly>>shift)*side+int(lx>>shift)
		depth := f.rootZoom
		for {
			node := nodes[pos]
			if node == buildNoTile {
				// Descend into a fresh block.
				node = base + uint32(len(blocks)/blockCells)
				nodes[pos] = node
				blocks = append(blocks, make([]uint32, blockCells)...)
				for i := len(blocks) - blockCells; i < len(blocks); i++ {
					blocks[i] = buildNoTile
				}
			} else if node < base {
				// Already covered by a coarser tile, which wins.
				break
			}
			b := int(node-base) * blockCells
			depth += 2
			if depth == level {
				// span ≤ 2 here, and an aligned span never crosses the block.
				for cy := ly; cy < ly+span; cy++ {
					for cx := lx; cx < lx+span; cx++ {
						blocks[b+childSlot(cx, cy, 0)] = value
					}
				}
				break
			}
			nodes, pos = blocks, b+childSlot(lx, ly, level-depth)
		}
	}
	var err error
	f.root, f.blocks, err = compactGrid(root, blocks, base)
	return err
}

// compactGrid merges identical blocks and narrows the nodes to uint16.
//
// The grid is read-only once built, so blocks with the same content can be
// shared: on the bundled data about half of them repeat. A block is always
// created after its parent, so walking blocks from the last one back meets
// every child before its parent, and a parent's content is compared with its
// child pointers already merged. Blocks no node reaches (their area was
// overwritten by a coarser tile) are dropped.
func compactGrid(root, blocks []uint32, base uint32) ([]uint16, []uint16, error) {
	isBlock := func(v uint32) bool { return v != buildNoTile && v >= base }
	count := len(blocks) / blockCells
	reachable := make([]bool, count)
	for _, v := range root {
		if isBlock(v) {
			reachable[v-base] = true
		}
	}
	// Children have larger indices than their parents.
	for b := range count {
		if reachable[b] {
			for _, v := range blocks[b*blockCells : (b+1)*blockCells] {
				if isBlock(v) {
					reachable[v-base] = true
				}
			}
		}
	}

	errRange := errors.New("tzf: FUZZY grid exceeds its node range")
	remap := make([]uint32, count)
	narrow := func(v uint32) (uint16, bool) {
		switch {
		case v == buildNoTile:
			return noTile, true
		case isBlock(v):
			v = base + remap[v-base]
		}
		return uint16(v), v < noTile
	}
	seen := make(map[[blockCells]uint16]uint32, count/2)
	// Merged blocks in the order first met: children before parents.
	merged := make([][blockCells]uint16, 0, count/2)
	for b := count - 1; b >= 0; b-- {
		if !reachable[b] {
			continue
		}
		var cells [blockCells]uint16
		for i, v := range blocks[b*blockCells : (b+1)*blockCells] {
			n, ok := narrow(v)
			if !ok {
				return nil, nil, errRange
			}
			cells[i] = n
		}
		id, ok := seen[cells]
		if !ok {
			id = uint32(len(merged))
			merged = append(merged, cells)
			seen[cells] = id
		}
		remap[b] = id
	}
	outBlocks := make([]uint16, 0, len(merged)*blockCells)
	for i := range merged {
		outBlocks = append(outBlocks, merged[i][:]...)
	}
	outRoot := make([]uint16, len(root))
	for i, v := range root {
		n, ok := narrow(v)
		if !ok {
			return nil, nil, errRange
		}
		outRoot[i] = n
	}
	return outRoot, outBlocks, nil
}

// getIndex returns the names index of the tile answer, or -1 when no tile
// covers the point (the caller falls back to polygons).
func (f *fuzzyIndex) getIndex(lng float64, lat float64) int {
	// NaN fails every comparison, so it is rejected here too.
	if !(lng >= -180 && lng <= 180 && lat >= -90 && lat <= 90) {
		return -1
	}
	// Derived from the idxZoom tile exactly as the per-zoom probe did, so
	// float rounding at tile edges cannot change an answer.
	x, y, _ := geom.NewTileID(lng, lat, uint(f.idxZoom)).Shift(f.idxZoom - f.leafZoom).XYZ()
	shift := f.leafZoom - f.rootZoom
	node := f.root[int(y>>shift)<<f.rootZoom|int(x>>shift)]
	depth := f.rootZoom
	for node != noTile && node >= f.blockBase {
		depth += 2
		node = f.blocks[int(node-f.blockBase)*blockCells+childSlot(x, y, f.leafZoom-depth)]
	}
	if node == noTile {
		return -1
	}
	return int(node)
}

// getTimezoneName returns the tile answer, or "" when no tile covers the
// point; the caller falls back to polygons.
func (f *fuzzyIndex) getTimezoneName(lng float64, lat float64) string {
	if i := f.getIndex(lng, lat); i >= 0 {
		return f.names[i]
	}
	return ""
}

// preindexFeature builds the preindex-tile Feature for tzName, or nil when
// the dataset does not contain the name or no tile names it. The same name
// may map to more than one directory item, so all matching indices count.
func (f *fuzzyIndex) preindexFeature(tzName string) *convert.FeatureItem {
	var targets []uint16
	for i, name := range f.names {
		if name == tzName {
			targets = append(targets, uint16(i))
		}
	}
	if len(targets) == 0 {
		return nil
	}
	var tiles []geom.TileID
	for i, key := range f.tiles {
		for _, idx := range f.tileIdx[f.tileOff[i]:f.tileOff[i+1]] {
			if slices.Contains(targets, idx) {
				tiles = append(tiles, key)
				break
			}
		}
	}
	if len(tiles) == 0 {
		return nil
	}
	return convert.RevertItemFromTiles(tzName, tiles)
}

// preindexFeatures builds one Feature per timezone owning at least one
// preindex tile, in dataset order.
func (f *fuzzyIndex) preindexFeatures() []*convert.FeatureItem {
	grouped := make(map[uint16][]geom.TileID)
	for i, key := range f.tiles {
		for _, idx := range f.tileIdx[f.tileOff[i]:f.tileOff[i+1]] {
			grouped[idx] = append(grouped[idx], key)
		}
	}
	features := make([]*convert.FeatureItem, 0, len(grouped))
	for i, name := range f.names {
		tiles, ok := grouped[uint16(i)]
		if !ok {
			continue
		}
		features = append(features, convert.RevertItemFromTiles(name, tiles))
	}
	return features
}
