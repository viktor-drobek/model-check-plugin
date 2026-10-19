package ir

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OptimizedSchema identifies an optimization overlay whose catalog remains the
// source of truth for canonical mcd-ir/1 structure.
const OptimizedSchema = "mcd-optimized-ir/1"

// StaticCatalogSchema identifies an immutable catalog of canonical mcd-ir/1
// structure. The catalog remains authoritative even when a later overlay
// describes a reduced or optimized representation.
const StaticCatalogSchema = "mcd-static-catalog/1"

// CatalogValidationSchema identifies a constructive structural validation
// result. It does not make a claim about executable or behavioral equivalence.
const CatalogValidationSchema = "mcd-catalog-validation/1"

// CatalogIsomorphismRelation identifies exact equality of canonical source and
// reconstructed catalog IR under the mcd-ir/1 structural contract.
const CatalogIsomorphismRelation = "catalog-isomorphism"

// CatalogRef binds an optimization overlay to the canonical catalog it reads.
type CatalogRef struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

// StaticCatalog stores the complete canonical IR, including unreachable
// elements, with a digest that binds overlays to those exact bytes.
type StaticCatalog struct {
	Schema     string          `json:"schema"`
	CatalogRef CatalogRef      `json:"catalog_ref"`
	IR         json.RawMessage `json:"ir"`
}

// CatalogValidation records a successful constructive structural validation.
// Its catalog reference identifies the complete, canonical catalog validated
// against the supplied source IR.
type CatalogValidation struct {
	Schema       string     `json:"schema"`
	RelationKind string     `json:"relation_kind"`
	CatalogRef   CatalogRef `json:"catalog_ref"`
}

type staticCatalogJSON StaticCatalog

// NewStaticCatalog validates m and records its deterministic canonical JSON.
func NewStaticCatalog(m *Model) (*StaticCatalog, error) {
	if err := Validate(m); err != nil {
		return nil, fmt.Errorf("static catalog: IR: %w", err)
	}
	canonical, err := MarshalJSON(m)
	if err != nil {
		return nil, fmt.Errorf("static catalog: marshal IR: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return &StaticCatalog{
		Schema:     StaticCatalogSchema,
		CatalogRef: CatalogRef{Schema: Schema, SHA256: hex.EncodeToString(digest[:])},
		IR:         append(json.RawMessage(nil), canonical...),
	}, nil
}

// MarshalJSON renders c deterministically after verifying its complete IR and
// catalog reference. It never emits caller-owned raw bytes unchecked.
func (c *StaticCatalog) MarshalJSON() ([]byte, error) {
	verified, _, err := c.validated()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(staticCatalogJSON(*verified)); err != nil {
		return nil, fmt.Errorf("static catalog: marshal: %w", err)
	}
	return buf.Bytes(), nil
}

// ReadStaticCatalog rejects malformed, non-canonical, or unbound catalog
// data before returning a defensive copy of its canonical IR.
func ReadStaticCatalog(data []byte) (*StaticCatalog, error) {
	if err := rejectDuplicateJSONMembers(data); err != nil {
		return nil, fmt.Errorf("static catalog: %w", err)
	}
	var catalog StaticCatalog
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("static catalog: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("static catalog: trailing data")
		}
		return nil, fmt.Errorf("static catalog: trailing data: %w", err)
	}
	if catalog.Schema != StaticCatalogSchema {
		return nil, fmt.Errorf("static catalog: schema %q, want %q", catalog.Schema, StaticCatalogSchema)
	}
	if catalog.CatalogRef.Schema != Schema {
		return nil, fmt.Errorf("static catalog: IR schema %q, want %q", catalog.CatalogRef.Schema, Schema)
	}
	model, err := UnmarshalJSON(catalog.IR)
	if err != nil {
		return nil, fmt.Errorf("static catalog: IR: %w", err)
	}
	canonical, err := MarshalJSON(model)
	if err != nil {
		return nil, fmt.Errorf("static catalog: canonicalize IR: %w", err)
	}
	digest := sha256.Sum256(canonical)
	if !strings.EqualFold(catalog.CatalogRef.SHA256, hex.EncodeToString(digest[:])) {
		return nil, fmt.Errorf("static catalog: IR digest mismatch")
	}
	catalog.CatalogRef.SHA256 = strings.ToLower(catalog.CatalogRef.SHA256)
	catalog.IR = append(json.RawMessage(nil), canonical...)
	return &catalog, nil
}

// Reconstruct returns a newly decoded, validated model from c's canonical IR.
func (c *StaticCatalog) Reconstruct() (*Model, error) {
	_, model, err := c.validated()
	return model, err
}

// ValidateCatalogIsomorphism confirms that catalog reconstructs to the same
// complete canonical mcd-ir/1 structure as source. This is a structural
// relation only; it does not validate LTS execution or behavioral equivalence.
func ValidateCatalogIsomorphism(source *Model, catalog *StaticCatalog) (*CatalogValidation, error) {
	if source == nil {
		return nil, fmt.Errorf("catalog structural mismatch: nil source IR")
	}
	sourceJSON, err := MarshalJSON(source)
	if err != nil {
		return nil, fmt.Errorf("catalog structural mismatch: canonicalize source IR: %w", err)
	}
	verified, recovered, err := catalog.validated()
	if err != nil {
		return nil, fmt.Errorf("catalog structural mismatch: catalog: %w", err)
	}
	recoveredJSON, err := MarshalJSON(recovered)
	if err != nil {
		return nil, fmt.Errorf("catalog structural mismatch: canonicalize reconstructed IR: %w", err)
	}
	if !bytes.Equal(sourceJSON, recoveredJSON) {
		return nil, fmt.Errorf("catalog structural mismatch: canonical source IR differs from reconstructed catalog IR")
	}
	return &CatalogValidation{
		Schema:       CatalogValidationSchema,
		RelationKind: CatalogIsomorphismRelation,
		CatalogRef:   verified.CatalogRef,
	}, nil
}

func (c *StaticCatalog) validated() (*StaticCatalog, *Model, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("static catalog: nil catalog")
	}
	encoded, err := c.marshalUnchecked()
	if err != nil {
		return nil, nil, err
	}
	verified, err := ReadStaticCatalog(encoded)
	if err != nil {
		return nil, nil, err
	}
	model, err := UnmarshalJSON(verified.IR)
	if err != nil {
		return nil, nil, err
	}
	return verified, model, nil
}

func (c *StaticCatalog) marshalUnchecked() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(staticCatalogJSON(*c)); err != nil {
		return nil, fmt.Errorf("static catalog: marshal: %w", err)
	}
	return buf.Bytes(), nil
}

func rejectDuplicateJSONMembers(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONValue(dec); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing data")
		}
		return err
	}
	return nil
}

func checkJSONValue(dec *json.Decoder) error {
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
		seen := make(map[string]struct{})
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("object member name is not a string")
			}
			folded := foldJSONName(name)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate member %q", name)
			}
			seen[folded] = struct{}{}
			if err := checkJSONValue(dec); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := checkJSONValue(dec); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	end, err := dec.Token()
	if err != nil {
		return err
	}
	if end != json.Delim('}') && end != json.Delim(']') {
		return fmt.Errorf("malformed JSON container")
	}
	return nil
}

func foldJSONName(name string) string {
	var folded strings.Builder
	folded.Grow(len(name))
	for _, r := range name {
		if r < utf8.RuneSelf {
			if 'a' <= r && r <= 'z' {
				r -= 'a' - 'A'
			}
			folded.WriteRune(r)
			continue
		}
		for {
			next := unicode.SimpleFold(r)
			if next <= r {
				r = next
				break
			}
			r = next
		}
		folded.WriteRune(r)
	}
	return folded.String()
}

// OptimizedIR is the portable, LLM-readable metadata for an optimized model.
// It deliberately contains no replacement structural catalog.
type OptimizedIR struct {
	Schema            string     `json:"schema"`
	CatalogRef        CatalogRef `json:"catalog_ref"`
	RelationKind      string     `json:"relation_kind"`
	PropertyScope     []string   `json:"property_scope,omitempty"`
	TransferDirection string     `json:"transfer_direction,omitempty"`
	EvidenceClass     string     `json:"evidence_class,omitempty"`
	Restrictions      []string   `json:"restrictions,omitempty"`
}

// LLMBrief is a deterministic semantic summary that an assistant may use to
// propose source-model changes. It never upgrades a weaker relation to exact.
type LLMBrief struct {
	RelationKind      string   `json:"relation_kind"`
	PropertyScope     []string `json:"property_scope,omitempty"`
	TransferDirection string   `json:"transfer_direction,omitempty"`
	EvidenceClass     string   `json:"evidence_class,omitempty"`
	Text              string   `json:"text"`
}

// ReadOptimizedIR validates data against catalog and returns an LLM-ready
// semantic brief. Callers must apply suggested changes to the canonical model
// and re-check them; this function does not prove an optimization correct.
func ReadOptimizedIR(data []byte, catalog *Model) (*LLMBrief, error) {
	if catalog == nil {
		return nil, fmt.Errorf("optimized ir: nil catalog")
	}
	static, err := NewStaticCatalog(catalog)
	if err != nil {
		return nil, fmt.Errorf("optimized ir: catalog: %w", err)
	}
	return ReadOptimizedIRCatalog(data, static)
}

// ReadOptimizedIRCatalog validates data against an immutable static catalog
// and returns an LLM-ready semantic brief.
func ReadOptimizedIRCatalog(data []byte, catalog *StaticCatalog) (*LLMBrief, error) {
	if catalog == nil {
		return nil, fmt.Errorf("optimized ir: nil catalog")
	}
	verified, model, err := catalog.validated()
	if err != nil {
		return nil, fmt.Errorf("optimized ir: catalog: %w", err)
	}
	var opt OptimizedIR
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&opt); err != nil {
		return nil, fmt.Errorf("optimized ir: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("optimized ir: trailing data after overlay")
		}
		return nil, fmt.Errorf("optimized ir: trailing data: %w", err)
	}
	if opt.Schema != OptimizedSchema {
		return nil, fmt.Errorf("optimized ir: schema %q, want %q", opt.Schema, OptimizedSchema)
	}
	if opt.CatalogRef.Schema != Schema {
		return nil, fmt.Errorf("optimized ir: catalog schema %q, want %q", opt.CatalogRef.Schema, Schema)
	}
	if opt.CatalogRef.Schema != verified.CatalogRef.Schema || !strings.EqualFold(opt.CatalogRef.SHA256, verified.CatalogRef.SHA256) {
		return nil, fmt.Errorf("optimized ir: catalog digest mismatch")
	}
	if !supportedRelation(opt.RelationKind) {
		return nil, fmt.Errorf("optimized ir: unsupported relation_kind %q", opt.RelationKind)
	}
	if err := validateRelationMetadata(&opt); err != nil {
		return nil, err
	}
	scope := append([]string(nil), opt.PropertyScope...)
	sort.Strings(scope)
	return &LLMBrief{RelationKind: opt.RelationKind, PropertyScope: scope, TransferDirection: opt.TransferDirection, EvidenceClass: opt.EvidenceClass, Text: briefText(model, opt.RelationKind, scope)}, nil
}

func validateRelationMetadata(opt *OptimizedIR) error {
	scoped := opt.RelationKind == "property-slice" || opt.RelationKind == "partial-order-reduction" || opt.RelationKind == "symmetry-reduction" || opt.RelationKind == "stuttering-bisimulation" || opt.RelationKind == "branching-bisimulation"
	if scoped && !nonBlank(opt.PropertyScope) {
		return fmt.Errorf("optimized ir: %s requires non-empty property_scope", opt.RelationKind)
	}
	if opt.RelationKind == "property-slice" || opt.RelationKind == "partial-order-reduction" || opt.RelationKind == "symmetry-reduction" {
		if strings.TrimSpace(opt.TransferDirection) == "" {
			return fmt.Errorf("optimized ir: %s requires transfer_direction", opt.RelationKind)
		}
	}
	if (opt.RelationKind == "stuttering-bisimulation" || opt.RelationKind == "branching-bisimulation") && !nonBlank(opt.Restrictions) {
		return fmt.Errorf("optimized ir: %s requires restrictions", opt.RelationKind)
	}
	if opt.RelationKind == "simulation" || opt.RelationKind == "underapproximation" || opt.RelationKind == "lossy" {
		if strings.TrimSpace(opt.TransferDirection) == "" {
			return fmt.Errorf("optimized ir: %s requires transfer_direction", opt.RelationKind)
		}
	}
	if opt.RelationKind == "underapproximation" || opt.RelationKind == "lossy" {
		if opt.EvidenceClass != "approximate" && opt.EvidenceClass != "bounded" && opt.EvidenceClass != "unknown" {
			return fmt.Errorf("optimized ir: %s requires approximate, bounded, or unknown evidence_class", opt.RelationKind)
		}
	}
	return nil
}

func nonBlank(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

func supportedRelation(kind string) bool {
	switch kind {
	case "state-isomorphism", "strong-bisimulation", "stuttering-bisimulation", "branching-bisimulation", "property-slice", "partial-order-reduction", "symmetry-reduction", "simulation", "underapproximation", "lossy":
		return true
	}
	return false
}

func briefText(m *Model, relation string, scope []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Canonical model %q uses %s.\n", m.Name, relation)
	fmt.Fprintln(&b, "Read the catalog as the source of truth; apply suggestions to the source model and re-check it.")
	if relation != "state-isomorphism" && relation != "strong-bisimulation" {
		fmt.Fprintln(&b, "This is a weaker relation: do not infer full-state equivalence outside the declared property scope.")
	}
	if len(scope) > 0 {
		fmt.Fprintf(&b, "Property scope: %s.\n", strings.Join(scope, ", "))
	}
	fmt.Fprintf(&b, "Globals: %s.\n", namesVars(m.Globals))
	for _, p := range m.Processes {
		fmt.Fprintf(&b, "Process %s has %d locations and %d edges.\n", p.Name, len(p.Locations), len(p.Edges))
		for i, e := range p.Edges {
			if e.Origin != nil && e.Origin.Name != "" {
				fmt.Fprintf(&b, "  Edge %s/%d: %s", p.Name, i, e.Origin.Name)
				if e.Origin.File != "" {
					fmt.Fprintf(&b, " (%s:%d)", e.Origin.File, e.Origin.Line)
				}
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func namesVars(vars []Var) string {
	if len(vars) == 0 {
		return "none"
	}
	names := make([]string, len(vars))
	for i, v := range vars {
		names[i] = v.Name
	}
	return strings.Join(names, ", ")
}
