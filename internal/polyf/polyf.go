// Package polyf is a zero-external-dependency port of github.com/ringsaturn/polyf.
// It replaces geometry.Poly with internal/geom.Polygon and keeps the same
// generic F[T] / RF[T] API surface.
package polyf

import (
	"errors"

	"github.com/ringsaturn/tzf/internal/geom"
	"github.com/tidwall/rtree"
)

var ErrNotFound = errors.New("polyf: not found")

// Item pairs an arbitrary value with the polygon it belongs to.
type Item[T any] struct {
	V    T
	Poly *geom.Polygon
}

// F is a simple linear-scan polygon finder. Suitable for small collections
// (< a few hundred polygons) where the overhead of a spatial index is not worth it.
type F[T any] struct {
	Items []*Item[T]
}

func (f *F[T]) Insert(poly *geom.Polygon, v T) {
	f.Items = append(f.Items, &Item[T]{V: v, Poly: poly})
}

func (f *F[T]) FindOneWithPoly(x, y float64) (*Item[T], error) {
	p := geom.Point{X: x, Y: y}
	for _, item := range f.Items {
		if item.Poly.ContainsPoint(p) {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (f *F[T]) FindOne(x, y float64) (T, error) {
	res, err := f.FindOneWithPoly(x, y)
	if err != nil {
		return *new(T), err
	}
	return res.V, nil
}

func (f *F[T]) FindAllWithPoly(x, y float64) ([]*Item[T], error) {
	p := geom.Point{X: x, Y: y}
	res := make([]*Item[T], 0)
	for _, item := range f.Items {
		if item.Poly.ContainsPoint(p) {
			res = append(res, item)
		}
	}
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

func (f *F[T]) FindAll(x, y float64) ([]T, error) {
	p := geom.Point{X: x, Y: y}
	res := make([]T, 0)
	for _, item := range f.Items {
		if item.Poly.ContainsPoint(p) {
			res = append(res, item.V)
		}
	}
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

// RF is an R-Tree-accelerated polygon finder. It uses a bounding-box index to
// narrow candidates before performing exact PIP checks.
type RF[T any] struct {
	tree *rtree.RTreeG[*Item[T]]
	f    *F[T]
}

// NewRFFromF builds an RF from an existing F, inserting all items into the
// R-Tree using their polygon bounding boxes.
func NewRFFromF[T any](f *F[T]) *RF[T] {
	tree := &rtree.RTreeG[*Item[T]]{}
	for _, item := range f.Items {
		r := item.Poly.Rect()
		tree.Insert([2]float64{r.Min.X, r.Min.Y}, [2]float64{r.Max.X, r.Max.Y}, item)
	}
	return &RF[T]{tree: tree, f: f}
}

func (rf *RF[T]) FindOneWithPoly(x, y, xDiff, yDiff float64) (*Item[T], error) {
	p := geom.Point{X: x, Y: y}
	var res *Item[T]
	rf.tree.Search(
		[2]float64{x - xDiff, y - yDiff},
		[2]float64{x + xDiff, y + yDiff},
		func(_, _ [2]float64, data *Item[T]) bool {
			if data.Poly.ContainsPoint(p) {
				res = data
				return false
			}
			return true
		},
	)
	if res == nil {
		return nil, ErrNotFound
	}
	return res, nil
}

func (rf *RF[T]) FindOne(x, y, xDiff, yDiff float64) (T, error) {
	res, err := rf.FindOneWithPoly(x, y, xDiff, yDiff)
	if err != nil {
		return *new(T), err
	}
	return res.V, nil
}

func (rf *RF[T]) FindAllWithPoly(x, y, xDiff, yDiff float64) ([]*Item[T], error) {
	p := geom.Point{X: x, Y: y}
	res := make([]*Item[T], 0)
	rf.tree.Search(
		[2]float64{x - xDiff, y - yDiff},
		[2]float64{x + xDiff, y + yDiff},
		func(_, _ [2]float64, data *Item[T]) bool {
			if data.Poly.ContainsPoint(p) {
				res = append(res, data)
			}
			return true
		},
	)
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

func (rf *RF[T]) FindAll(x, y, xDiff, yDiff float64) ([]T, error) {
	p := geom.Point{X: x, Y: y}
	res := make([]T, 0)
	rf.tree.Search(
		[2]float64{x - xDiff, y - yDiff},
		[2]float64{x + xDiff, y + yDiff},
		func(_, _ [2]float64, data *Item[T]) bool {
			if data.Poly.ContainsPoint(p) {
				res = append(res, data.V)
			}
			return true
		},
	)
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}
