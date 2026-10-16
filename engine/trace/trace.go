// Package trace defines the versioned, fail-closed trace-ledger contract used
// to connect requirements, implementation artefacts, models, and review
// decisions. It deliberately records evidence without treating any evidence
// label as an implementation-correctness verdict.
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const Schema = "mcd-trace-ledger/1"

const maxJSONNestingDepth = 64

var (
	idRE     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]*$`)
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// NodeKind identifies the kind of an artefact recorded by a trace ledger.
type NodeKind string

const (
	Issue           NodeKind = "Issue"
	Requirement     NodeKind = "Requirement"
	Assumption      NodeKind = "Assumption"
	StandardClause  NodeKind = "StandardClause"
	ProtocolMessage NodeKind = "ProtocolMessage"
	BinaryField     NodeKind = "BinaryField"
	SourceSymbol    NodeKind = "SourceSymbol"
	Endpoint        NodeKind = "Endpoint"
	ModelTransition NodeKind = "ModelTransition"
	Property        NodeKind = "Property"
	Scenario        NodeKind = "Scenario"
	Test            NodeKind = "Test"
	RuntimeSpan     NodeKind = "RuntimeSpan"
	Counterexample  NodeKind = "Counterexample"
	ReviewDecision  NodeKind = "ReviewDecision"
)

// EdgeKind identifies a closed set of relations between ledger nodes.
type EdgeKind string

const (
	Implements  EdgeKind = "implements"
	MapsTo      EdgeKind = "maps_to"
	Calls       EdgeKind = "calls"
	Sends       EdgeKind = "sends"
	Receives    EdgeKind = "receives"
	DecodesAs   EdgeKind = "decodes_as"
	Tests       EdgeKind = "tests"
	Verifies    EdgeKind = "verifies"
	Assumes     EdgeKind = "assumes"
	Omits       EdgeKind = "omits"
	Contradicts EdgeKind = "contradicts"
	DerivedFrom EdgeKind = "derived_from"
)

// EvidenceLevel records how an artefact was obtained. Levels are not ordered:
// in particular, a model verdict or a review decision does not automatically
// establish a claim about source-code correctness.
type EvidenceLevel string

const (
	Syntactic               EvidenceLevel = "syntactic"
	Decoded                 EvidenceLevel = "decoded"
	Grammar                 EvidenceLevel = "grammar"
	Inferred                EvidenceLevel = "inferred"
	Contract                EvidenceLevel = "contract"
	NormativeCandidate      EvidenceLevel = "normative_candidate"
	LicensedClauseReference EvidenceLevel = "licensed_clause_reference"
	Runtime                 EvidenceLevel = "runtime"
	Modelled                EvidenceLevel = "modelled"
	Verified                EvidenceLevel = "verified"
	Reviewed                EvidenceLevel = "reviewed"
)

// Producer identifies the tool or reviewer that produced an artefact.
type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Provenance pins an artefact to its source and producer. At least one of
// SourceRevision and ContentSHA256 is required. Anchor is the stable source
// location used to review the claim.
type Provenance struct {
	SourceRevision string   `json:"source_revision,omitempty"`
	ContentSHA256  string   `json:"content_sha256,omitempty"`
	Anchor         string   `json:"anchor"`
	Producer       Producer `json:"producer"`
}

// Node is a typed artefact in a trace graph. Evidence contains ledger-record
// identifiers whose Subject is this node.
type Node struct {
	ID         string     `json:"id"`
	Kind       NodeKind   `json:"kind"`
	Label      string     `json:"label"`
	Provenance Provenance `json:"provenance"`
	Evidence   []string   `json:"evidence"`
}

// Edge is a typed relation between two exact node IDs. Its endpoints are never
// resolved by label similarity.
type Edge struct {
	ID         string     `json:"id"`
	Kind       EdgeKind   `json:"kind"`
	Source     string     `json:"source"`
	Target     string     `json:"target"`
	Provenance Provenance `json:"provenance"`
	Evidence   []string   `json:"evidence"`
}

// Evidence is an immutable ledger record for one node or edge. Subject has the
// exact form "node:<id>" or "edge:<id>".
type Evidence struct {
	ID         string        `json:"id"`
	Subject    string        `json:"subject"`
	Level      EvidenceLevel `json:"level"`
	Provenance Provenance    `json:"provenance"`
}

// Gap records an unresolved obligation explicitly. It is not a substitute for
// a dangling node or edge reference, which validation always rejects.
type Gap struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Reference  string     `json:"reference"`
	Reason     string     `json:"reason"`
	Provenance Provenance `json:"provenance"`
}

// Document is a complete trace graph and its append-only evidence ledger.
type Document struct {
	Schema   string     `json:"schema"`
	Nodes    []Node     `json:"nodes"`
	Edges    []Edge     `json:"edges"`
	Evidence []Evidence `json:"evidence"`
	Gaps     []Gap      `json:"gaps"`
}

// UnmarshalJSON parses and validates a trace ledger. The parser rejects
// duplicate keys, unknown fields, and trailing values at every object level.
func UnmarshalJSON(data []byte) (*Document, error) {
	if err := rejectDuplicateKeys(data); err != nil {
		return nil, fmt.Errorf("trace: %w", err)
	}
	if err := validateExactJSONKeys(data); err != nil {
		return nil, fmt.Errorf("trace: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("trace: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trace: trailing data after the document")
	}
	if err := Validate(&d); err != nil {
		return nil, fmt.Errorf("trace: %w", err)
	}
	return &d, nil
}

// Validate enforces the closed vocabulary, exact references, provenance, and
// evidence ownership rules of the trace-ledger contract.
func Validate(d *Document) error {
	if d == nil {
		return fmt.Errorf("document is nil")
	}
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, want %q", d.Schema, Schema)
	}
	if d.Nodes == nil || d.Edges == nil || d.Evidence == nil || d.Gaps == nil {
		return fmt.Errorf("nodes, edges, evidence, and gaps must be arrays")
	}

	nodes := make(map[string]Node, len(d.Nodes))
	for i, n := range d.Nodes {
		path := fmt.Sprintf("nodes[%d]", i)
		if err := validID(n.ID, path+".id"); err != nil {
			return err
		}
		if !validNodeKind(n.Kind) {
			return fmt.Errorf("%s.kind %q is outside the vocabulary", path, n.Kind)
		}
		if strings.TrimSpace(n.Label) == "" {
			return fmt.Errorf("%s.label is blank", path)
		}
		if _, exists := nodes[n.ID]; exists {
			return fmt.Errorf("%s.id %q duplicates a node", path, n.ID)
		}
		if err := validateProvenance(n.Provenance, path+".provenance"); err != nil {
			return err
		}
		if err := validateEvidenceIDs(n.Evidence, path+".evidence"); err != nil {
			return err
		}
		nodes[n.ID] = n
	}

	edges := make(map[string]Edge, len(d.Edges))
	for i, e := range d.Edges {
		path := fmt.Sprintf("edges[%d]", i)
		if err := validID(e.ID, path+".id"); err != nil {
			return err
		}
		if !validEdgeKind(e.Kind) {
			return fmt.Errorf("%s.kind %q is outside the vocabulary", path, e.Kind)
		}
		if _, ok := nodes[e.Source]; !ok {
			return fmt.Errorf("%s.source %q does not name a node", path, e.Source)
		}
		if _, ok := nodes[e.Target]; !ok {
			return fmt.Errorf("%s.target %q does not name a node", path, e.Target)
		}
		if _, exists := edges[e.ID]; exists {
			return fmt.Errorf("%s.id %q duplicates an edge", path, e.ID)
		}
		if err := validateProvenance(e.Provenance, path+".provenance"); err != nil {
			return err
		}
		if err := validateEvidenceIDs(e.Evidence, path+".evidence"); err != nil {
			return err
		}
		edges[e.ID] = e
	}

	evidence := make(map[string]Evidence, len(d.Evidence))
	for i, ev := range d.Evidence {
		path := fmt.Sprintf("evidence[%d]", i)
		if err := validID(ev.ID, path+".id"); err != nil {
			return err
		}
		if _, exists := evidence[ev.ID]; exists {
			return fmt.Errorf("%s.id %q duplicates evidence", path, ev.ID)
		}
		if !validEvidenceLevel(ev.Level) {
			return fmt.Errorf("%s.level %q is outside the vocabulary", path, ev.Level)
		}
		if err := validateSubject(ev.Subject, nodes, edges, path+".subject"); err != nil {
			return err
		}
		if err := validateProvenance(ev.Provenance, path+".provenance"); err != nil {
			return err
		}
		evidence[ev.ID] = ev
	}

	referencedEvidence := make(map[string]bool, len(d.Evidence))
	for _, n := range d.Nodes {
		id := n.ID
		for _, evidenceID := range n.Evidence {
			ev, ok := evidence[evidenceID]
			if !ok {
				return fmt.Errorf("node %q references unknown evidence %q", id, evidenceID)
			}
			if ev.Subject != "node:"+id {
				return fmt.Errorf("node %q references evidence %q for %q", id, evidenceID, ev.Subject)
			}
			referencedEvidence[evidenceID] = true
		}
	}
	for _, e := range d.Edges {
		id := e.ID
		for _, evidenceID := range e.Evidence {
			ev, ok := evidence[evidenceID]
			if !ok {
				return fmt.Errorf("edge %q references unknown evidence %q", id, evidenceID)
			}
			if ev.Subject != "edge:"+id {
				return fmt.Errorf("edge %q references evidence %q for %q", id, evidenceID, ev.Subject)
			}
			referencedEvidence[evidenceID] = true
		}
	}
	for i, ev := range d.Evidence {
		if !referencedEvidence[ev.ID] {
			return fmt.Errorf("evidence[%d] %q is not listed by its subject", i, ev.ID)
		}
		if err := validateEvidenceApplicability(ev, nodes, edges, fmt.Sprintf("evidence[%d]", i)); err != nil {
			return err
		}
	}

	gaps := make(map[string]bool, len(d.Gaps))
	for i, gap := range d.Gaps {
		path := fmt.Sprintf("gaps[%d]", i)
		if err := validID(gap.ID, path+".id"); err != nil {
			return err
		}
		if gaps[gap.ID] {
			return fmt.Errorf("%s.id %q duplicates a gap", path, gap.ID)
		}
		if !idRE.MatchString(gap.Kind) {
			return fmt.Errorf("%s.kind %q is not a stable identifier", path, gap.Kind)
		}
		if strings.TrimSpace(gap.Kind) == "" || strings.TrimSpace(gap.Reference) == "" || strings.TrimSpace(gap.Reason) == "" {
			return fmt.Errorf("%s needs non-blank kind, reference, and reason", path)
		}
		if err := validateProvenance(gap.Provenance, path+".provenance"); err != nil {
			return err
		}
		gaps[gap.ID] = true
	}
	return nil
}

func validID(id, path string) error {
	if !idRE.MatchString(id) {
		return fmt.Errorf("%s %q is not a stable identifier", path, id)
	}
	return nil
}

func uniqueIDs(ids []string, path string) error {
	if ids == nil {
		return fmt.Errorf("%s must be an array", path)
	}
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if err := validID(id, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("%s contains duplicate evidence %q", path, id)
		}
		seen[id] = true
	}
	return nil
}

func validateEvidenceIDs(ids []string, path string) error {
	if err := uniqueIDs(ids, path); err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("%s must not be empty", path)
	}
	return nil
}

func validateProvenance(p Provenance, path string) error {
	sourceRevision := strings.TrimSpace(p.SourceRevision)
	if p.SourceRevision != "" && sourceRevision == "" {
		return fmt.Errorf("%s.source_revision is blank", path)
	}
	if sourceRevision == "" && p.ContentSHA256 == "" {
		return fmt.Errorf("%s needs source_revision or content_sha256", path)
	}
	if p.ContentSHA256 != "" && !sha256RE.MatchString(p.ContentSHA256) {
		return fmt.Errorf("%s.content_sha256 is not a lowercase SHA-256", path)
	}
	if strings.TrimSpace(p.Anchor) == "" {
		return fmt.Errorf("%s.anchor is blank", path)
	}
	if strings.TrimSpace(p.Producer.Name) == "" || strings.TrimSpace(p.Producer.Version) == "" {
		return fmt.Errorf("%s.producer needs non-blank name and version", path)
	}
	return nil
}

func validateSubject(subject string, nodes map[string]Node, edges map[string]Edge, path string) error {
	kind, id, ok := strings.Cut(subject, ":")
	if !ok || id == "" {
		return fmt.Errorf("%s %q must be node:<id> or edge:<id>", path, subject)
	}
	switch kind {
	case "node":
		if _, ok := nodes[id]; !ok {
			return fmt.Errorf("%s %q names no node", path, subject)
		}
	case "edge":
		if _, ok := edges[id]; !ok {
			return fmt.Errorf("%s %q names no edge", path, subject)
		}
	default:
		return fmt.Errorf("%s %q must be node:<id> or edge:<id>", path, subject)
	}
	return nil
}

func validateEvidenceApplicability(ev Evidence, nodes map[string]Node, edges map[string]Edge, path string) error {
	if ev.Level != Modelled && ev.Level != Verified {
		return nil
	}
	kind, id, _ := strings.Cut(ev.Subject, ":")
	switch kind {
	case "node":
		if isModelArtifact(nodes[id].Kind) {
			return nil
		}
	case "edge":
		edge := edges[id]
		if isModelArtifact(nodes[edge.Source].Kind) && isModelArtifact(nodes[edge.Target].Kind) {
			return nil
		}
	}
	return fmt.Errorf("%s.level %q must apply only to a formal-model artifact", path, ev.Level)
}

func isModelArtifact(kind NodeKind) bool {
	switch kind {
	case ModelTransition, Property, Counterexample:
		return true
	}
	return false
}

func validNodeKind(kind NodeKind) bool {
	switch kind {
	case Issue, Requirement, Assumption, StandardClause, ProtocolMessage, BinaryField, SourceSymbol, Endpoint, ModelTransition, Property, Scenario, Test, RuntimeSpan, Counterexample, ReviewDecision:
		return true
	}
	return false
}

func validEdgeKind(kind EdgeKind) bool {
	switch kind {
	case Implements, MapsTo, Calls, Sends, Receives, DecodesAs, Tests, Verifies, Assumes, Omits, Contradicts, DerivedFrom:
		return true
	}
	return false
}

func validEvidenceLevel(level EvidenceLevel) bool {
	switch level {
	case Syntactic, Decoded, Grammar, Inferred, Contract, NormativeCandidate, LicensedClauseReference, Runtime, Modelled, Verified, Reviewed:
		return true
	}
	return false
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := consumeJSONValue(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing data after the document")
	}
	return nil
}

func consumeJSONValue(dec *json.Decoder, depth int) error {
	if depth > maxJSONNestingDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", maxJSONNestingDepth)
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = true
			if err := consumeJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := consumeJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func validateExactJSONKeys(data []byte) error {
	var root json.RawMessage = data
	return validateJSONDocument(root)
}

func validateJSONDocument(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "document", "schema", "nodes", "edges", "evidence", "gaps")
	if err != nil {
		return err
	}
	if err := validateJSONArray(object["nodes"], "nodes", validateJSONNode); err != nil {
		return err
	}
	if err := validateJSONArray(object["edges"], "edges", validateJSONEdge); err != nil {
		return err
	}
	if err := validateJSONArray(object["evidence"], "evidence", validateJSONEvidence); err != nil {
		return err
	}
	return validateJSONArray(object["gaps"], "gaps", validateJSONGap)
}

func validateJSONNode(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "node", "id", "kind", "label", "provenance", "evidence")
	if err != nil {
		return err
	}
	return validateJSONProvenance(object["provenance"])
}

func validateJSONEdge(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "edge", "id", "kind", "source", "target", "provenance", "evidence")
	if err != nil {
		return err
	}
	return validateJSONProvenance(object["provenance"])
}

func validateJSONEvidence(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "evidence", "id", "subject", "level", "provenance")
	if err != nil {
		return err
	}
	return validateJSONProvenance(object["provenance"])
}

func validateJSONGap(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "gap", "id", "kind", "reference", "reason", "provenance")
	if err != nil {
		return err
	}
	return validateJSONProvenance(object["provenance"])
}

func validateJSONProvenance(raw json.RawMessage) error {
	object, err := validateJSONObject(raw, "provenance", "source_revision", "content_sha256", "anchor", "producer")
	if err != nil {
		return err
	}
	if rawSourceRevision, ok := object["source_revision"]; ok {
		if err := validateNonBlankJSONString(rawSourceRevision, "provenance.source_revision"); err != nil {
			return err
		}
	}
	if rawContentSHA256, ok := object["content_sha256"]; ok {
		if err := validateSHA256JSONString(rawContentSHA256, "provenance.content_sha256"); err != nil {
			return err
		}
	}
	_, err = validateJSONObject(object["producer"], "producer", "name", "version")
	return err
}

func validateNonBlankJSONString(raw json.RawMessage, path string) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", path, err)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is blank", path)
	}
	return nil
}

func validateSHA256JSONString(raw json.RawMessage, path string) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", path, err)
	}
	if !sha256RE.MatchString(value) {
		return fmt.Errorf("%s must be a lowercase SHA-256", path)
	}
	return nil
}

func validateJSONObject(raw json.RawMessage, path string, allowed ...string) (map[string]json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%s must be an object: %w", path, err)
	}
	if object == nil {
		return nil, nil
	}
	allowedKeys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = true
	}
	for key := range object {
		if !allowedKeys[key] {
			return nil, fmt.Errorf("unknown field %q in %s", key, path)
		}
	}
	return object, nil
}

func validateJSONArray(raw json.RawMessage, path string, validate func(json.RawMessage) error) error {
	if raw == nil {
		return nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("%s must be an array: %w", path, err)
	}
	for _, value := range values {
		if err := validate(value); err != nil {
			return err
		}
	}
	return nil
}

// Canonical returns a validation-preserving copy whose lists are sorted by
// stable ID. It does not modify the caller's document.
func Canonical(d *Document) (*Document, error) {
	if err := Validate(d); err != nil {
		return nil, err
	}
	out := *d
	out.Nodes = append([]Node{}, d.Nodes...)
	out.Edges = append([]Edge{}, d.Edges...)
	out.Evidence = append([]Evidence{}, d.Evidence...)
	out.Gaps = append([]Gap{}, d.Gaps...)
	for i := range out.Nodes {
		out.Nodes[i].Evidence = append([]string{}, out.Nodes[i].Evidence...)
		sort.Strings(out.Nodes[i].Evidence)
	}
	for i := range out.Edges {
		out.Edges[i].Evidence = append([]string{}, out.Edges[i].Evidence...)
		sort.Strings(out.Edges[i].Evidence)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].ID < out.Edges[j].ID })
	sort.Slice(out.Evidence, func(i, j int) bool { return out.Evidence[i].ID < out.Evidence[j].ID })
	sort.Slice(out.Gaps, func(i, j int) bool { return out.Gaps[i].ID < out.Gaps[j].ID })
	return &out, nil
}

// JSON returns the canonical document encoded with fixed indentation and a
// trailing newline.
func JSON(d *Document) ([]byte, error) {
	canonical, err := Canonical(d)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(canonical); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
