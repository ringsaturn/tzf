package embedenc

import (
	"encoding/binary"
	"fmt"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"math"
	"slices"

	"github.com/ringsaturn/tzf/v2/internal/geom"
	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

// encodeFuzzy builds a FUZZY section from a preindex tile set. Name indices
// reference the file's NAMES order; the encoder fails on any preindex name
// absent from names, and on a preindex/geometry data-version mismatch.
func encodeFuzzy(pre *pb.PreindexTimezones, names []string, version string) ([]byte, error) {
	if pre == nil || len(pre.Keys) == 0 {
		return nil, fmt.Errorf("fuzzy: %w: empty preindex", embedbin.ErrMalformed)
	}
	if pre.Version != version {
		return nil, fmt.Errorf("fuzzy: %w: preindex version %q != data version %q", embedbin.ErrMalformed, pre.Version, version)
	}
	idxZoom, aggZoom := pre.IdxZoom, pre.AggZoom
	if aggZoom < 0 || aggZoom > idxZoom || idxZoom > 28 {
		return nil, fmt.Errorf("fuzzy: %w: zoom range agg=%d idx=%d", embedbin.ErrMalformed, aggZoom, idxZoom)
	}
	if len(names) > embedbin.FuzzyMaxNames {
		return nil, fmt.Errorf("fuzzy: %w: %d timezones exceed the 15-bit value limit", embedbin.ErrMalformed, len(names))
	}
	nameIdx := make(map[string]uint16, len(names))
	for i, n := range names {
		nameIdx[n] = uint16(i)
	}

	// Group tile entries; order within a tile preserves the source preindex
	// key order (first-listed wins in single-result lookups).
	tiles := make(map[geom.TileID][]uint16, len(pre.Keys))
	for i, item := range pre.Keys {
		if item == nil {
			return nil, fmt.Errorf("fuzzy: %w: nil preindex key %d", embedbin.ErrMalformed, i)
		}
		if item.X < 0 || item.Y < 0 || int64(item.X) >= 1<<28 || int64(item.Y) >= 1<<28 {
			return nil, fmt.Errorf("fuzzy: %w: tile x/y out of range at key %d", embedbin.ErrMalformed, i)
		}
		if item.Z < aggZoom || item.Z > idxZoom {
			return nil, fmt.Errorf("fuzzy: %w: tile zoom %d outside [%d,%d]", embedbin.ErrMalformed, item.Z, aggZoom, idxZoom)
		}
		idx, ok := nameIdx[item.Name]
		if !ok {
			return nil, fmt.Errorf("fuzzy: %w: preindex name %q absent from NAMES", embedbin.ErrMalformed, item.Name)
		}
		key := geom.NewTileIDFromXYZ(uint32(item.X), uint32(item.Y), uint8(item.Z))
		tiles[key] = append(tiles[key], idx)
	}

	keys := make([]geom.TileID, 0, len(tiles))
	for key := range tiles {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	values := make([]uint16, len(keys))
	var multiDir []uint16
	var multiValues []uint16
	for i, key := range keys {
		group := tiles[key]
		if len(group) == 1 {
			values[i] = group[0]
			continue
		}
		groupIndex := len(multiDir) / 2
		if groupIndex >= embedbin.FuzzyMaxNames {
			return nil, fmt.Errorf("fuzzy: %w: multi group count exceeds 15-bit limit", embedbin.ErrMalformed)
		}
		if len(multiValues)+len(group) > math.MaxUint16 {
			return nil, fmt.Errorf("fuzzy: %w: multi value count exceeds uint16", embedbin.ErrMalformed)
		}
		multiDir = append(multiDir, uint16(len(multiValues)), uint16(len(group)))
		multiValues = append(multiValues, group...)
		values[i] = embedbin.FuzzyMulti | uint16(groupIndex)
	}

	raw := embedbin.FuzzyHeaderLen + 8*uint64(len(keys)) + 2*uint64(len(values)) +
		2*uint64(len(multiDir)) + 2*uint64(len(multiValues))
	padded := embedbin.Align4(raw)
	if padded > math.MaxUint32 {
		return nil, fmt.Errorf("fuzzy: %w: section capacity", embedbin.ErrMalformed)
	}
	out := make([]byte, padded)
	out[0] = uint8(idxZoom)
	out[1] = uint8(aggZoom)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(keys)))
	binary.LittleEndian.PutUint32(out[8:], uint32(len(multiDir)/2))
	binary.LittleEndian.PutUint32(out[12:], uint32(len(multiValues)))
	pos := int(embedbin.FuzzyHeaderLen)
	for _, key := range keys {
		binary.LittleEndian.PutUint64(out[pos:], uint64(key))
		pos += 8
	}
	for _, v := range values {
		binary.LittleEndian.PutUint16(out[pos:], v)
		pos += 2
	}
	for _, v := range multiDir {
		binary.LittleEndian.PutUint16(out[pos:], v)
		pos += 2
	}
	for _, v := range multiValues {
		binary.LittleEndian.PutUint16(out[pos:], v)
		pos += 2
	}
	return out, nil
}
