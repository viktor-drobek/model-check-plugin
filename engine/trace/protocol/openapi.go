package protocol

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"modelcheck/trace"
)

const openAPIAdapterVersion = "1"

// AnalyzeOpenAPI analyzes the supported offline JSON OpenAPI declaration
// profile. It does not open paths, fetch references, or infer runtime behavior.
func AnalyzeOpenAPI(in ContractInput) ([]byte, error) {
	return analyzeOpenAPIWithBudget(in, defaultBudget)
}

func analyzeOpenAPIWithBudget(in ContractInput, limits budget) ([]byte, error) {
	return contractRun(in, "openapi", limits, parseOpenAPI)
}

func parseOpenAPI(data []byte) (contractPlan, error) {
	value, err := contractJSON(data)
	if err != nil {
		return contractPlan{}, err
	}
	root, err := contractObject(value, "openapi", "info", "paths", "components", "servers", "tags", "externalDocs", "security", "jsonSchemaDialect")
	if err != nil {
		return contractPlan{}, err
	}
	version, ok := root["openapi"].(string)
	if !ok {
		return contractPlan{}, contractErr("schema_invalid")
	}
	if version != "3.0.3" && version != "3.1.0" {
		return contractPlan{}, contractErr("unsupported_version")
	}
	if _, err := openAPIInfo(root["info"]); err != nil {
		return contractPlan{}, err
	}
	paths, err := openAPIMap(root["paths"])
	if err != nil {
		return contractPlan{}, contractErr("schema_invalid")
	}
	plan := contractPlan{gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "OpenAPI declarations are not behavior, execution, source linkage, transport correctness, or a model verdict"}}}
	templateShapes := map[string]bool{}
	for _, key := range []string{"servers", "tags", "externalDocs", "security", "jsonSchemaDialect"} {
		if _, exists := root[key]; exists {
			plan.gaps = append(plan.gaps, contractGap{key: contractPointer(key), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI metadata is not mapped by this profile"})
		}
	}
	for _, route := range sortedOpenAPIKeys(paths) {
		if !openAPIRoute(route) {
			return contractPlan{}, contractErr("schema_invalid")
		}
		shape := openAPIRouteShape(route)
		if templateShapes[shape] {
			return contractPlan{}, contractErr("schema_invalid")
		}
		templateShapes[shape] = true
		item, err := contractObject(paths[route], "get", "put", "post", "delete", "options", "head", "patch", "trace", "$ref", "summary", "description", "parameters", "servers")
		if err != nil {
			return contractPlan{}, contractErr("schema_invalid")
		}
		for _, key := range []string{"$ref", "summary", "description", "parameters", "servers"} {
			if _, exists := item[key]; exists {
				plan.gaps = append(plan.gaps, contractGap{key: contractPointer("paths", route, key), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI path-item declaration is not mapped by this profile"})
			}
		}
		for _, method := range openAPIMethods {
			operation, exists := item[method]
			if !exists {
				continue
			}
			if err := openAPIOperation(&plan, route, method, operation); err != nil {
				return contractPlan{}, err
			}
		}
	}
	if components, exists := root["components"]; exists {
		if err := openAPIComponents(&plan, components); err != nil {
			return contractPlan{}, err
		}
	}
	return plan, nil
}

var openAPIMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

func openAPIInfo(value any) (map[string]any, error) {
	info, err := contractObject(value, "title", "version", "description")
	if err != nil {
		return nil, contractErr("schema_invalid")
	}
	for _, key := range []string{"title", "version"} {
		if text, ok := info[key].(string); !ok || text == "" || len(text) > 256 {
			return nil, contractErr("schema_invalid")
		}
	}
	if description, exists := info["description"]; exists {
		if _, ok := description.(string); !ok {
			return nil, contractErr("schema_invalid")
		}
	}
	return info, nil
}

func openAPIOperation(plan *contractPlan, route, method string, value any) error {
	operation, err := contractObject(value, "operationId", "summary", "description", "parameters", "requestBody", "responses", "callbacks", "security", "servers", "tags", "deprecated", "externalDocs")
	if err != nil {
		return contractErr("schema_invalid")
	}
	responses, ok := operation["responses"]
	if !ok {
		return contractErr("schema_invalid")
	}
	responseObject, err := openAPIMap(responses)
	if err != nil || len(responseObject) == 0 {
		return contractErr("schema_invalid")
	}
	plan.gaps = append(plan.gaps, contractGap{key: contractPointer("paths", route, method, "responses"), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI response declarations are not mapped by this profile"})
	for code, response := range responseObject {
		if !openAPIResponseCode(code) {
			return contractErr("schema_invalid")
		}
		if _, err := openAPIMap(response); err != nil {
			return contractErr("schema_invalid")
		}
	}
	label := contractLabel(strings.ToUpper(method)+" ", route)
	if operationID, exists := operation["operationId"]; exists {
		id, ok := operationID.(string)
		if !ok || !contractName(id) {
			return contractErr("schema_invalid")
		}
		label = id
		for _, fact := range plan.facts {
			if fact.kind == trace.Endpoint && fact.label == id {
				return contractErr("schema_invalid")
			}
		}
	}
	key := contractPointer("paths", route, method)
	plan.facts = append(plan.facts, contractFact{key: key, label: label, kind: trace.Endpoint})
	for _, child := range []string{"summary", "description", "parameters", "requestBody", "callbacks", "security", "servers", "tags", "deprecated", "externalDocs"} {
		if _, exists := operation[child]; exists {
			plan.gaps = append(plan.gaps, contractGap{key: contractPointer("paths", route, method, child), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI operation declaration is not mapped by this profile"})
		}
	}
	return nil
}

func openAPIComponents(plan *contractPlan, value any) error {
	components, err := contractObject(value, "schemas", "responses", "parameters", "examples", "requestBodies", "headers", "securitySchemes", "links", "callbacks", "pathItems")
	if err != nil {
		return contractErr("schema_invalid")
	}
	for _, key := range []string{"responses", "parameters", "examples", "requestBodies", "headers", "securitySchemes", "links", "callbacks", "pathItems"} {
		if _, exists := components[key]; exists {
			plan.gaps = append(plan.gaps, contractGap{key: contractPointer("components", key), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI component section is not mapped by this profile"})
		}
	}
	schemas, exists := components["schemas"]
	if !exists {
		return nil
	}
	schemaObject, err := openAPIMap(schemas)
	if err != nil {
		return contractErr("schema_invalid")
	}
	for _, name := range sortedOpenAPIKeys(schemaObject) {
		if !contractName(name) {
			return contractErr("schema_invalid")
		}
		key := contractPointer("components", "schemas", name)
		plan.facts = append(plan.facts, contractFact{key: key, label: name, kind: trace.ProtocolMessage})
		if err := openAPISchema(plan, schemaObject[name], key, true); err != nil {
			return err
		}
	}
	return nil
}

// openAPISchema checks every nested schema object against the profile while
// emitting facts only for direct properties of a component schema.
func openAPISchema(plan *contractPlan, value any, parent string, direct bool) error {
	schema, err := contractObject(value, openAPISchemaKeywords...)
	if err != nil {
		return contractErr("schema_invalid")
	}
	for key := range schema {
		if key != "properties" || !direct {
			plan.gaps = append(plan.gaps, contractGap{key: openAPIChildPointer(parent, key), kind: "openapi.unmapped_declaration", reason: "recognized OpenAPI schema keyword is not mapped by this profile"})
		}
	}
	properties, exists := schema["properties"]
	if exists {
		propertyObject, err := openAPIMap(properties)
		if err != nil {
			return contractErr("schema_invalid")
		}
		for _, name := range sortedOpenAPIKeys(propertyObject) {
			if !contractName(name) {
				return contractErr("schema_invalid")
			}
			propertyKey := openAPIChildPointer(openAPIChildPointer(parent, "properties"), name)
			if direct {
				plan.facts = append(plan.facts, contractFact{key: propertyKey, label: name, parent: parent, kind: trace.BinaryField})
			}
			if err := openAPISchema(plan, propertyObject[name], propertyKey, false); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties", "not"} {
		value, exists := schema[key]
		if !exists {
			continue
		}
		child, ok := value.(map[string]any)
		if !ok || child == nil {
			return contractErr("schema_invalid")
		}
		if err := openAPISchema(plan, child, openAPIChildPointer(parent, key), false); err != nil {
			return err
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		values, exists := schema[key]
		if !exists {
			continue
		}
		items, ok := values.([]any)
		if !ok {
			return contractErr("schema_invalid")
		}
		for index, item := range items {
			child, ok := item.(map[string]any)
			if !ok || child == nil {
				return contractErr("schema_invalid")
			}
			childKey := openAPIChildPointer(openAPIChildPointer(parent, key), strconv.Itoa(index))
			if err := openAPISchema(plan, child, childKey, false); err != nil {
				return err
			}
		}
	}
	return nil
}

var openAPISchemaKeywords = []string{
	"$ref", "title", "description", "type", "format", "enum", "const", "default", "examples", "nullable", "deprecated", "readOnly", "writeOnly", "required", "properties", "items", "additionalProperties", "allOf", "anyOf", "oneOf", "not", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems", "minProperties", "maxProperties", "discriminator", "xml", "externalDocs",
}

func openAPIChildPointer(parent, child string) string {
	return parent + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(child)
}

func openAPIMap(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, contractErr("schema_invalid")
	}
	return object, nil
}

func sortedOpenAPIKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func openAPIRoute(route string) bool {
	if len(route) == 0 || len(route) > 256 || !strings.HasPrefix(route, "/") || strings.ContainsAny(route, "?#\\") {
		return false
	}
	for _, part := range strings.Split(route, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func openAPIRouteShape(route string) string {
	parts := strings.Split(route, "/")
	for i, part := range parts {
		if len(part) >= 2 && part[0] == '{' && part[len(part)-1] == '}' {
			parts[i] = "{}"
		}
	}
	return strings.Join(parts, "/")
}

func openAPIResponseCode(value string) bool {
	if value == "default" {
		return true
	}
	n, err := strconv.Atoi(value)
	return err == nil && n >= 100 && n <= 599 && fmt.Sprintf("%d", n) == value
}
