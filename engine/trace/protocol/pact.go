package protocol

import (
	"net/url"
	"strconv"
	"strings"

	"modelcheck/trace"
)

// AnalyzePact projects a narrow, offline Pact v4 declaration into contract
// evidence. It does not run Pact, contact a provider, or evaluate matching rules.
func AnalyzePact(input ContractInput) ([]byte, error) {
	return analyzePactWithBudget(input, defaultBudget)
}

func analyzePactWithBudget(input ContractInput, limits budget) ([]byte, error) {
	return contractRun(input, "pact", limits, planPact)
}

func planPact(data []byte) (contractPlan, error) {
	value, err := contractJSON(data)
	if err != nil {
		return contractPlan{}, err
	}
	root, err := contractObject(value, "consumer", "provider", "metadata", "interactions")
	if err != nil {
		return contractPlan{}, err
	}
	if err := pactNamedParty(root["consumer"]); err != nil {
		return contractPlan{}, err
	}
	if err := pactNamedParty(root["provider"]); err != nil {
		return contractPlan{}, err
	}
	if err := pactMetadata(root["metadata"]); err != nil {
		return contractPlan{}, err
	}
	interactions, ok := root["interactions"].([]any)
	if !ok || len(interactions) < 1 || len(interactions) > 256 {
		return contractPlan{}, contractErr("schema_invalid")
	}

	plan := contractPlan{gaps: []contractGap{{key: "#", kind: "pact.declarations_only", reason: "the declaration records no Pact verification, execution, or conformance result"}}}
	keys := make(map[string]bool, len(interactions))
	for index, raw := range interactions {
		pointer := contractPointer("interactions", strconv.Itoa(index))
		kind, err := pactInteractionKind(raw)
		if err != nil {
			return contractPlan{}, err
		}
		interaction, err := pactInteractionObject(raw, kind)
		if err != nil {
			return contractPlan{}, err
		}
		key, ok := interaction["key"].(string)
		if !ok || !contractName(key) || keys[key] {
			return contractPlan{}, contractErr("schema_invalid")
		}
		keys[key] = true
		description, ok := interaction["description"].(string)
		if !ok || !pactDescription(description) {
			return contractPlan{}, contractErr("schema_invalid")
		}
		if pending, present := interaction["pending"]; present {
			if _, ok := pending.(bool); !ok {
				return contractPlan{}, contractErr("schema_invalid")
			}
		}
		for _, name := range []string{"pending", "providerStates", "comments", "transport"} {
			if _, present := interaction[name]; present {
				plan.gaps = append(plan.gaps, contractGap{key: pointer + "/" + name, kind: "pact.unmapped_declaration", reason: "the Pact declaration is outside the narrow offline profile"})
			}
		}
		switch kind {
		case "Synchronous/HTTP":
			if err := pactHTTPInteraction(&plan, interaction, pointer); err != nil {
				return contractPlan{}, err
			}
		case "Asynchronous/Messages", "Synchronous/Messages":
			if err := pactMessageInteraction(&plan, interaction, pointer); err != nil {
				return contractPlan{}, err
			}
		}
	}
	return plan, nil
}

func pactNamedParty(value any) error {
	party, err := contractObject(value, "name")
	if err != nil {
		return err
	}
	name, ok := party["name"].(string)
	if !ok || !pactText(name) {
		return contractErr("schema_invalid")
	}
	return nil
}

func pactInteractionKind(value any) (string, error) {
	interaction, ok := value.(map[string]any)
	if !ok {
		return "", contractErr("schema_invalid")
	}
	kind, ok := interaction["type"].(string)
	if !ok || !pactInteractionType(kind) {
		return "", contractErr("schema_invalid")
	}
	return kind, nil
}

func pactInteractionObject(value any, kind string) (map[string]any, error) {
	allowed := []string{"key", "type", "description", "pending", "providerStates", "comments", "transport"}
	switch kind {
	case "Synchronous/HTTP":
		allowed = append(allowed, "request", "response")
	case "Asynchronous/Messages", "Synchronous/Messages":
		allowed = append(allowed, "contents", "request", "response")
	default:
		return nil, contractErr("schema_invalid")
	}
	return contractObject(value, allowed...)
}

func pactMetadata(value any) error {
	metadata, err := contractObject(value, "pactSpecification")
	if err != nil {
		return err
	}
	specification, err := contractObject(metadata["pactSpecification"], "version")
	if err != nil {
		return err
	}
	version, ok := specification["version"].(string)
	if !ok {
		return contractErr("schema_invalid")
	}
	if version != "4.0" {
		return contractErr("unsupported_version")
	}
	return nil
}

func pactHTTPInteraction(plan *contractPlan, interaction map[string]any, pointer string) error {
	request, err := pactHTTPMessage(interaction["request"], true)
	if err != nil {
		return err
	}
	response, err := pactHTTPMessage(interaction["response"], false)
	if err != nil {
		return err
	}
	endpointKey := pointer
	plan.facts = append(plan.facts,
		contractFact{key: endpointKey, label: "Pact HTTP interaction", kind: trace.Endpoint},
		contractFact{key: pointer + "/request", label: "Pact HTTP request", parent: endpointKey, kind: trace.ProtocolMessage},
		contractFact{key: pointer + "/response", label: "Pact HTTP response", parent: endpointKey, kind: trace.ProtocolMessage},
	)
	for _, message := range []struct {
		name string
		data map[string]any
	}{{"request", request}, {"response", response}} {
		for _, opaque := range []string{"query", "headers", "body", "matchingRules", "generators"} {
			if _, present := message.data[opaque]; present {
				plan.gaps = append(plan.gaps, contractGap{key: pointer + "/" + message.name + "/" + opaque, kind: "pact.unmapped_declaration", reason: "the Pact declaration is not executed or interpreted by this profile"})
			}
		}
	}
	return nil
}

func pactHTTPMessage(value any, request bool) (map[string]any, error) {
	allowed := []string{"headers", "body", "matchingRules", "generators"}
	if request {
		allowed = append([]string{"method", "path", "query"}, allowed...)
	} else {
		allowed = append([]string{"status"}, allowed...)
	}
	message, err := contractObject(value, allowed...)
	if err != nil {
		return nil, err
	}
	if request {
		method, ok := message["method"].(string)
		path, pathOK := message["path"].(string)
		if !ok || !pactHTTPMethod(method) || !pathOK || !pactRoute(path) {
			return nil, contractErr("schema_invalid")
		}
	} else {
		status, ok := message["status"].(jsonNumber)
		if !ok || !pactStatus(status) {
			return nil, contractErr("schema_invalid")
		}
	}
	return message, nil
}

// jsonNumber keeps pact.go independent of the decoder's numeric representation.
type jsonNumber interface{ String() string }

func pactStatus(number jsonNumber) bool {
	n, err := strconv.Atoi(number.String())
	return err == nil && strconv.Itoa(n) == number.String() && n >= 100 && n <= 599
}

func pactMessageInteraction(plan *contractPlan, interaction map[string]any, pointer string) error {
	count := 0
	for _, field := range []string{"contents", "request", "response"} {
		if value, present := interaction[field]; present {
			switch value.(type) {
			case map[string]any, []any:
				plan.gaps = append(plan.gaps, contractGap{key: pointer + "/" + field, kind: "pact.unmapped_declaration", reason: "message declarations are recognised but not mapped by this profile"})
				count++
			default:
				return contractErr("schema_invalid")
			}
		}
	}
	if count == 0 {
		return contractErr("schema_invalid")
	}
	return nil
}

func pactInteractionType(value string) bool {
	switch value {
	case "Synchronous/HTTP", "Asynchronous/Messages", "Synchronous/Messages":
		return true
	default:
		return false
	}
}

func pactHTTPMethod(value string) bool {
	switch value {
	case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH":
		return true
	default:
		return false
	}
}

func pactRoute(value string) bool {
	if len(value) == 0 || len(value) > 256 || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#\\") || strings.Contains(value, "//") {
		return false
	}
	for _, segment := range strings.Split(value, "/")[1:] {
		if segment == "." || segment == ".." {
			return false
		}
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Path == value
}

func pactDescription(value string) bool {
	return pactText(value)
}

func pactText(value string) bool {
	return len(value) > 0 && len(value) <= 256 && strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}
