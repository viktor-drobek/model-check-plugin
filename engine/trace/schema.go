package trace

import _ "embed"

// schemaJSON is the public JSON Schema for Document.
//
//go:embed schema.json
var schemaJSON []byte

//go:embed schema-v2.json
var schemaV2JSON []byte

// JSONSchema returns a copy of the versioned document schema.
func JSONSchema() []byte {
	return append([]byte(nil), schemaJSON...)
}

// JSONSchemaV2 returns a copy of the schema-v2 document contract.
func JSONSchemaV2() []byte {
	return append([]byte(nil), schemaV2JSON...)
}
