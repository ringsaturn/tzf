package model

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// magic prefixes every serialized intermediate so a stale protobuf .bin (or
// any other file) fails loudly instead of decoding into garbage.
const magic = "TZFGOB1\n"

// The union members register under stable short names, decoupled from this
// package's import path, so a converter elsewhere (e.g. the frozen v1 line
// translating protobuf-era files) can emit compatible streams by registering
// structurally identical types under the same names.
func init() {
	gob.RegisterName("tzf.RingSegment_Inline", &RingSegment_Inline{})
	gob.RegisterName("tzf.RingSegment_EdgeForward", &RingSegment_EdgeForward{})
	gob.RegisterName("tzf.RingSegment_EdgeReversed", &RingSegment_EdgeReversed{})
	gob.RegisterName("tzf.CompressedRingSegment_Inline", &CompressedRingSegment_Inline{})
	gob.RegisterName("tzf.CompressedRingSegment_EdgeForward", &CompressedRingSegment_EdgeForward{})
	gob.RegisterName("tzf.CompressedRingSegment_EdgeReversed", &CompressedRingSegment_EdgeReversed{})
}

// Marshal serializes one pipeline intermediate (pass a pointer).
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(magic)
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, fmt.Errorf("model: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// Unmarshal deserializes data produced by Marshal into v (a pointer).
func Unmarshal(data []byte, v any) error {
	if len(data) < len(magic) || string(data[:len(magic)]) != magic {
		return fmt.Errorf("model: not a TZF pipeline intermediate (missing %q header — protobuf-era .bin files must be regenerated)", magic[:len(magic)-1])
	}
	if err := gob.NewDecoder(bytes.NewReader(data[len(magic):])).Decode(v); err != nil {
		return fmt.Errorf("model: decode: %w", err)
	}
	return nil
}

// Clone deep-copies a pipeline intermediate through the codec.
func Clone[T any](v *T) *T {
	data, err := Marshal(v)
	if err != nil {
		panic(err) // all model types are gob-encodable
	}
	out := new(T)
	if err := Unmarshal(data, out); err != nil {
		panic(err)
	}
	return out
}
