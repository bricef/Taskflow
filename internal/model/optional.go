package model

// Optional represents a value that may or may not have been provided.
// Unlike a pointer, it cleanly distinguishes three states:
//   - Not provided:  Optional[T]{Set: false}
//   - Provided nil:  Optional[*string]{Set: true, Value: nil}   (clear the field)
//   - Provided value: Optional[string]{Set: true, Value: "foo"}
type Optional[T any] struct {
	Value T
	Set   bool
}

// Set creates an Optional with the given value.
func Set[T any](v T) Optional[T] {
	return Optional[T]{Value: v, Set: true}
}

// Unset returns an empty Optional (not provided).
func Unset[T any]() Optional[T] {
	return Optional[T]{}
}
