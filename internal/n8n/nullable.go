package n8n

import (
	"bytes"
	"encoding/json"
)

// Nullable is an optional write field that can also be sent as JSON null,
// for schemas where null means something omission does not (clear a
// description, move a variable to global scope). The zero value is "omitted";
// tag the field with omitzero so it is left out of the request.
type Nullable[T any] struct {
	Value T
	// Null sends an explicit JSON null instead of Value.
	Null bool
	// Set reports that the field is present at all.
	Set bool
}

// SetValue returns a present, non-null field.
func SetValue[T any](value T) Nullable[T] { return Nullable[T]{Value: value, Set: true} }

// SetNull returns a present field sent as JSON null.
func SetNull[T any]() Nullable[T] { return Nullable[T]{Null: true, Set: true} }

// IsZero reports an omitted field, for the omitzero tag option.
func (n Nullable[T]) IsZero() bool { return !n.Set }

// MarshalJSON writes null or the value.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Null || !n.Set {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// UnmarshalJSON records presence, so a strict input document can clear a
// field with null rather than have the null read as "omitted".
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*n = SetNull[T]()
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*n = SetValue(value)
	return nil
}
