package ir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MarshalJSON renders m deterministically: fixed struct field order, no
// maps, two-space indentation, trailing newline. The same Model always
// produces the same bytes, which is what `mcd parse` round-trip tests rely
// on.
func MarshalJSON(m *Model) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalJSON parses and validates a Model. Unknown fields are errors so
// that a misspelled key cannot silently drop a guard or an initial value.
func UnmarshalJSON(data []byte) (*Model, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Model
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("ir: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("ir: trailing data after the model")
	}
	if err := Validate(&m); err != nil {
		return nil, fmt.Errorf("ir: %w", err)
	}
	return &m, nil
}
