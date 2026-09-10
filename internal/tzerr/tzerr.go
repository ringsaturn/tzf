// Package tzerr holds sentinel errors shared between the root tzf package and
// the internal finder implementations, so a single value backs errors.Is
// regardless of which implementation produced the error.
package tzerr

import "errors"

// ErrNoTimezoneFound is returned when a lookup or export finds no timezone.
// The root package re-exports it as tzf.ErrNoTimezoneFound.
var ErrNoTimezoneFound = errors.New("tzf: no timezone found")
