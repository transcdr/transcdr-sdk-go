package transcdr

import (
	"bytes"
	"encoding/json"
)

// Nullable is a request field with three states: left out (the zero value),
// an explicit JSON null, which clears the value where the API allows it, or a
// value. Fields of this type are tagged `omitzero`.
//
//	transcdr.BillingSettingsParams{MonthlyLimitCents: transcdr.Null[int64]()} // remove the limit
//	transcdr.BillingSettingsParams{MonthlyLimitCents: transcdr.Value[int64](50_000)}
type Nullable[T any] struct {
	value T
	state uint8 // 0 unset, 1 null, 2 value
}

// Value is a Nullable holding v.
func Value[T any](v T) Nullable[T] { return Nullable[T]{value: v, state: 2} }

// Null is a Nullable sent as JSON null.
func Null[T any]() Nullable[T] { return Nullable[T]{state: 1} }

// IsZero reports that the field is left out; `omitzero` drops it.
func (n Nullable[T]) IsZero() bool { return n.state == 0 }

// IsNull reports an explicit null.
func (n Nullable[T]) IsNull() bool { return n.state == 1 }

// Get returns the value and whether there is one.
func (n Nullable[T]) Get() (T, bool) { return n.value, n.state == 2 }

// MarshalJSON writes null or the value.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.state != 2 {
		return []byte("null"), nil
	}
	return json.Marshal(n.value)
}

// UnmarshalJSON reads null or a value.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		*n = Null[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = Value(v)
	return nil
}

// String returns a pointer to s, for optional request fields.
func String(s string) *string { return &s }

// Int returns a pointer to n.
func Int(n int) *int { return &n }

// Int64 returns a pointer to n.
func Int64(n int64) *int64 { return &n }

// Float64 returns a pointer to f.
func Float64(f float64) *float64 { return &f }

// Bool returns a pointer to b.
func Bool(b bool) *bool { return &b }
