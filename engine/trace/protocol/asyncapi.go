package protocol

import (
	"sort"
	"strconv"
	"strings"

	"modelcheck/trace"
)

// AnalyzeAsyncAPI records direct declarations from the narrow offline AsyncAPI
// 2.6.0/3.0.0 JSON profile. It neither expands references nor infers behaviour.
func AnalyzeAsyncAPI(in ContractInput) ([]byte, error) {
	return analyzeAsyncAPIWithBudget(in, defaultBudget)
}

func analyzeAsyncAPIWithBudget(in ContractInput, limits budget) ([]byte, error) {
	return contractRun(in, "asyncapi", limits, planAsyncAPI)
}

func planAsyncAPI(data []byte) (contractPlan, error) {
	value, err := contractJSON(data)
	if err != nil {
		return contractPlan{}, err
	}
	root, err := contractObject(value, "asyncapi", "id", "info", "servers", "defaultContentType", "channels", "operations", "components", "tags", "externalDocs")
	if err != nil {
		return contractPlan{}, err
	}
	version, ok := root["asyncapi"].(string)
	if !ok {
		return contractPlan{}, contractErr("schema_invalid")
	}
	if version != "2.6.0" && version != "3.0.0" {
		return contractPlan{}, contractErr("unsupported_version")
	}
	info, err := contractObject(root["info"], "title", "version", "description")
	if err != nil || !asyncAPIText(info["title"]) || !asyncAPIText(info["version"]) {
		return contractPlan{}, contractErr("schema_invalid")
	}
	channels, ok := root["channels"].(map[string]any)
	if !ok {
		return contractPlan{}, contractErr("schema_invalid")
	}
	// contractRun independently derives the exact profile for identity emission.
	// The profile requires exactly one document-level declarations-only scope gap.
	plan := contractPlan{gaps: []contractGap{{key: "#", kind: "asyncapi.declarations_only", reason: "declarations are contract evidence, not execution or conformance"}}}
	if _, found := info["description"]; found {
		asyncAPIGap(&plan, contractPointer("info", "description"), "asyncapi.unmapped_declaration", "recognised document description is not mapped by this profile")
	}
	seenOperationIDs := map[string]bool{}
	for _, name := range asyncAPISortedKeys(channels) {
		if !asyncAPIChannelName(name) {
			return contractPlan{}, contractErr("schema_invalid")
		}
		pointer := contractPointer("channels", name)
		allowed := []string{"description", "subscribe", "publish", "parameters", "bindings", "servers"}
		if version == "3.0.0" {
			allowed = []string{"address", "messages", "description", "parameters", "bindings", "servers", "tags", "externalDocs", "title", "summary"}
		}
		channel, err := contractObject(channels[name], allowed...)
		if err != nil {
			return contractPlan{}, err
		}
		plan.facts = append(plan.facts, contractFact{key: pointer, label: contractLabel("AsyncAPI channel ", name), kind: trace.Endpoint})
		if version == "2.6.0" {
			if err := asyncAPIPlanV2Channel(&plan, pointer, channel, seenOperationIDs); err != nil {
				return contractPlan{}, err
			}
		} else if err := asyncAPIPlanV3Channel(&plan, pointer, channel); err != nil {
			return contractPlan{}, err
		}
	}
	if version == "2.6.0" {
		if _, found := root["operations"]; found {
			return contractPlan{}, contractErr("schema_invalid")
		}
	} else if operations, found := root["operations"]; found {
		if err := asyncAPIPlanV3Operations(&plan, operations, channels); err != nil {
			return contractPlan{}, err
		}
	}
	if components, found := root["components"]; found {
		if err := asyncAPIPlanComponents(&plan, components); err != nil {
			return contractPlan{}, err
		}
	}
	for _, key := range []string{"id", "servers", "defaultContentType", "tags", "externalDocs"} {
		if _, found := root[key]; found {
			asyncAPIGap(&plan, contractPointer(key), "asyncapi.unmapped_declaration", "recognised root declaration is not mapped by this profile")
		}
	}
	return plan, nil
}

func asyncAPIPlanV2Channel(plan *contractPlan, pointer string, channel map[string]any, seenOperationIDs map[string]bool) error {
	for _, action := range []string{"publish", "subscribe"} {
		value, found := channel[action]
		if !found {
			continue
		}
		opPointer := pointer + "/" + action
		op, err := contractObject(value, "operationId", "message", "summary", "description", "tags", "externalDocs", "bindings", "traits", "security")
		if err != nil {
			return err
		}
		if id, found := op["operationId"]; found {
			if !asyncAPIText(id) || !contractName(id.(string)) {
				return contractErr("schema_invalid")
			}
			// The operation gap may contain only references, but this identifier
			// is still consumed and must not inherit that gap's URL exception.
			lower := strings.ToLower(id.(string))
			if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
				return contractErrAt("schema_invalid", "json")
			}
			if seenOperationIDs[id.(string)] {
				return contractErr("schema_invalid")
			}
			seenOperationIDs[id.(string)] = true
		}
		asyncAPIGap(plan, opPointer, "asyncapi.operation_unmapped", "operation declaration is not transport behaviour evidence")
		message, found := op["message"]
		if !found {
			return contractErr("schema_invalid")
		}
		if err := asyncAPIPlanMessage(plan, opPointer+"/message", pointer, message); err != nil {
			return err
		}
		for _, key := range []string{"summary", "description", "tags", "externalDocs", "bindings", "traits", "security"} {
			if _, found := op[key]; found {
				asyncAPIGap(plan, opPointer+"/"+key, "asyncapi.unmapped_declaration", "recognised operation declaration is not mapped by this profile")
			}
		}
	}
	for _, key := range []string{"description", "parameters", "bindings", "servers"} {
		if _, found := channel[key]; found {
			asyncAPIGap(plan, pointer+"/"+key, "asyncapi.unmapped_declaration", "recognised channel declaration is not mapped by this profile")
		}
	}
	return nil
}

func asyncAPIPlanV3Channel(plan *contractPlan, pointer string, channel map[string]any) error {
	if values, found := channel["messages"]; found {
		messages, ok := values.(map[string]any)
		if !ok {
			return contractErr("schema_invalid")
		}
		for _, name := range asyncAPISortedKeys(messages) {
			if !contractName(name) {
				return contractErr("schema_invalid")
			}
			if err := asyncAPIPlanMessage(plan, pointer+"/messages/"+asyncAPIPointerToken(name), pointer, messages[name]); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"address", "description", "parameters", "bindings", "servers", "tags", "externalDocs", "title", "summary"} {
		if _, found := channel[key]; found {
			asyncAPIGap(plan, pointer+"/"+key, "asyncapi.unmapped_declaration", "recognised channel declaration is not mapped by this profile")
		}
	}
	return nil
}

func asyncAPIPlanV3Operations(plan *contractPlan, value any, channels map[string]any) error {
	operations, ok := value.(map[string]any)
	if !ok {
		return contractErr("schema_invalid")
	}
	for _, name := range asyncAPISortedKeys(operations) {
		if !contractName(name) {
			return contractErr("schema_invalid")
		}
		pointer := contractPointer("operations", name)
		op, err := contractObject(operations[name], "action", "channel", "title", "summary", "description", "tags", "externalDocs", "bindings", "traits", "security", "messages")
		if err != nil {
			return err
		}
		action, ok := op["action"].(string)
		if !ok || (action != "send" && action != "receive") {
			return contractErr("schema_invalid")
		}
		channel, err := contractObject(op["channel"], "$ref")
		if err != nil || len(channel) != 1 {
			return contractErr("schema_invalid")
		}
		ref, ok := channel["$ref"].(string)
		if !ok || !strings.HasPrefix(ref, "#/channels/") {
			return contractErr("schema_invalid")
		}
		namePart := strings.TrimPrefix(ref, "#/channels/")
		if strings.Contains(namePart, "/") {
			return contractErr("schema_invalid")
		}
		resolved := strings.NewReplacer("~1", "/", "~0", "~").Replace(namePart)
		if _, found := channels[resolved]; !found {
			return contractErr("schema_invalid")
		}
		asyncAPIGap(plan, pointer, "asyncapi.operation_unmapped", "operation action is not transport behaviour evidence")
		if value, found := op["messages"]; found {
			messages, ok := value.([]any)
			if !ok {
				return contractErr("schema_invalid")
			}
			for index, value := range messages {
				message, err := contractObject(value, "$ref")
				if err != nil || len(message) != 1 || !asyncAPIText(message["$ref"]) {
					return contractErr("schema_invalid")
				}
				asyncAPIGap(plan, pointer+"/messages/"+strconv.Itoa(index), "asyncapi.unmapped_declaration", "operation message reference is not expanded by this profile")
			}
		}
		for _, key := range []string{"title", "summary", "description", "tags", "externalDocs", "bindings", "traits", "security", "messages"} {
			if _, found := op[key]; found {
				asyncAPIGap(plan, pointer+"/"+key, "asyncapi.unmapped_declaration", "recognised operation declaration is not mapped by this profile")
			}
		}
	}
	return nil
}

func asyncAPIPlanComponents(plan *contractPlan, value any) error {
	components, err := contractObject(value, "messages", "schemas", "securitySchemes", "parameters", "correlationIds", "operationTraits", "messageTraits", "serverBindings", "channelBindings", "operationBindings", "messageBindings")
	if err != nil {
		return err
	}
	for _, collection := range []string{"messages", "schemas"} {
		values, found := components[collection]
		if !found {
			continue
		}
		object, ok := values.(map[string]any)
		if !ok {
			return contractErr("schema_invalid")
		}
		for _, name := range asyncAPISortedKeys(object) {
			if !contractName(name) {
				return contractErr("schema_invalid")
			}
			pointer := contractPointer("components", collection, name)
			if collection == "messages" {
				if err := asyncAPIPlanMessage(plan, pointer, "", object[name]); err != nil {
					return err
				}
			} else {
				plan.facts = append(plan.facts, contractFact{key: pointer, label: "AsyncAPI schema " + name, kind: trace.ProtocolMessage})
				if err := asyncAPIPlanSchema(plan, pointer, pointer, object[name], true); err != nil {
					return err
				}
			}
		}
	}
	for key := range components {
		if key != "messages" && key != "schemas" {
			asyncAPIGap(plan, contractPointer("components", key), "asyncapi.unmapped_declaration", "recognised component declaration is not mapped by this profile")
		}
	}
	return nil
}

func asyncAPIPlanMessage(plan *contractPlan, pointer, parent string, value any) error {
	message, err := contractObject(value, "$ref", "name", "title", "summary", "description", "headers", "payload", "correlationId", "contentType", "tags", "externalDocs", "bindings", "examples", "traits")
	if err != nil {
		return err
	}
	if ref, found := message["$ref"]; found {
		if len(message) != 1 || !asyncAPIText(ref) {
			return contractErr("schema_invalid")
		}
		asyncAPIGap(plan, pointer, "asyncapi.reference_unexpanded", "local declaration reference was checked but not expanded")
		return nil
	}
	label := "AsyncAPI message"
	if name, found := message["name"]; found {
		if !asyncAPIText(name) || !contractName(name.(string)) {
			return contractErr("schema_invalid")
		}
		label += " " + name.(string)
	}
	plan.facts = append(plan.facts, contractFact{key: pointer, label: label, parent: parent, kind: trace.ProtocolMessage})
	if payload, found := message["payload"]; found {
		if err := asyncAPIPlanSchema(plan, pointer+"/payload", pointer, payload, true); err != nil {
			return err
		}
	}
	for _, key := range []string{"title", "summary", "description", "headers", "correlationId", "contentType", "tags", "externalDocs", "bindings", "examples", "traits"} {
		if _, found := message[key]; found {
			asyncAPIGap(plan, pointer+"/"+key, "asyncapi.unmapped_declaration", "recognised message declaration is not mapped by this profile")
		}
	}
	return nil
}

// asyncAPIPlanSchema admits only the profile's safe, non-executing schema
// keywords. BinaryField facts are emitted solely for the direct properties of
// a message payload or component schema; nested schemas remain gap-only.
func asyncAPIPlanSchema(plan *contractPlan, pointer, parent string, value any, direct bool) error {
	schema, err := contractObject(value, openAPISchemaKeywords...)
	if err != nil {
		return err
	}
	if ref, found := schema["$ref"]; found {
		if len(schema) != 1 || !asyncAPIText(ref) {
			return contractErr("schema_invalid")
		}
		asyncAPIGap(plan, pointer, "asyncapi.reference_unexpanded", "local declaration reference was checked but not expanded")
		return nil
	}
	if raw, found := schema["type"]; found && !asyncAPIText(raw) {
		return contractErr("schema_invalid")
	}
	if properties, found := schema["properties"]; found {
		object, ok := properties.(map[string]any)
		if !ok {
			return contractErr("schema_invalid")
		}
		for _, name := range asyncAPISortedKeys(object) {
			if !contractName(name) {
				return contractErr("schema_invalid")
			}
			// Nested properties emit gap anchors even when they produce no facts.
			lower := strings.ToLower(name)
			if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
				return contractErrAt("schema_invalid", "json")
			}
			property := pointer + "/properties/" + asyncAPIPointerToken(name)
			if direct {
				plan.facts = append(plan.facts, contractFact{key: property, label: "AsyncAPI field " + name, parent: parent, kind: trace.BinaryField})
			}
			if err := asyncAPIPlanSchema(plan, property, parent, object[name], false); err != nil {
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
		if err := asyncAPIPlanSchema(plan, pointer+"/"+key, parent, child, false); err != nil {
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
			childPointer := pointer + "/" + key + "/" + strconv.Itoa(index)
			if err := asyncAPIPlanSchema(plan, childPointer, parent, child, false); err != nil {
				return err
			}
		}
	}
	for _, key := range openAPISchemaKeywords {
		if (key == "items" || key == "additionalProperties" || key == "not") && asyncAPIReferenceOnly(schema[key]) {
			continue
		}
		if key == "properties" && direct {
			continue
		}
		if _, found := schema[key]; found {
			asyncAPIGap(plan, pointer+"/"+key, "asyncapi.unmapped_declaration", "recognised schema declaration is not mapped by this profile")
		}
	}
	return nil
}

func asyncAPIReferenceOnly(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return false
	}
	ref, ok := object["$ref"].(string)
	return ok && ref != ""
}

func asyncAPIGap(plan *contractPlan, key, kind, reason string) {
	plan.gaps = append(plan.gaps, contractGap{key: key, kind: kind, reason: reason})
}
func asyncAPIText(value any) bool {
	text, ok := value.(string)
	return ok && len(text) > 0 && len(text) <= 256
}
func asyncAPIChannelName(name string) bool {
	lower := strings.ToLower(name)
	if len(name) == 0 || len(name) > 256 || strings.TrimSpace(name) != name || strings.Contains(lower, "http:") || strings.Contains(lower, "https:") || strings.HasPrefix(name, "/") || strings.Contains(name, "//") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" {
			return false
		}
	}
	return true
}
func asyncAPIPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}
func asyncAPISortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
