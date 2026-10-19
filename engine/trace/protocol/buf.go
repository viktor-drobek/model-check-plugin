package protocol

import (
	"strconv"
	"strings"
)

// AnalyzeBuf accepts the narrow JSON-only Buf v2 declaration profile. It never
// invokes Buf, reads modules, or resolves dependencies.
func AnalyzeBuf(input ContractInput) ([]byte, error) {
	return analyzeBuf(input, defaultBudget)
}

func analyzeBufWithBudget(input ContractInput, limits budget) ([]byte, error) {
	return analyzeBuf(input, limits)
}

func analyzeBuf(input ContractInput, limits budget) ([]byte, error) {
	return contractRun(input, "buf", limits, parseBufContract)
}

func parseBufContract(data []byte) (contractPlan, error) {
	value, err := contractJSON(data)
	if err != nil {
		return contractPlan{}, err
	}
	root, err := contractObject(value, "version", "modules", "deps")
	if err != nil {
		return contractPlan{}, err
	}
	version, ok := root["version"].(string)
	if !ok || version != "v2" {
		return contractPlan{}, contractErr("unsupported_version")
	}
	modules, ok := root["modules"].([]any)
	if !ok || len(modules) == 0 || len(modules) > 32 {
		return contractPlan{}, contractErr("schema_invalid")
	}
	if deps, exists := root["deps"]; exists {
		list, ok := deps.([]any)
		if !ok {
			return contractPlan{}, contractErr("schema_invalid")
		}
		if len(list) != 0 {
			return contractPlan{}, contractErr("unsupported_declaration")
		}
	}
	plan := contractPlan{gaps: []contractGap{{key: "#", kind: "buf.declarations_only", reason: "declarations are not Buf execution"}}}
	seen := make(map[string]struct{}, len(modules))
	for i, item := range modules {
		module, err := contractObject(item, "path")
		if err != nil {
			return contractPlan{}, err
		}
		path, ok := module["path"].(string)
		if !ok || !bufLogicalPath(path) {
			return contractPlan{}, contractErr("schema_invalid")
		}
		if _, found := seen[path]; found {
			return contractPlan{}, contractErr("schema_invalid")
		}
		seen[path] = struct{}{}
		plan.gaps = append(plan.gaps, contractGap{key: contractPointer("modules", strconv.Itoa(i)), kind: "buf.module_unresolved", reason: "local module is not resolved"})
	}
	return plan, nil
}

func bufLogicalPath(value string) bool {
	if len(value) == 0 || len(value) > 256 || strings.HasPrefix(value, "/") || strings.Contains(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || !contractName(strings.ReplaceAll(part, "-", "_")) {
			return false
		}
	}
	return true
}
