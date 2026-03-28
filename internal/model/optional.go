package model

import "encoding/json"

// Optional represents a value that may or may not have been provided.
// Unlike a pointer, it cleanly distinguishes three states:
//   - Not provided:  Optional[T]{Set: false}
//   - Provided nil:  Optional[*string]{Set: true, Value: nil}   (clear the field)
//   - Provided value: Optional[string]{Set: true, Value: "foo"}
//
// Implements json.Unmarshaler: when a JSON field is present, Set becomes true.
// When a JSON field is absent, UnmarshalJSON is never called, so Set stays false.
type Optional[T any] struct {
	Value T
	Set   bool
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	return json.Unmarshal(data, &o.Value)
}

func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Set {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// Set creates an Optional with the given value.
func Set[T any](v T) Optional[T] {
	return Optional[T]{Value: v, Set: true}
}

// Unset returns an empty Optional (not provided).
func Unset[T any]() Optional[T] {
	return Optional[T]{}
}
