// Package backend defines the fail-closed M3.0a in-process backend contract.
package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"unicode/utf8"

	"modelcheck/ir"
)

// ContractVersion identifies this closed backend-contract vocabulary.
const ContractVersion = "mcd-backend-contract/1"

// Request-input limits bound the v1 contract's fixed-size fields and IR preflight.
const maxBound uint64 = 2_147_483_647
const maxPropertyBytes = 1 << 20
const maxTextBytes = 256
const maxUTF8WalkValues = 4 * ir.MaxIRWeight
const maxUTF8WalkTextBytes = 16 << 20
const maxCanonicalModelBytes = 256 << 20

// maxCanonicalValueBytes covers shallow scalar JSON syntax.
const maxCanonicalValueBytes = 20

// Capability identifies a contract feature requested and acknowledged by a backend.
type Capability string

// Capability labels defined by ContractVersion.
const CapabilityExactVerdicts Capability = "exact-verdicts"

// Terminal is a closed terminal status for a backend-contract result.
type Terminal string

// Terminal statuses defined by ContractVersion.
const (
	StatusVerified         Terminal = "verified"
	StatusViolated         Terminal = "violated"
	StatusInconclusive     Terminal = "inconclusive"
	StatusResourceStop     Terminal = "resource-stop"
	StatusUnsupported      Terminal = "unsupported"
	StatusInvalidInput     Terminal = "invalid-input"
	StatusCancelled        Terminal = "cancelled"
	StatusIsolationRefused Terminal = "isolation-refused"
	StatusBackendFailure   Terminal = "backend-failure"
	StatusContractInvalid  Terminal = "contract-invalid"
)

// Evidence classifies the strength of a backend result without asserting a verdict.
type Evidence string

// Evidence classes defined by ContractVersion.
const (
	EvidenceExact                  Evidence = "exact"
	EvidenceApproximate            Evidence = "approximate"
	EvidenceCertifiedSound         Evidence = "certified-sound"
	EvidenceOracleExceptionReduced Evidence = "oracle-exception-reduced"
)

// TraceAssurance describes how the result relates to a trace artifact.
type TraceAssurance string

// Trace-assurance classes defined by ContractVersion.
const (
	TraceNotApplicable             TraceAssurance = "not-applicable"
	TraceReplayed                  TraceAssurance = "replayed"
	TraceAbsentApproved            TraceAssurance = "trace-absent-approved"
	TraceReplayUnavailableApproved TraceAssurance = "replay-unavailable-approved"
)

// Verdict is a closed property conclusion allowed by this contract version.
type Verdict string

// Property verdicts defined by ContractVersion.
const VerdictVerified Verdict = "verified"

// HostTerminal identifies a terminal outcome owned by the integration host.
type HostTerminal string

// Host terminal statuses defined by ContractVersion.
const (
	HostNone             HostTerminal = "none"
	HostInvalidInput     HostTerminal = "invalid-input"
	HostCancelled        HostTerminal = "cancelled"
	HostIsolationRefused HostTerminal = "isolation-refused"
	HostBackendFailure   HostTerminal = "backend-failure"
	HostResourceStop     HostTerminal = "resource-stop"
)

// ReasonCode describes a closed reason emitted by the contract kernel.
type ReasonCode string

// InvalidInputReason is a reason owned by the host for invalid input.
type InvalidInputReason ReasonCode

// BackendFailureReason is a reason owned by the host for a backend failure.
type BackendFailureReason ReasonCode

// ContractInvalidReason is a reason synthesized by the contract kernel.
type ContractInvalidReason ReasonCode

// UnsupportedReason is a reason synthesized for an unavailable capability.
type UnsupportedReason ReasonCode

const (
	InvalidInputReasonMalformedModel InvalidInputReason   = "malformed-model"
	BackendFailureReasonExecution    BackendFailureReason = "backend-execution-failed"

	ReasonInvalidRequest                   ContractInvalidReason = "invalid-request"
	ReasonInvalidHostFacts                 ContractInvalidReason = "invalid-host-facts"
	ReasonPayloadWithHostTerminal          ContractInvalidReason = "payload-with-host-terminal"
	ReasonPayloadWithUnavailableCapability ContractInvalidReason = "payload-with-unavailable-capability"
	ReasonIdentityMismatch                 ContractInvalidReason = "identity-mismatch"
	ReasonInvalidMetadata                  ContractInvalidReason = "invalid-metadata"
	ReasonInvalidTraceAssurance            ContractInvalidReason = "invalid-trace-assurance"
	ReasonInvalidResultRow                 ContractInvalidReason = "invalid-result-row"
	ReasonInvalidResourceStop              ContractInvalidReason = "invalid-resource-stop"
	ReasonUndeclaredResultRow              ContractInvalidReason = "undeclared-result-row"
	ReasonZeroValidated                    ContractInvalidReason = "zero-validated"
	ReasonExactVerdictsUnavailable         UnsupportedReason     = "exact-verdicts-unavailable"
)

// Resource identifies the bound that stopped or constrained a run.
type Resource string

const (
	ResourceStates       Resource = "states"
	ResourceDepth        Resource = "depth"
	ResourceMilliseconds Resource = "milliseconds"
	ResourceMemoryMB     Resource = "memory_mb"
)

// ResourcePhase identifies whether a resource stop occurred before or during execution.
type ResourcePhase string

const (
	PhaseAdmission ResourcePhase = "admission"
	PhaseExecution ResourcePhase = "execution"
)

// Bounds contains the four positive resource limits bound into a request.
type Bounds struct{ States, Depth, Milliseconds, MemoryMB uint64 }

// ResourceStop records the request-bound resource limit and observed consumption.
type ResourceStop struct {
	Resource Resource
	Phase    ResourcePhase
	Limit    uint64
	Consumed uint64
}

// RequestSpec supplies the immutable inputs for a backend-contract request.
type RequestSpec struct {
	ContractVersion, BackendID, BackendVersion string
	Model                                      *ir.Model
	Property                                   []byte
	Nonce                                      [16]byte
	Capabilities                               []Capability
	Bounds                                     Bounds
}

// Request is a validated immutable backend-contract request.
type Request struct {
	identity                  string
	backendID, backendVersion string
	capabilities              []Capability
	bounds                    Bounds
	valid                     bool
}

// HostFactsSpec supplies host-owned capabilities and a possible host terminal result.
type HostFactsSpec struct {
	AvailableCapabilities []Capability
	Terminal              HostTerminal
	InvalidInputReason    InvalidInputReason
	BackendFailureReason  BackendFailureReason
	ResourceStop          *ResourceStop
}

// HostFacts is a validated host-owned fact set bound to one Request.
type HostFacts struct {
	identity  string
	available []Capability
	terminal  HostTerminal
	reason    ReasonCode
	resource  *ResourceStop
	valid     bool
}

// Provenance identifies the implementation that produced a valid backend payload.
type Provenance struct{ BackendID, BackendVersion, Implementation, BuildIdentity string }

// Result is the untrusted backend payload consumed by Validate.
type Result struct {
	Status                              Terminal
	Identity, BackendID, BackendVersion string
	Acknowledged                        []Capability
	Provenance                          Provenance
	Evidence                            Evidence
	Trace                               TraceAssurance
	Verdict                             Verdict
	ResourceStop                        *ResourceStop
}

// Validated is an immutable result produced exclusively by Validate.
type Validated struct {
	result Result
	reason ReasonCode
	valid  bool
}

// NewRequest validates and binds an immutable backend-contract request.
func NewRequest(s RequestSpec) (Request, error) {
	if s.ContractVersion != ContractVersion || !validText(s.BackendID) || !validText(s.BackendVersion) {
		return Request{}, fmt.Errorf("backend: invalid contract or backend identity")
	}
	capabilities := append([]Capability(nil), s.Capabilities...)
	if s.Model == nil || !allUTF8(reflect.ValueOf(s.Model)) {
		return Request{}, fmt.Errorf("backend: invalid model text")
	}
	if err := ir.Validate(s.Model); err != nil {
		return Request{}, fmt.Errorf("backend: validate model: %w", err)
	}
	if len(s.Property) == 0 || len(s.Property) > maxPropertyBytes || bytes.Equal(s.Nonce[:], make([]byte, 16)) {
		return Request{}, fmt.Errorf("backend: invalid property or nonce")
	}
	if !sameCaps(capabilities, []Capability{CapabilityExactVerdicts}) || !validBounds(s.Bounds) {
		return Request{}, fmt.Errorf("backend: invalid capabilities or bounds")
	}
	canonical, err := ir.MarshalJSON(s.Model)
	if err != nil {
		return Request{}, fmt.Errorf("backend: marshal model: %w", err)
	}
	if _, err := ir.UnmarshalJSON(canonical); err != nil {
		return Request{}, fmt.Errorf("backend: canonical model: %w", err)
	}
	md, pd := sha256.Sum256(canonical), sha256.Sum256(s.Property)
	id := requestDigest(s.ContractVersion, ir.Schema, hex.EncodeToString(md[:]), hex.EncodeToString(pd[:]), s.BackendID, s.BackendVersion, capabilities, s.Bounds, s.Nonce)
	return Request{identity: id, backendID: s.BackendID, backendVersion: s.BackendVersion, capabilities: capabilities, bounds: s.Bounds, valid: true}, nil
}

// NewHostFacts validates host-owned facts and binds them to r.
func NewHostFacts(r Request, s HostFactsSpec) (HostFacts, error) {
	available := append([]Capability(nil), s.AvailableCapabilities...)
	if !r.valid || !strictCaps(available) || !validHost(s) {
		return HostFacts{}, fmt.Errorf("backend: invalid host facts")
	}
	h := HostFacts{identity: r.identity, available: available, terminal: s.Terminal, reason: hostReason(s), valid: true}
	if s.ResourceStop != nil {
		x := *s.ResourceStop
		h.resource = &x
	}
	if s.Terminal == HostResourceStop && !validHostStop(r.bounds, *h.resource) {
		return HostFacts{}, fmt.Errorf("backend: invalid host resource stop")
	}
	return h, nil
}

// Validate fail-closes an untrusted payload against request and host facts.
func Validate(r Request, h HostFacts, p Result) Validated {
	bad := func(reason ContractInvalidReason) Validated {
		return Validated{result: Result{Status: StatusContractInvalid, Identity: r.identity}, reason: ReasonCode(reason), valid: true}
	}
	if !r.valid {
		return bad(ReasonInvalidRequest)
	}
	p = cloneResult(p)
	if !h.valid || h.identity != r.identity {
		return bad(ReasonInvalidHostFacts)
	}
	if h.terminal != HostNone {
		if !zeroResult(p) {
			return bad(ReasonPayloadWithHostTerminal)
		}
		return hostResult(r, h)
	}
	if !contains(h.available, CapabilityExactVerdicts) {
		if !zeroResult(p) {
			return bad(ReasonPayloadWithUnavailableCapability)
		}
		return Validated{result: Result{Status: StatusUnsupported, Identity: r.identity}, reason: ReasonCode(ReasonExactVerdictsUnavailable), valid: true}
	}
	if p.Identity != r.identity || p.BackendID != r.backendID || p.BackendVersion != r.backendVersion {
		return bad(ReasonIdentityMismatch)
	}
	if !sameCaps(p.Acknowledged, r.capabilities) || !validProvenance(p.Provenance, r) {
		return bad(ReasonInvalidMetadata)
	}
	switch p.Status {
	case StatusVerified:
	case StatusInconclusive:
	case StatusResourceStop:
	default:
		return bad(ReasonUndeclaredResultRow)
	}
	if p.Trace != TraceNotApplicable {
		return bad(ReasonInvalidTraceAssurance)
	}
	switch p.Status {
	case StatusVerified:
		if p.Evidence != EvidenceExact || p.Verdict != VerdictVerified || p.ResourceStop != nil {
			return bad(ReasonInvalidResultRow)
		}
	case StatusInconclusive:
		if (p.Evidence != EvidenceExact && p.Evidence != EvidenceApproximate) || p.Verdict != "" || p.ResourceStop != nil {
			return bad(ReasonInvalidResultRow)
		}
	case StatusResourceStop:
		if p.Evidence != "" || p.Verdict != "" || p.ResourceStop == nil {
			return bad(ReasonInvalidResultRow)
		}
		if !validBackendStop(r.bounds, *p.ResourceStop) {
			return bad(ReasonInvalidResourceStop)
		}
	}
	return Validated{result: cloneResult(p), valid: true}
}

// Status returns the validated terminal status.
func (v Validated) Status() Terminal {
	if !v.valid {
		return StatusContractInvalid
	}
	return v.result.Status
}

// Reason returns the contract or host reason associated with the result.
func (v Validated) Reason() ReasonCode {
	if !v.valid {
		return ReasonCode(ReasonZeroValidated)
	}
	return v.reason
}

// Result returns a defensive copy of the validated result.
func (v Validated) Result() Result {
	if !v.valid {
		return Result{Status: StatusContractInvalid}
	}
	return cloneResult(v.result)
}

// Identity returns the immutable binding digest for the request.
func (r Request) Identity() string { return r.identity }

func hostResult(r Request, h HostFacts) Validated {
	out := Result{Identity: r.identity}
	switch h.terminal {
	case HostInvalidInput:
		out.Status = StatusInvalidInput
	case HostCancelled:
		out.Status = StatusCancelled
	case HostIsolationRefused:
		out.Status = StatusIsolationRefused
	case HostBackendFailure:
		out.Status = StatusBackendFailure
	case HostResourceStop:
		out.Status = StatusResourceStop
		x := *h.resource
		out.ResourceStop = &x
	}
	return Validated{result: out, reason: h.reason, valid: true}
}
func validBounds(b Bounds) bool {
	return b.States > 0 && b.Depth > 0 && b.Milliseconds > 0 && b.MemoryMB > 0 && b.States <= maxBound && b.Depth <= maxBound && b.Milliseconds <= maxBound && b.MemoryMB <= maxBound
}
func validHost(s HostFactsSpec) bool {
	switch s.Terminal {
	case HostNone, HostCancelled, HostIsolationRefused:
		return s.InvalidInputReason == "" && s.BackendFailureReason == "" && s.ResourceStop == nil
	case HostInvalidInput:
		return s.InvalidInputReason == InvalidInputReasonMalformedModel && s.BackendFailureReason == "" && s.ResourceStop == nil
	case HostBackendFailure:
		return s.InvalidInputReason == "" && s.BackendFailureReason == BackendFailureReasonExecution && s.ResourceStop == nil
	case HostResourceStop:
		return s.InvalidInputReason == "" && s.BackendFailureReason == "" && s.ResourceStop != nil
	default:
		return false
	}
}
func hostReason(s HostFactsSpec) ReasonCode {
	if s.Terminal == HostInvalidInput {
		return ReasonCode(s.InvalidInputReason)
	}
	if s.Terminal == HostBackendFailure {
		return ReasonCode(s.BackendFailureReason)
	}
	return ""
}
func validHostStop(b Bounds, s ResourceStop) bool {
	return validStop(b, s, true)
}
func validBackendStop(b Bounds, s ResourceStop) bool {
	return validStop(b, s, false)
}
func validStop(b Bounds, s ResourceStop, allowAdmission bool) bool {
	var limit uint64
	switch s.Resource {
	case ResourceStates:
		limit = b.States
	case ResourceDepth:
		limit = b.Depth
	case ResourceMilliseconds:
		limit = b.Milliseconds
	case ResourceMemoryMB:
		limit = b.MemoryMB
	default:
		return false
	}
	if s.Limit != limit || s.Consumed > maxBound {
		return false
	}
	if s.Phase == PhaseAdmission {
		return allowAdmission && s.Consumed <= s.Limit
	}
	return s.Phase == PhaseExecution && s.Consumed >= s.Limit
}
func validProvenance(p Provenance, r Request) bool {
	return p.BackendID == r.backendID && p.BackendVersion == r.backendVersion && validText(p.Implementation) && validText(p.BuildIdentity)
}
func validText(s string) bool { return s != "" && len(s) <= maxTextBytes && utf8.ValidString(s) }
func strictCaps(c []Capability) bool {
	for i, x := range c {
		if x != CapabilityExactVerdicts || i > 0 && c[i-1] >= x {
			return false
		}
	}
	return true
}
func sameCaps(a, b []Capability) bool {
	return len(a) == len(b) && sort.SliceIsSorted(a, func(i, j int) bool { return a[i] < a[j] }) && sort.SliceIsSorted(b, func(i, j int) bool { return b[i] < b[j] }) && reflect.DeepEqual(a, b)
}
func contains(c []Capability, x Capability) bool {
	for _, v := range c {
		if v == x {
			return true
		}
	}
	return false
}
func zeroResult(p Result) bool {
	return p.Status == "" && p.Identity == "" && p.BackendID == "" && p.BackendVersion == "" && len(p.Acknowledged) == 0 && p.Provenance == (Provenance{}) && p.Evidence == "" && p.Trace == "" && p.Verdict == "" && p.ResourceStop == nil
}
func cloneResult(p Result) Result {
	p.Acknowledged = append([]Capability(nil), p.Acknowledged...)
	if p.ResourceStop != nil {
		x := *p.ResourceStop
		p.ResourceStop = &x
	}
	return p
}
func requestDigest(version, schema, model, property, backendID, backendVersion string, capabilities []Capability, bounds Bounds, nonce [16]byte) string {
	h := sha256.New()
	for _, p := range []string{version, schema, model, property, backendID, backendVersion} {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	var count [2]byte
	binary.BigEndian.PutUint16(count[:], uint16(len(capabilities)))
	h.Write(count[:])
	for _, capability := range capabilities {
		p := string(capability)
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	for _, bound := range []uint64{bounds.States, bounds.Depth, bounds.Milliseconds, bounds.MemoryMB} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], bound)
		h.Write(n[:])
	}
	h.Write(nonce[:])
	return hex.EncodeToString(h.Sum(nil))
}
func allUTF8(v reflect.Value) bool {
	state := utf8WalkState{
		active:     map[reflect.Type]map[uintptr]bool{},
		expr:       map[*ir.Expr]exprMeasure{},
		exprActive: map[*ir.Expr]bool{},
	}
	return state.walk(v)
}

type utf8WalkState struct {
	active     map[reflect.Type]map[uintptr]bool
	expr       map[*ir.Expr]exprMeasure
	exprActive map[*ir.Expr]bool
	exprRoots  int
	runWork    int
	canonical  int64
	values     int
	textBytes  int
}

type exprMeasure struct{ nodes, height, textBytes, values int }

func (s *utf8WalkState) walk(v reflect.Value) bool {
	s.values++
	s.chargeCanonical(v)
	if !s.withinLimits() {
		return false
	}
	if !v.IsValid() {
		return true
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return true
		}
		return s.walk(v.Elem())
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		if expression, ok := v.Interface().(*ir.Expr); ok {
			if !s.measureExpr(expression, 1) {
				return false
			}
			s.exprRoots += s.expr[expression].nodes
			s.textBytes += s.expr[expression].textBytes
			s.values += s.expr[expression].values - 1
			s.canonical += int64(s.expr[expression].nodes) * 80
			return s.exprRoots <= ir.MaxIRWeight && s.withinLimits()
		}
		pointer := v.Pointer()
		active := s.active[v.Type()]
		if active == nil {
			active = map[uintptr]bool{}
			s.active[v.Type()] = active
		}
		if active[pointer] {
			return false
		}
		active[pointer] = true
		defer delete(active, pointer)
		if !s.walk(v.Elem()) {
			return false
		}
		if run, ok := v.Interface().(*ir.RunOp); ok && !s.chargeRunWork(run) {
			return false
		}
		return s.withinLimits()
	}
	if v.Kind() == reflect.String {
		text := v.String()
		s.textBytes += len(text)
		return s.withinLimits() && utf8.ValidString(text)
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" && !s.walk(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !s.walk(v.Index(i)) {
				return false
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
	return true
}

func (s *utf8WalkState) withinLimits() bool {
	if s.values > maxUTF8WalkValues || s.textBytes > maxUTF8WalkTextBytes {
		return false
	}
	return s.canonical+int64(s.textBytes)*6 <= maxCanonicalModelBytes
}

func (s *utf8WalkState) chargeCanonical(v reflect.Value) {
	charge := int64(maxCanonicalValueBytes)
	if v.IsValid() && v.Kind() == reflect.Struct {
		switch v.Type() {
		case reflect.TypeFor[ir.Assign]():
			charge = 200
		case reflect.TypeFor[ir.RecvArg]():
			charge = 160
		}
	}
	s.canonical += charge
}

func (s *utf8WalkState) chargeRunWork(run *ir.RunOp) bool {
	targets := len(run.Pool)
	if targets == 0 {
		targets = 1
	}
	workPerTarget := 1
	textPerTarget := 0
	for _, assignment := range run.Init {
		workPerTarget++
		textPerTarget += len(assignment.Var)
		for _, expression := range []*ir.Expr{assignment.Index, assignment.Value} {
			if expression == nil {
				continue
			}
			measure, ok := s.expr[expression]
			if !ok || measure.nodes > ir.MaxIRWeight-workPerTarget {
				return false
			}
			workPerTarget += measure.nodes
			textPerTarget += measure.textBytes
		}
	}
	if targets > (ir.MaxIRWeight-s.runWork)/workPerTarget {
		return false
	}
	if textPerTarget > 0 && targets > (maxUTF8WalkTextBytes-s.textBytes)/textPerTarget {
		return false
	}
	s.runWork += targets * workPerTarget
	s.textBytes += targets * textPerTarget
	return true
}

func (s *utf8WalkState) measureExpr(expression *ir.Expr, depth int) bool {
	if expression == nil {
		return true
	}
	if depth > ir.MaxExprHeight {
		return false
	}
	if _, ok := s.expr[expression]; ok {
		return true
	}
	if s.exprActive[expression] {
		return false
	}
	if !utf8.ValidString(expression.Op) || !utf8.ValidString(expression.Var) {
		return false
	}
	s.exprActive[expression] = true
	defer delete(s.exprActive, expression)
	measure := exprMeasure{nodes: 1, height: 1, textBytes: len(expression.Op) + len(expression.Var), values: 1}
	if measure.textBytes > maxUTF8WalkTextBytes {
		return false
	}
	for _, argument := range expression.Args {
		measure.values++
		if measure.values > maxUTF8WalkValues {
			return false
		}
		if argument == nil {
			return false
		}
		if !s.measureExpr(argument, depth+1) {
			return false
		}
		child := s.expr[argument]
		measure.nodes += child.nodes
		if measure.nodes > ir.MaxIRWeight {
			return false
		}
		measure.textBytes += child.textBytes
		if measure.textBytes > maxUTF8WalkTextBytes {
			return false
		}
		if child.height+1 > measure.height {
			measure.height = child.height + 1
		}
		measure.values += child.values
		if measure.values > maxUTF8WalkValues {
			return false
		}
	}
	if measure.height > ir.MaxExprHeight {
		return false
	}
	s.expr[expression] = measure
	return true
}
