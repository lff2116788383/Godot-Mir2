package common

// StaticVector mirrors the semantic part of C++ StaticVector<T, Capacity>.
// Encoding is message-specific because the C++ element width and the
// surrounding packed struct determine the exact wire layout.
type StaticVector[T any] struct {
	Limit int
	Data  []T
}

// NewStaticVector creates a vector with the requested legacy capacity.
func NewStaticVector[T any](limit int) StaticVector[T] {
	return StaticVector[T]{Limit: limit}
}

// Capacity returns the configured compile-time capacity equivalent.
func (v StaticVector[T]) Capacity() int {
	return v.Limit
}

// Len returns the current number of elements.
func (v StaticVector[T]) Len() int {
	return len(v.Data)
}

// Empty reports whether the vector has no elements.
func (v StaticVector[T]) Empty() bool {
	return len(v.Data) == 0
}

// Clear removes all elements.
func (v *StaticVector[T]) Clear() {
	v.Data = nil
}

// Push appends one element after checking the static capacity.
func (v *StaticVector[T]) Push(value T) error {
	if v.Limit < 0 || len(v.Data) >= v.Limit {
		return ErrStaticCollectionFull
	}
	v.Data = append(v.Data, value)
	return nil
}

// Assign replaces all elements after checking the static capacity.
func (v *StaticVector[T]) Assign(values []T) error {
	if v.Limit < 0 || len(values) > v.Limit {
		return ErrStaticCollectionFull
	}
	v.Data = append(v.Data[:0], values...)
	return nil
}

// Slice returns the current elements.
func (v StaticVector[T]) Slice() []T {
	return v.Data
}
