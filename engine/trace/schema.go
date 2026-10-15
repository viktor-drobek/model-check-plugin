package trace

import _ "embed"

// schemaJSON is the public JSON Schema for Document.
//
//go:embed schema.json
var schemaJSON []byte

// JSONSchema returns a copy of the versioned document schema.
func JSONSchema() []byte {
	return append([]byte(nil), schemaJSON...)
}
