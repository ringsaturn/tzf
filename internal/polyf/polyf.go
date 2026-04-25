// Package polyf is a zero-external-dependency port of github.com/ringsaturn/polyf.
// It replaces geometry.Poly with internal/geom.Polygon and keeps the same
// generic F[T] / RF[T] API surface.
package polyf

import (
	"errors"

	"github.com/ringsaturn/tzf/internal/geom"
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
