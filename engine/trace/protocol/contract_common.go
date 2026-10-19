package protocol

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"modelcheck/trace"
)

const contractAdapterVersion = "1"

// Pointers expand to at most 2048*6 JSON bytes; labels/reasons add 256*6,
// paths add 512, leaving over 2 KiB for fixed IDs, provenance and indentation.
const contractMaxEncodedRecord = uint64(16 << 10)

// ContractInput contains an offline contract artifact and its caller-supplied
// lowercase SHA-256 digest. The artifact path is logical and is never opened.
type ContractInput struct {
	Artifact      Input
	ContentSHA256 string
}

type contractFact struct {
	key, label, parent string
	kind               trace.NodeKind
}

type contractGap struct {
	key, kind, reason string
}

type contractPlan struct {
	facts []contractFact
	gaps  []contractGap
}

// contractErr returns a Stage 4 error with a constant, non-secret locator.
func contractErr(code string) error {
	return &RuleError{Code: "stage4." + code, Locator: "schema"}
}

func contractErrAt(code, locator string) error {
	switch locator {
	case "input", "metadata", "digest", "json", "schema", "budget":
	default:
		locator = "schema"
	}
	return &RuleError{Code: "stage4." + code, Locator: locator}
}

// contractObject requires an object whose keys are all explicitly allowed.
func contractObject(value any, allowed ...string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, contractErr("schema_invalid")
	}
	permitted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		permitted[key] = struct{}{}
	}
	for key := range object {
		if _, ok := permitted[key]; !ok {
			return nil, contractErr("schema_invalid")
		}
	}
	return object, nil
}

// contractName reports whether s is a bounded stable contract identifier.
func contractName(s string) bool {
	if len(s) == 0 || len(s) > 200 {
		return false
	}
	for i, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == ':' || r == '-') {
			return false
		}
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// contractLabel keeps admitted names within the ledger label limit.
func contractLabel(prefix, name string) string {
	if len(prefix)+len(name) > 256 {
		return name
	}
	return prefix + name
}

// contractPointer returns a canonical local JSON Pointer from unescaped parts.
func contractPointer(parts ...string) string {
	if len(parts) == 0 {
		return "#"
	}
	encoded := make([]string, len(parts))
	for i, part := range parts {
		encoded[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(part)
	}
	return "#/" + strings.Join(encoded, "/")
}

// contractJSON parses strict JSON then scans every decoded value for secrets,
// host paths, unsafe references, and local-reference cycles.
func contractJSON(data []byte) (any, error) {
	value, err := decodeJSON(data)
	if err != nil {
		return nil, contractRecode(err)
	}
	state := contractJSONState{root: value, values: 0, refs: 0, targets: make(map[string]any)}
	if err := state.scan(value, false); err != nil {
		return nil, err
	}
	if err := state.resolveReferences(); err != nil {
		return nil, err
	}
	return value, nil
}

func contractRecode(err error) error {
	if rule, ok := err.(*RuleError); ok {
		if strings.HasPrefix(rule.Code, "stage4.") {
			return contractErrAt(strings.TrimPrefix(rule.Code, "stage4."), rule.Locator)
		}
		switch rule.Code {
		case "stage3b.secret_indicator":
			return contractErrAt("secret_indicator", "json")
		case "stage3b.preflight_limit":
			return contractErrAt("preflight_limit", "budget")
		}
	}
	return contractErrAt("schema_invalid", "json")
}

type contractJSONState struct {
	root    any
	values  int
	refs    int
	targets map[string]any
	links   map[string][]string
}

func (s *contractJSONState) scan(value any, skipPathCheck bool) error {
	return s.scanAt(value, skipPathCheck, "#")
}

func (s *contractJSONState) scanAt(value any, skipPathCheck bool, at string) error {
	s.values++
	if s.values > 4096 {
		return contractErrAt("preflight_limit", "budget")
	}
	switch item := value.(type) {
	case map[string]any:
		for _, key := range contractSortedKeys(item) {
			child := item[key]
			if secretValue(key) {
				return contractErrAt("secret_indicator", "json")
			}
			route := s.openAPIRouteKey(at)
			if (route && (!openAPIRoute(key) || contractUnsafeRoutePath(key))) || (!route && contractUnsafePath(key)) {
				return contractErrAt("schema_invalid", "json")
			}
			if err := s.scanAt(child, key == "$ref", openAPIChildPointer(at, key)); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range item {
			if err := s.scanAt(child, false, fmt.Sprintf("%s/%d", at, index)); err != nil {
				return err
			}
		}
	case string:
		if secretValue(item) {
			return contractErrAt("secret_indicator", "json")
		}
		route := s.pactRouteValue(at)
		if (route && !pactRoute(item)) || (!route && !skipPathCheck && contractUnsafePath(item)) {
			return contractErrAt("schema_invalid", "json")
		}
	}
	return nil
}

func contractUnsafeRoutePath(value string) bool {
	return unsafeFileReference(strings.ToLower(value)) || unsafeDrivePath(value)
}

func contractSortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *contractJSONState) openAPIRouteKey(at string) bool {
	root, ok := s.root.(map[string]any)
	if !ok || at != "#/paths" {
		return false
	}
	_, ok = root["openapi"].(string)
	return ok
}

func (s *contractJSONState) pactRouteValue(at string) bool {
	parts := strings.Split(at, "/")
	if len(parts) != 5 || parts[1] != "interactions" || parts[3] != "request" || parts[4] != "path" {
		return false
	}
	value, ok := resolveContractPointer(s.root, strings.Join(parts[:3], "/"))
	if !ok {
		return false
	}
	interaction, ok := value.(map[string]any)
	return ok && interaction["type"] == "Synchronous/HTTP"
}

func contractUnsafePath(value string) bool {
	lower := strings.ToLower(value)
	if unsafeFileReference(lower) || strings.HasPrefix(value, `\\`) || unsafeDrivePath(value) {
		return true
	}
	for _, prefix := range []string{"/home/", "/root/", "/tmp/", "/etc/", "/var/", "/data/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, component := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if component == "." || component == ".." {
			return true
		}
	}
	return false
}

func unsafeFileReference(value string) bool {
	for start := 0; start < len(value); {
		at := strings.Index(value[start:], "file:")
		if at < 0 {
			return false
		}
		at += start
		end := at + len("file:")
		if end < len(value) && (value[end] == '/' || value[end] == '\\') {
			return true
		}
		start = end
	}
	return false
}

func unsafeDrivePath(value string) bool {
	for i := 0; i+2 < len(value); i++ {
		if i > 0 && ((value[i-1] >= 'a' && value[i-1] <= 'z') || (value[i-1] >= 'A' && value[i-1] <= 'Z')) {
			continue
		}
		if ((value[i] >= 'a' && value[i] <= 'z') || (value[i] >= 'A' && value[i] <= 'Z')) && value[i+1] == ':' && (value[i+2] == '/' || value[i+2] == '\\') {
			return true
		}
	}
	return false
}

func (s *contractJSONState) resolveReferences() error {
	refs := make(map[string][]string)
	if err := collectContractRefs(s.root, "#", refs, &s.refs); err != nil {
		return err
	}
	for _, targets := range refs {
		for _, target := range targets {
			value, ok := resolveContractPointer(s.root, target)
			if !ok {
				return contractErrAt("schema_invalid", "json")
			}
			if _, ok := value.(map[string]any); !ok {
				return contractErrAt("schema_invalid", "json")
			}
			s.targets[target] = value
		}
	}
	// Model only reference relationships. A target depends on every reference
	// nested beneath it, so cycles through properties and other containers are
	// found without expanding declarations into facts.
	graph := make(map[string][]string, len(s.targets))
	for target := range s.targets {
		for from, nested := range refs {
			if from != target && !strings.HasPrefix(from, target+"/") {
				continue
			}
			graph[target] = append(graph[target], nested...)
		}
	}
	visited, active := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(node string) error {
		if active[node] {
			return contractErrAt("schema_invalid", "json")
		}
		if visited[node] {
			return nil
		}
		visited[node], active[node] = true, true
		for _, target := range graph[node] {
			if err := visit(target); err != nil {
				return err
			}
		}
		delete(active, node)
		return nil
	}
	for node := range graph {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}

func collectContractRefs(value any, at string, refs map[string][]string, count *int) error {
	switch item := value.(type) {
	case map[string]any:
		for _, key := range contractSortedKeys(item) {
			child := item[key]
			var next string
			if at == "#" {
				next = contractPointer(key)
			} else {
				next = at + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
			}
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok || !validContractPointer(ref) {
					return contractErrAt("schema_invalid", "json")
				}
				(*count)++
				if *count > 256 {
					return contractErrAt("preflight_limit", "budget")
				}
				refs[at] = append(refs[at], ref)
			}
			if err := collectContractRefs(child, next, refs, count); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range item {
			if err := collectContractRefs(child, fmt.Sprintf("%s/%d", at, index), refs, count); err != nil {
				return err
			}
		}
	}
	return nil
}

func validContractPointer(pointer string) bool {
	if pointer == "#" {
		return true
	}
	if !strings.HasPrefix(pointer, "#/") || len(pointer) > 2048 {
		return false
	}
	for _, part := range strings.Split(pointer[2:], "/") {
		if part == "." || part == ".." {
			return false
		}
		for i := 0; i < len(part); i++ {
			if part[i] == '~' && (i+1 == len(part) || (part[i+1] != '0' && part[i+1] != '1')) {
				return false
			}
			if part[i] == '~' {
				i++
			}
		}
	}
	return true
}

func resolveContractPointer(root any, pointer string) (any, bool) {
	if pointer == "#" {
		return root, true
	}
	value := root
	for _, encoded := range strings.Split(pointer[2:], "/") {
		part := strings.NewReplacer("~1", "/", "~0", "~").Replace(encoded)
		switch container := value.(type) {
		case map[string]any:
			var found bool
			value, found = container[part]
			if !found {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(container) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = container[index]
		default:
			return nil, false
		}
	}
	return value, true
}

// contractRun admits an input once, asks a family parser for a bounded plan,
// validates the plan, and emits an all-or-nothing canonical v1 trace ledger.
func contractRun(input ContractInput, family string, limits budget, parse func([]byte) (contractPlan, error)) ([]byte, error) {
	if parse == nil || !contractName(family) {
		return nil, contractErrAt("schema_invalid", "schema")
	}
	snapshot, release, err := begin(input.Artifact, input.ContentSHA256, func() bool { return true }, []string{family})
	if err != nil {
		return nil, contractAdmissionErr(err)
	}
	defer release()
	plan, err := parse(snapshot)
	if err != nil {
		return nil, contractRecode(err)
	}
	profile, err := contractProfile(snapshot, family)
	if err != nil {
		return nil, err
	}
	if err := validateContractPlan(plan, family); err != nil {
		return nil, err
	}
	if family != "protobuf" {
		value, err := decodeJSON(snapshot)
		if err != nil {
			return nil, contractRecode(err)
		}
		if err := validateContractURLs(value, "#", plan.gaps, plan.facts); err != nil {
			return nil, err
		}
	}
	return emitContractPlan(input, family, profile, plan, limits)
}

type contractURLScope struct {
	gapOnly map[string]bool
}

func newContractURLScope(gaps []contractGap, facts []contractFact) contractURLScope {
	factDescendants := make(map[string]bool, len(facts))
	for _, fact := range facts {
		for _, ancestor := range contractURLAncestors(fact.key) {
			factDescendants[ancestor] = true
		}
	}
	scope := contractURLScope{gapOnly: make(map[string]bool, len(gaps))}
	for _, gap := range gaps {
		if gap.key == "#" || gap.kind == "buf.module_unresolved" {
			continue
		}
		scope.gapOnly[gap.key] = !factDescendants[gap.key]
	}
	return scope
}

func contractURLAncestors(key string) []string {
	ancestors := []string{key}
	for key != "#" {
		index := strings.LastIndexByte(key, '/')
		if index < 0 {
			break
		}
		if index == 0 {
			key = "#"
		} else {
			key = key[:index]
		}
		ancestors = append(ancestors, key)
	}
	return ancestors
}

// URL-valued declarations are admitted only beneath explicitly recorded,
// gap-only constructs. The declarations-only root gap grants no exception.
func validateContractURLs(value any, at string, gaps []contractGap, facts []contractFact) error {
	scope := newContractURLScope(gaps, facts)
	return validateContractURLsWithScope(value, at, scope)
}

func validateContractURLsWithScope(value any, at string, scope contractURLScope) error {
	check := func(text, pointer string) error {
		lower := strings.ToLower(text)
		if !strings.HasPrefix(lower, "http:") && !strings.HasPrefix(lower, "https:") {
			return nil
		}
		for _, ancestor := range contractURLAncestors(pointer) {
			if scope.gapOnly[ancestor] {
				return nil
			}
		}
		return contractErrAt("schema_invalid", "json")
	}
	switch item := value.(type) {
	case map[string]any:
		for _, key := range contractSortedKeys(item) {
			pointer := openAPIChildPointer(at, key)
			// A declaration key cannot grant itself an exception through the
			// gap recorded for its value; only an enclosing gap can do so.
			if err := check(key, at); err != nil {
				return err
			}
			if err := validateContractURLsWithScope(item[key], pointer, scope); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range item {
			if err := validateContractURLsWithScope(child, fmt.Sprintf("%s/%d", at, index), scope); err != nil {
				return err
			}
		}
	case string:
		return check(item, at)
	}
	return nil
}

func contractAdmissionErr(err error) error {
	rule, ok := err.(*RuleError)
	if !ok {
		return contractErrAt("schema_invalid", "schema")
	}
	switch rule.Code {
	case "stage3b.input_length":
		return contractErrAt("input_length", "input")
	case "stage3b.resource_admission_busy":
		return contractErrAt("resource_admission_busy", "input")
	case "stage3b.metadata_invalid":
		return contractErrAt("metadata_invalid", "metadata")
	case "stage3b.secret_indicator":
		return contractErrAt("secret_indicator", "metadata")
	case "stage3b.digest_mismatch":
		return contractErrAt("digest_mismatch", "digest")
	default:
		return contractErrAt("schema_invalid", "schema")
	}
}

func contractProfile(data []byte, family string) (string, error) {
	if family == "protobuf" {
		return "proto3", nil
	}
	value, err := decodeJSON(data)
	if err != nil {
		return "", contractRecode(err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return "", contractErrAt("schema_invalid", "schema")
	}
	switch family {
	case "openapi":
		profile, _ := root["openapi"].(string)
		if profile == "3.0.3" || profile == "3.1.0" {
			return profile, nil
		}
	case "asyncapi":
		profile, _ := root["asyncapi"].(string)
		if profile == "2.6.0" || profile == "3.0.0" {
			return profile, nil
		}
	case "buf":
		if profile, _ := root["version"].(string); profile == "v2" {
			return profile, nil
		}
	case "pact":
		if metadata, ok := root["metadata"].(map[string]any); ok {
			if specification, ok := metadata["pactSpecification"].(map[string]any); ok {
				if profile, _ := specification["version"].(string); profile == "4.0" {
					return profile, nil
				}
			}
		}
	}
	return "", contractErrAt("schema_invalid", "schema")
}

func validateContractPlan(plan contractPlan, family string) error {
	if len(plan.facts) > 1024 || len(plan.gaps) > 1024 {
		return contractErrAt("preflight_limit", "budget")
	}
	keys := make(map[string]struct{}, len(plan.facts))
	for _, fact := range plan.facts {
		if !contractPlanKey(fact.key) || len(fact.label) == 0 || len(fact.label) > 256 || !contractFactKind(fact.kind) {
			return contractErrAt("schema_invalid", "schema")
		}
		if _, found := keys[fact.key]; found {
			return contractErrAt("schema_invalid", "schema")
		}
		keys[fact.key] = struct{}{}
	}
	for _, fact := range plan.facts {
		if fact.parent != "" {
			if !contractPlanKey(fact.parent) || fact.parent == fact.key {
				return contractErrAt("schema_invalid", "schema")
			}
			if _, found := keys[fact.parent]; !found {
				return contractErrAt("schema_invalid", "schema")
			}
		}
	}
	gapKeys := make(map[string]struct{}, len(plan.gaps))
	declarationsOnly := 0
	for _, gap := range plan.gaps {
		if !contractGapKey(gap.key) || !contractName(gap.kind) || len(gap.reason) == 0 || len(gap.reason) > 256 {
			return contractErrAt("schema_invalid", "schema")
		}
		if _, found := gapKeys[gap.key]; found {
			return contractErrAt("schema_invalid", "schema")
		}
		gapKeys[gap.key] = struct{}{}
		if gap.kind == family+".declarations_only" {
			if gap.key != "#" {
				return contractErrAt("schema_invalid", "schema")
			}
			declarationsOnly++
		}
	}
	if declarationsOnly != 1 {
		return contractErrAt("schema_invalid", "schema")
	}
	return nil
}

func contractGapKey(key string) bool {
	return contractPlanKey(key)
}

func contractPlanKey(key string) bool {
	return contractName(key) || validContractPointer(key)
}

func contractFactKind(kind trace.NodeKind) bool {
	return kind == trace.Endpoint || kind == trace.ProtocolMessage || kind == trace.BinaryField
}

func emitContractPlan(input ContractInput, family, profile string, plan contractPlan, limits budget) ([]byte, error) {
	edges := 0
	for _, fact := range plan.facts {
		if fact.parent != "" {
			edges++
		}
	}
	records := uint64(len(plan.facts) + edges + len(plan.facts) + edges + len(plan.gaps))
	// Bound every encoded record independently before graph allocation, including
	// the six-byte JSON escapes used for control characters in pointer anchors.
	output := uint64(1024) + records*contractMaxEncodedRecord
	if err := checkBudget(records, output, limits); err != nil {
		return nil, contractErrAt("preflight_limit", "budget")
	}
	producer := trace.Producer{Name: "mcd-" + family + "-adapter", Version: contractAdapterVersion}
	base := "contract:" + url.PathEscape(input.Artifact.Path)
	ids := make(map[string]string, len(plan.facts))
	document := newDocument(trace.Schema)
	for _, fact := range plan.facts {
		identity := tupleHash("stage4", family, profile, "node", string(fact.kind), input.Artifact.Path, input.ContentSHA256, fact.key)
		id := "contract-node-" + identity
		ids[fact.key] = id
		anchor := base + fact.key
		provenance := trace.Provenance{ContentSHA256: input.ContentSHA256, Anchor: anchor, Producer: producer}
		evidenceID := "contract-evidence-" + tupleHash("stage4", family, profile, "evidence", "contract", id, input.Artifact.Path, input.ContentSHA256, fact.key)
		document.Nodes = append(document.Nodes, trace.Node{ID: id, Kind: fact.kind, Label: fact.label, Provenance: provenance, Evidence: []string{evidenceID}})
		document.Evidence = append(document.Evidence, trace.Evidence{ID: evidenceID, Subject: "node:" + id, Level: trace.Contract, Provenance: provenance})
	}
	for _, fact := range plan.facts {
		if fact.parent == "" {
			continue
		}
		identity := tupleHash("stage4", family, profile, "edge", string(trace.DerivedFrom), ids[fact.key], ids[fact.parent], input.Artifact.Path, input.ContentSHA256, fact.key)
		id := "contract-edge-" + identity
		anchor := base + fact.key
		provenance := trace.Provenance{ContentSHA256: input.ContentSHA256, Anchor: anchor, Producer: producer}
		evidenceID := "contract-evidence-" + tupleHash("stage4", family, profile, "evidence", "contract", id, input.Artifact.Path, input.ContentSHA256, fact.key)
		document.Edges = append(document.Edges, trace.Edge{ID: id, Kind: trace.DerivedFrom, Source: ids[fact.key], Target: ids[fact.parent], Provenance: provenance, Evidence: []string{evidenceID}})
		document.Evidence = append(document.Evidence, trace.Evidence{ID: evidenceID, Subject: "edge:" + id, Level: trace.Contract, Provenance: provenance})
	}
	for _, gap := range plan.gaps {
		reference := "contract:" + family
		identity := tupleHash("stage4", family, profile, "gap", gap.kind, reference, gap.reason, input.Artifact.Path, input.ContentSHA256, gap.key)
		anchor := base + gap.key
		provenance := trace.Provenance{ContentSHA256: input.ContentSHA256, Anchor: anchor, Producer: producer}
		document.Gaps = append(document.Gaps, trace.Gap{ID: "contract-gap-" + identity, Kind: gap.kind, Reference: reference, Reason: gap.reason, Provenance: provenance})
	}
	// finish validates and canonicalizes; no partial document escapes on failure.
	outputBytes, err := finish(document)
	if err != nil {
		return nil, contractErrAt("ledger_invalid", "schema")
	}
	return outputBytes, nil
}
