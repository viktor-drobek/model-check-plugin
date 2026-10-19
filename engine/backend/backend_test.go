package backend

import (
	"bytes"
	"reflect"
	"sync"
	"testing"

	"modelcheck/ir"
)

func fixtureModel() *ir.Model {
	return &ir.Model{
		Schema: ir.Schema,
		Name:   "fixture",
		Processes: []ir.Process{{
			Name:      "P",
			Locations: []ir.Location{{Name: "end", Labels: []ir.Label{ir.End}}},
			Initial:   0,
		}},
	}
}

func testRequest(t *testing.T) Request {
	t.Helper()
	var nonce [16]byte
	nonce[0] = 1
	r, err := NewRequest(RequestSpec{
		ContractVersion: ContractVersion,
		BackendID:       "fixture",
		BackendVersion:  "v1",
		Model:           fixtureModel(),
		Property:        []byte("property/1"),
		Nonce:           nonce,
		Capabilities:    []Capability{CapabilityExactVerdicts},
		Bounds:          Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func testHost(t *testing.T, r Request, available []Capability) HostFacts {
	t.Helper()
	h, err := NewHostFacts(r, HostFactsSpec{Terminal: HostNone, AvailableCapabilities: available})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func exactResult(r Request) Result {
	return Result{
		Status:         StatusVerified,
		Identity:       r.Identity(),
		BackendID:      "fixture",
		BackendVersion: "v1",
		Acknowledged:   []Capability{CapabilityExactVerdicts},
		Provenance: Provenance{
			BackendID:      "fixture",
			BackendVersion: "v1",
			Implementation: "fixture",
			BuildIdentity:  "build",
		},
		Evidence: EvidenceExact,
		Trace:    TraceNotApplicable,
		Verdict:  VerdictVerified,
	}
}

func inconclusiveResult(r Request, evidence Evidence) Result {
	p := exactResult(r)
	p.Status = StatusInconclusive
	p.Evidence = evidence
	p.Verdict = ""
	return p
}

func resourceStopResult(r Request, phase ResourcePhase, consumed uint64) Result {
	p := exactResult(r)
	p.Status, p.Evidence, p.Trace, p.Verdict = StatusResourceStop, "", TraceNotApplicable, ""
	p.ResourceStop = &ResourceStop{Resource: ResourceStates, Phase: phase, Limit: 1, Consumed: consumed}
	return p
}

func TestValidateAcceptedRows(t *testing.T) {
	r := testRequest(t)
	for _, tc := range []struct {
		name  string
		value Result
	}{
		{"verified exact", exactResult(r)},
		{"inconclusive exact", inconclusiveResult(r, EvidenceExact)},
		{"inconclusive approximate", inconclusiveResult(r, EvidenceApproximate)},
		{"execution resource stop at bound", resourceStopResult(r, PhaseExecution, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), tc.value)
			if got.Status() != tc.value.Status || got.Reason() != "" {
				t.Fatalf("got status=%q reason=%q", got.Status(), got.Reason())
			}
		})
	}
}

func TestValidateFailsClosed(t *testing.T) {
	r := testRequest(t)
	for _, tc := range []struct {
		name   string
		host   []Capability
		mutate func(*Result)
		want   ContractInvalidReason
	}{
		{"identity mismatch", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Identity = "wrong" }, ReasonIdentityMismatch},
		{"backend ID mismatch", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.BackendID = "other" }, ReasonIdentityMismatch},
		{"backend version mismatch", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.BackendVersion = "other" }, ReasonIdentityMismatch},
		{"missing provenance", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Provenance = Provenance{} }, ReasonInvalidMetadata},
		{"provenance backend ID mismatch", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Provenance.BackendID = "other" }, ReasonInvalidMetadata},
		{"missing acknowledgement", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Acknowledged = nil }, ReasonInvalidMetadata},
		{"unapproved evidence", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Evidence = "certified-sound" }, ReasonInvalidResultRow},
		{"unapproved trace", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Trace = "replayed" }, ReasonInvalidTraceAssurance},
		{"backend declares contract invalid", []Capability{CapabilityExactVerdicts}, func(p *Result) { p.Status = StatusContractInvalid }, ReasonUndeclaredResultRow},
		{"invalid execution resource stop", []Capability{CapabilityExactVerdicts}, func(p *Result) { *p = resourceStopResult(r, PhaseExecution, 0) }, ReasonInvalidResourceStop},
		{"payload with unavailable capability", nil, func(*Result) {}, ReasonPayloadWithUnavailableCapability},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := exactResult(r)
			tc.mutate(&p)
			got := Validate(r, testHost(t, r, tc.host), p)
			if got.Status() != StatusContractInvalid || got.Reason() != ReasonCode(tc.want) {
				t.Fatalf("got status=%q reason=%q, want contract-invalid/%q", got.Status(), got.Reason(), tc.want)
			}
		})
	}
}

func TestValidatePinsFailurePrecedenceAndHostAuthority(t *testing.T) {
	r := testRequest(t)
	valid := testHost(t, r, []Capability{CapabilityExactVerdicts})
	other := r
	other.identity = "foreign"
	foreign, err := NewHostFacts(other, HostFactsSpec{Terminal: HostCancelled})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := NewHostFacts(r, HostFactsSpec{Terminal: HostCancelled})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		host HostFacts
		zero bool
		edit func(*Result)
		want ReasonCode
	}{
		{"foreign host before terminal", foreign, false, func(*Result) {}, ReasonCode(ReasonInvalidHostFacts)},
		{"host terminal before unavailable capability", cancelled, true, func(*Result) {}, ""},
		{"identity before metadata", valid, false, func(p *Result) { p.Identity, p.Acknowledged = "wrong", nil }, ReasonCode(ReasonIdentityMismatch)},
		{"metadata before terminal row", valid, false, func(p *Result) { p.Acknowledged, p.Status = nil, StatusViolated }, ReasonCode(ReasonInvalidMetadata)},
		{"terminal row before trace", valid, false, func(p *Result) { p.Status, p.Trace = StatusViolated, TraceReplayed }, ReasonCode(ReasonUndeclaredResultRow)},
		{"resource row before resource data", valid, false, func(p *Result) {
			p.Status, p.Evidence, p.Verdict, p.ResourceStop = StatusResourceStop, EvidenceExact, "", &ResourceStop{Resource: ResourceStates, Phase: PhaseExecution, Limit: 1, Consumed: 1}
		}, ReasonCode(ReasonInvalidResultRow)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Result{}
			if !tc.zero {
				p = exactResult(r)
			}
			tc.edit(&p)
			got := Validate(r, tc.host, p)
			if tc.want == "" {
				if got.Status() != StatusCancelled || got.Reason() != "" {
					t.Fatalf("host terminal = %q/%q", got.Status(), got.Reason())
				}
				return
			}
			if got.Status() != StatusContractInvalid || got.Reason() != tc.want {
				t.Fatalf("got %q/%q, want contract-invalid/%q", got.Status(), got.Reason(), tc.want)
			}
		})
	}
}

func TestUnavailableCapabilitySynthesizesOnlyCanonicalResult(t *testing.T) {
	r := testRequest(t)
	got := Validate(r, testHost(t, r, nil), Result{})
	want := Result{Status: StatusUnsupported, Identity: r.Identity()}
	if got.Status() != StatusUnsupported || got.Reason() != ReasonCode(ReasonExactVerdictsUnavailable) || !reflect.DeepEqual(got.Result(), want) {
		t.Fatalf("unsupported result = %+v, reason=%q", got.Result(), got.Reason())
	}
}

func TestHostTerminalRowsAndReasonEnums(t *testing.T) {
	r := testRequest(t)
	for _, tc := range []struct {
		name   string
		spec   HostFactsSpec
		want   Terminal
		reason ReasonCode
	}{
		{"invalid input", HostFactsSpec{Terminal: HostInvalidInput, InvalidInputReason: InvalidInputReasonMalformedModel}, StatusInvalidInput, ReasonCode(InvalidInputReasonMalformedModel)},
		{"cancelled", HostFactsSpec{Terminal: HostCancelled}, StatusCancelled, ""},
		{"isolation refused", HostFactsSpec{Terminal: HostIsolationRefused}, StatusIsolationRefused, ""},
		{"backend failure", HostFactsSpec{Terminal: HostBackendFailure, BackendFailureReason: BackendFailureReasonExecution}, StatusBackendFailure, ReasonCode(BackendFailureReasonExecution)},
		{"admission resource stop below bound", HostFactsSpec{Terminal: HostResourceStop, ResourceStop: &ResourceStop{Resource: ResourceStates, Phase: PhaseAdmission, Limit: 1, Consumed: 0}}, StatusResourceStop, ""},
		{"admission resource stop at bound", HostFactsSpec{Terminal: HostResourceStop, ResourceStop: &ResourceStop{Resource: ResourceStates, Phase: PhaseAdmission, Limit: 1, Consumed: 1}}, StatusResourceStop, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.AvailableCapabilities = []Capability{CapabilityExactVerdicts}
			h, err := NewHostFacts(r, tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			got := Validate(r, h, Result{})
			if got.Status() != tc.want || got.Reason() != tc.reason {
				t.Fatalf("got status=%q reason=%q", got.Status(), got.Reason())
			}
			if tc.want == StatusResourceStop && !reflect.DeepEqual(got.Result().ResourceStop, tc.spec.ResourceStop) {
				t.Fatalf("host resource stop = %+v, want %+v", got.Result().ResourceStop, tc.spec.ResourceStop)
			}
			if got := Validate(r, h, exactResult(r)); got.Status() != StatusContractInvalid || got.Reason() != ReasonCode(ReasonPayloadWithHostTerminal) {
				t.Fatalf("host terminal with payload = %q/%q", got.Status(), got.Reason())
			}
		})
	}
	for _, tc := range []HostFactsSpec{
		{Terminal: HostInvalidInput, InvalidInputReason: "host-path"},
		{Terminal: HostBackendFailure, BackendFailureReason: "host-path"},
		{Terminal: HostInvalidInput, BackendFailureReason: BackendFailureReasonExecution},
		{Terminal: HostResourceStop},
		{Terminal: HostResourceStop, ResourceStop: &ResourceStop{Resource: ResourceStates, Phase: PhaseAdmission, Limit: 1, Consumed: 2}},
	} {
		t.Run("rejected host facts", func(t *testing.T) {
			tc.AvailableCapabilities = []Capability{CapabilityExactVerdicts}
			if _, err := NewHostFacts(r, tc); err == nil {
				t.Fatal("accepted invalid host facts")
			}
		})
	}
}

func TestResourceStopBoundaries(t *testing.T) {
	r := testRequest(t)
	for _, tc := range []struct {
		phase ResourcePhase
		used  uint64
		valid bool
	}{
		{PhaseAdmission, 0, false},
		{PhaseAdmission, 1, false},
		{PhaseAdmission, 2, false},
		{PhaseExecution, 0, false},
		{PhaseExecution, 1, true},
		{PhaseExecution, ^uint64(0), false},
	} {
		got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), resourceStopResult(r, tc.phase, tc.used))
		if (got.Status() == StatusResourceStop) != tc.valid {
			t.Fatalf("phase=%q consumed=%d status=%q", tc.phase, tc.used, got.Status())
		}
	}
}

func TestNewRequestRejectsInvalidInput(t *testing.T) {
	valid := RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: fixtureModel(), Property: []byte("p"), Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}
	valid.Nonce[0] = 1
	for _, tc := range []struct {
		name   string
		mutate func(*RequestSpec)
	}{
		{"zero nonce", func(s *RequestSpec) { s.Nonce = [16]byte{} }},
		{"wrong contract version", func(s *RequestSpec) { s.ContractVersion = "other" }},
		{"empty backend ID", func(s *RequestSpec) { s.BackendID = "" }},
		{"empty backend version", func(s *RequestSpec) { s.BackendVersion = "" }},
		{"oversized backend ID", func(s *RequestSpec) { s.BackendID = string(make([]byte, maxTextBytes+1)) }},
		{"invalid UTF-8 backend version", func(s *RequestSpec) { s.BackendVersion = string([]byte{0xff}) }},
		{"zero states", func(s *RequestSpec) { s.Bounds.States = 0 }},
		{"zero depth", func(s *RequestSpec) { s.Bounds.Depth = 0 }},
		{"zero milliseconds", func(s *RequestSpec) { s.Bounds.Milliseconds = 0 }},
		{"zero memory", func(s *RequestSpec) { s.Bounds.MemoryMB = 0 }},
		{"oversized states", func(s *RequestSpec) { s.Bounds.States = maxBound + 1 }},
		{"oversized depth", func(s *RequestSpec) { s.Bounds.Depth = maxBound + 1 }},
		{"oversized milliseconds", func(s *RequestSpec) { s.Bounds.Milliseconds = maxBound + 1 }},
		{"oversized memory", func(s *RequestSpec) { s.Bounds.MemoryMB = maxBound + 1 }},
		{"wrong capability", func(s *RequestSpec) { s.Capabilities = nil }},
		{"missing model", func(s *RequestSpec) { s.Model = nil }},
		{"missing property", func(s *RequestSpec) { s.Property = nil }},
		{"oversized property", func(s *RequestSpec) { s.Property = make([]byte, maxPropertyBytes+1) }},
		{"invalid UTF-8 model", func(s *RequestSpec) { s.Model.Name = string([]byte{0xff}) }},
		{"invalid model", func(s *RequestSpec) { s.Model.Processes = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			s.Model = fixtureModel()
			tc.mutate(&s)
			if _, err := NewRequest(s); err == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
}

func TestRequestIdentityBindingAndInputCopies(t *testing.T) {
	r := testRequest(t)
	base := RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: fixtureModel(), Property: []byte("property/1"), Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}
	base.Nonce[0] = 1
	variants := []func(*RequestSpec){
		func(s *RequestSpec) { s.Model.Name = "other" },
		func(s *RequestSpec) { s.Property = []byte("property/2") },
		func(s *RequestSpec) { s.BackendID = "other" },
		func(s *RequestSpec) { s.BackendVersion = "v2" },
		func(s *RequestSpec) { s.Bounds.States = 2 },
		func(s *RequestSpec) { s.Bounds.Depth = 2 },
		func(s *RequestSpec) { s.Bounds.Milliseconds = 2 },
		func(s *RequestSpec) { s.Bounds.MemoryMB = 2 },
		func(s *RequestSpec) { s.Nonce[1] = 1 },
	}
	for _, mutate := range variants {
		s := base
		s.Model = fixtureModel()
		s.Property = append([]byte(nil), base.Property...)
		mutate(&s)
		changed, err := NewRequest(s)
		if err != nil {
			t.Fatal(err)
		}
		if changed.Identity() == r.Identity() {
			t.Fatal("identity did not bind changed request data")
		}
	}
	model := fixtureModel()
	property := []byte("property/1")
	capabilities := []Capability{CapabilityExactVerdicts}
	s := base
	s.Model, s.Property, s.Capabilities = model, property, capabilities
	copied, err := NewRequest(s)
	if err != nil {
		t.Fatal(err)
	}
	model.Name, property[0], capabilities[0] = "changed", 'X', "changed"
	if copied.Identity() != r.Identity() {
		t.Fatal("request retained caller-owned input")
	}
	if got := Validate(copied, testHost(t, copied, []Capability{CapabilityExactVerdicts}), exactResult(copied)); got.Status() != StatusVerified {
		t.Fatalf("request retained caller-owned capabilities: %q/%q", got.Status(), got.Reason())
	}
}

func TestValidateRejectsMalformedAcknowledgementAndProvenance(t *testing.T) {
	r := testRequest(t)
	h := testHost(t, r, []Capability{CapabilityExactVerdicts})
	for _, tc := range []struct {
		name string
		edit func(*Result)
	}{
		{"duplicate acknowledgement", func(p *Result) { p.Acknowledged = []Capability{CapabilityExactVerdicts, CapabilityExactVerdicts} }},
		{"extra acknowledgement", func(p *Result) { p.Acknowledged = []Capability{CapabilityExactVerdicts, "other"} }},
		{"provenance version mismatch", func(p *Result) { p.Provenance.BackendVersion = "v2" }},
		{"empty implementation", func(p *Result) { p.Provenance.Implementation = "" }},
		{"long build identity", func(p *Result) { p.Provenance.BuildIdentity = string(bytes.Repeat([]byte("x"), maxTextBytes+1)) }},
		{"invalid UTF-8 provenance", func(p *Result) { p.Provenance.Implementation = string([]byte{0xff}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := exactResult(r)
			tc.edit(&p)
			got := Validate(r, h, p)
			if got.Status() != StatusContractInvalid || got.Reason() != ReasonCode(ReasonInvalidMetadata) {
				t.Fatalf("got %q/%q", got.Status(), got.Reason())
			}
		})
	}
}

func TestRequestIdentityIsDeterministicForIndependentInput(t *testing.T) {
	first := testRequest(t)
	second := testRequest(t)
	if first.Identity() != second.Identity() {
		t.Fatalf("independent equivalent requests have identities %q and %q", first.Identity(), second.Identity())
	}
}

func TestNewRequestAcceptsMaximumPropertyAndBounds(t *testing.T) {
	var nonce [16]byte
	nonce[0] = 1
	_, err := NewRequest(RequestSpec{
		ContractVersion: ContractVersion,
		BackendID:       "fixture",
		BackendVersion:  "v1",
		Model:           fixtureModel(),
		Property:        bytes.Repeat([]byte("p"), maxPropertyBytes),
		Nonce:           nonce,
		Capabilities:    []Capability{CapabilityExactVerdicts},
		Bounds:          Bounds{States: maxBound, Depth: maxBound, Milliseconds: maxBound, MemoryMB: maxBound},
	})
	if err != nil {
		t.Fatalf("rejected maximum valid request: %v", err)
	}
}

func TestRequestDigestBindsCapabilitySet(t *testing.T) {
	var nonce [16]byte
	nonce[0] = 1
	bounds := Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}
	base := requestDigest(ContractVersion, ir.Schema, "model", "property", "fixture", "v1", []Capability{CapabilityExactVerdicts}, bounds, nonce)
	changed := requestDigest(ContractVersion, ir.Schema, "model", "property", "fixture", "v1", []Capability{"other"}, bounds, nonce)
	if base == changed {
		t.Fatal("request digest did not bind capabilities")
	}
}

func TestRequestGoldenCanonicalIRAndIdentity(t *testing.T) {
	const wantCanonical = "{\n" +
		"  \"schema\": \"mcd-ir/1\",\n" +
		"  \"name\": \"fixture\",\n" +
		"  \"processes\": [\n" +
		"    {\n" +
		"      \"name\": \"P\",\n" +
		"      \"locations\": [\n" +
		"        {\n" +
		"          \"name\": \"end\",\n" +
		"          \"labels\": [\n" +
		"            \"end\"\n" +
		"          ]\n" +
		"        }\n" +
		"      ],\n" +
		"      \"initial\": 0,\n" +
		"      \"edges\": null\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"
	const wantIdentity = "78521c0563c576433c6b3624800260901763dbd9543bf5ccf7d89d3d601f8ce0"
	canonical, err := ir.MarshalJSON(fixtureModel())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, []byte(wantCanonical)) {
		t.Fatalf("canonical IR changed:\n%s", canonical)
	}
	if got := testRequest(t).Identity(); got != wantIdentity {
		t.Fatalf("request identity = %q, want %q", got, wantIdentity)
	}
}

func TestValidatedAndHostFactsDoNotAliasInput(t *testing.T) {
	r := testRequest(t)
	available := []Capability{CapabilityExactVerdicts}
	h := testHost(t, r, available)
	available[0] = "changed"
	p := resourceStopResult(r, PhaseExecution, 1)
	v := Validate(r, h, p)
	p.Acknowledged[0] = "changed"
	p.ResourceStop.Consumed = 99
	first := v.Result()
	first.Acknowledged[0] = "changed-again"
	first.ResourceStop.Consumed = 99
	second := v.Result()
	if second.Acknowledged[0] != CapabilityExactVerdicts || second.ResourceStop.Consumed != 1 {
		t.Fatalf("validated result aliased: %+v", second)
	}
	stop := &ResourceStop{Resource: ResourceStates, Phase: PhaseExecution, Limit: 1, Consumed: 1}
	h, err := NewHostFacts(r, HostFactsSpec{Terminal: HostResourceStop, ResourceStop: stop})
	if err != nil {
		t.Fatal(err)
	}
	stop.Consumed = 99
	if got := Validate(r, h, Result{}).Result().ResourceStop; got == nil || got.Consumed != 1 {
		t.Fatalf("host facts aliased resource stop: %+v", got)
	}
}

func TestValidateIsSafeUnderParallelAccess(t *testing.T) {
	r := testRequest(t)
	h := testHost(t, r, []Capability{CapabilityExactVerdicts})
	p := exactResult(r)
	var group sync.WaitGroup
	for range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			got := Validate(r, h, p)
			result := got.Result()
			result.Acknowledged[0] = "mutated"
			if got.Status() != StatusVerified || got.Result().Identity != r.Identity() || got.Result().Acknowledged[0] != CapabilityExactVerdicts {
				t.Errorf("unexpected validated result")
			}
		}()
	}
	group.Wait()
}

func TestNewRequestAndNewHostFactsAreSafeUnderParallelAccess(t *testing.T) {
	var nonce [16]byte
	nonce[0] = 1
	spec := RequestSpec{
		ContractVersion: ContractVersion,
		BackendID:       "fixture",
		BackendVersion:  "v1",
		Model:           fixtureModel(),
		Property:        []byte("property/1"),
		Nonce:           nonce,
		Capabilities:    []Capability{CapabilityExactVerdicts},
		Bounds:          Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1},
	}
	available := []Capability{CapabilityExactVerdicts}
	identities := make(map[string]struct{}, 64)
	var identitiesMu sync.Mutex
	var group sync.WaitGroup
	for i := range 64 {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			requestSpec := spec
			requestSpec.Nonce[1] = byte(i + 1)
			r, err := NewRequest(requestSpec)
			if err != nil {
				t.Errorf("NewRequest: %v", err)
				return
			}
			identitiesMu.Lock()
			_, duplicate := identities[r.Identity()]
			identities[r.Identity()] = struct{}{}
			identitiesMu.Unlock()
			if duplicate {
				t.Errorf("duplicate request identity %q", r.Identity())
			}
			h, err := NewHostFacts(r, HostFactsSpec{Terminal: HostNone, AvailableCapabilities: available})
			if err != nil {
				t.Errorf("NewHostFacts: %v", err)
				return
			}
			if got := Validate(r, h, exactResult(r)); got.Status() != StatusVerified || got.Reason() != "" {
				t.Errorf("validated result = %q/%q", got.Status(), got.Reason())
			}
		}(i)
	}
	group.Wait()
	if got := len(identities); got != 64 {
		t.Fatalf("unique request identities = %d, want 64", got)
	}
}

func TestValidatedAccessorsAreSafeUnderParallelAccess(t *testing.T) {
	r := testRequest(t)
	v := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), exactResult(r))
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			result := v.Result()
			result.Acknowledged[0] = "mutated"
			if v.Status() != StatusVerified || v.Reason() != "" || v.Result().Acknowledged[0] != CapabilityExactVerdicts {
				t.Error("validated accessors exposed mutable state")
			}
		}()
	}
	group.Wait()
}

func TestNewRequestRejectsCyclicIR(t *testing.T) {
	m := fixtureModel()
	expr := &ir.Expr{Op: "!"}
	expr.Args = []*ir.Expr{expr}
	m.Processes[0].Edges = []ir.Edge{{From: 0, To: 0, Guard: expr}}
	var nonce [16]byte
	nonce[0] = 1
	if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
		t.Fatal("accepted cyclic model")
	}
}

func TestUTF8WalkAcceptsSharedIRExpression(t *testing.T) {
	m := fixtureModel()
	shared := ir.Const(1)
	m.Processes[0].Edges = []ir.Edge{{From: 0, To: 0, Guard: ir.Binary("+", shared, shared)}}
	if !allUTF8(reflect.ValueOf(m)) {
		t.Fatal("rejected a shared expression")
	}
}

func TestNewRequestRejectsNestedInvalidUTF8AndMutualCycles(t *testing.T) {
	invalid := string([]byte{0xff})
	for _, tc := range []struct {
		name   string
		mutate func(*ir.Model)
	}{
		{"edge text", func(m *ir.Model) { m.Processes[0].Edges = []ir.Edge{{Text: invalid}} }},
		{"origin file", func(m *ir.Model) { m.Processes[0].Origin = &ir.Origin{File: invalid} }},
		{"channel selection", func(m *ir.Model) { m.Processes[0].Edges = []ir.Edge{{Send: &ir.ChanOp{Chan: invalid}}} }},
		{"property formula", func(m *ir.Model) { m.Properties = []ir.Property{{ID: "p", Kind: ir.KindLTL, Formula: invalid}} }},
		{"mutual expression cycle", func(m *ir.Model) {
			a, b := &ir.Expr{Op: "add"}, &ir.Expr{Op: "add"}
			a.Args, b.Args = []*ir.Expr{b, b}, []*ir.Expr{a, a}
			m.Processes[0].Edges = []ir.Edge{{Guard: a, Send: &ir.ChanOp{Sel: b}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fixtureModel()
			tc.mutate(m)
			if allUTF8(reflect.ValueOf(m)) {
				t.Fatal("accepted malformed model text or expression cycle")
			}
			var nonce [16]byte
			nonce[0] = 1
			if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
				t.Fatal("accepted malformed model")
			}
		})
	}
}

func TestExpressionGraphBounds(t *testing.T) {
	chain := ir.Const(1)
	for range ir.MaxExprHeight - 1 {
		chain = ir.Unary("!", chain)
	}
	if !allUTF8(reflect.ValueOf(chain)) {
		t.Fatal("rejected expression at maximum height")
	}
	tooTall := ir.Unary("!", chain)
	if allUTF8(reflect.ValueOf(tooTall)) {
		t.Fatal("accepted expression above maximum height")
	}
	dag := ir.Const(1)
	for range 40 {
		dag = ir.Binary("+", dag, dag)
	}
	if allUTF8(reflect.ValueOf(dag)) {
		t.Fatal("accepted exponential expression DAG")
	}
}

func TestExpressionBudgetCountsSharedContainersPerUse(t *testing.T) {
	dag := ir.Const(1)
	for range 20 {
		dag = ir.Binary("+", dag, dag)
	}
	shared := &ir.ChanOp{Sel: dag}
	one := &ir.Model{Processes: []ir.Process{{Edges: []ir.Edge{{Send: shared}}}}}
	if !allUTF8(reflect.ValueOf(one)) {
		t.Fatal("rejected one bounded expression root")
	}
	two := &ir.Model{Processes: []ir.Process{{Edges: []ir.Edge{{Send: shared}, {Send: shared}}}}}
	if allUTF8(reflect.ValueOf(two)) {
		t.Fatal("accepted shared container above model expression budget")
	}
}

func TestUTF8WalkBoundsSharedContainers(t *testing.T) {
	shared := make([]ir.Edge, 1_000)
	m := fixtureModel()
	m.Processes = make([]ir.Process, 20_000)
	for i := range m.Processes {
		m.Processes[i] = ir.Process{Name: "P", Locations: []ir.Location{{Name: "end", Labels: []ir.Label{ir.End}}}, Edges: shared}
	}
	if allUTF8(reflect.ValueOf(m)) {
		t.Fatal("accepted shared containers above generic walk budget")
	}
}

func TestUTF8WalkBoundsSharedTextBytes(t *testing.T) {
	shared := string(bytes.Repeat([]byte("x"), 1<<20))
	m := fixtureModel()
	m.Processes[0].Edges = make([]ir.Edge, 17)
	for i := range m.Processes[0].Edges {
		m.Processes[0].Edges[i].Text = shared
	}
	if allUTF8(reflect.ValueOf(m)) {
		t.Fatal("accepted shared text above aggregate UTF-8 budget")
	}
	var nonce [16]byte
	nonce[0] = 1
	if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
		t.Fatal("accepted model above aggregate UTF-8 budget")
	}
}

func TestUTF8WalkBoundsSharedExpressionTextBytes(t *testing.T) {
	leaf := &ir.Expr{Op: "const", Var: string(bytes.Repeat([]byte("x"), 1<<20))}
	expression := leaf
	for range 4 {
		expression = ir.Binary("add", expression, expression)
	}
	m := fixtureModel()
	m.Processes[0].Edges = []ir.Edge{{Guard: expression}}
	if allUTF8(reflect.ValueOf(m)) {
		t.Fatal("accepted shared expression text above aggregate UTF-8 budget")
	}
	var nonce [16]byte
	nonce[0] = 1
	if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
		t.Fatal("accepted shared expression text above aggregate UTF-8 budget")
	}
}

func TestUTF8WalkBoundsSharedNilExpressionArguments(t *testing.T) {
	expression := &ir.Expr{Op: "ignored", Args: make([]*ir.Expr, 1<<20)}
	m := fixtureModel()
	m.Properties = make([]ir.Property, 17)
	for i := range m.Properties {
		m.Properties[i] = ir.Property{ID: string(rune('a' + i)), Kind: ir.KindDeadlock, Expr: expression}
	}
	if allUTF8(reflect.ValueOf(m)) {
		t.Fatal("accepted shared nil expression arguments above work budget")
	}
	var nonce [16]byte
	nonce[0] = 1
	if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
		t.Fatal("accepted shared nil expression arguments above work budget")
	}
}

func TestNewRequestRejectsNilExpressionArguments(t *testing.T) {
	m := fixtureModel()
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock, Expr: &ir.Expr{Op: "ignored", Args: []*ir.Expr{nil}}}}
	var nonce [16]byte
	nonce[0] = 1
	if _, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: m, Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}); err == nil {
		t.Fatal("accepted a nil expression argument")
	}
}

func TestUTF8WalkBoundsRunValidationWork(t *testing.T) {
	shared := ir.Const(1)
	run := &ir.RunOp{Pool: make([]int, 3_000), Init: make([]ir.Assign, 2_000)}
	for i := range run.Init {
		run.Init[i] = ir.Assign{Value: shared}
	}
	if allUTF8(reflect.ValueOf(run)) {
		t.Fatal("accepted run validation work above the model budget")
	}
}

func TestUTF8WalkBoundsCanonicalModelEstimate(t *testing.T) {
	state := utf8WalkState{canonical: maxCanonicalModelBytes + 1}
	if state.withinLimits() {
		t.Fatal("accepted canonical JSON estimate above the budget")
	}
}

func TestZeroAndForeignContractValuesFailClosed(t *testing.T) {
	r := testRequest(t)
	if got := Validate(Request{}, HostFacts{}, Result{}); got.Status() != StatusContractInvalid || got.Reason() != ReasonCode(ReasonInvalidRequest) {
		t.Fatalf("zero request = %q/%q", got.Status(), got.Reason())
	}
	if got := Validate(r, HostFacts{}, Result{}); got.Status() != StatusContractInvalid || got.Reason() != ReasonCode(ReasonInvalidHostFacts) {
		t.Fatalf("zero host = %q/%q", got.Status(), got.Reason())
	}
	otherSpec := RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: fixtureModel(), Property: []byte("property/1"), Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}}
	otherSpec.Nonce[1] = 1
	other, err := NewRequest(otherSpec)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := NewHostFacts(other, HostFactsSpec{Terminal: HostNone})
	if err != nil {
		t.Fatal(err)
	}
	if got := Validate(r, foreign, Result{}); got.Reason() != ReasonCode(ReasonInvalidHostFacts) {
		t.Fatalf("foreign host = %q", got.Reason())
	}
	var zero Validated
	if zero.Status() != StatusContractInvalid || zero.Reason() != ReasonCode(ReasonZeroValidated) || zero.Result().Identity != "" {
		t.Fatalf("zero validated = %+v/%q", zero.Result(), zero.Reason())
	}
}

func TestNewHostFactsRejectsInvalidCapabilitySets(t *testing.T) {
	r := testRequest(t)
	for _, capabilities := range [][]Capability{{"unknown"}, {CapabilityExactVerdicts, CapabilityExactVerdicts}, {"z", CapabilityExactVerdicts}} {
		if _, err := NewHostFacts(r, HostFactsSpec{Terminal: HostNone, AvailableCapabilities: capabilities}); err == nil {
			t.Fatalf("accepted capabilities %q", capabilities)
		}
	}
	if _, err := NewHostFacts(Request{}, HostFactsSpec{Terminal: HostNone}); err == nil {
		t.Fatal("accepted zero request")
	}
	if _, err := NewHostFacts(r, HostFactsSpec{Terminal: HostTerminal("unknown")}); err == nil {
		t.Fatal("accepted unknown terminal")
	}
}

func TestPayloadTableRejectsEveryOtherBasicTuple(t *testing.T) {
	r := testRequest(t)
	h := testHost(t, r, []Capability{CapabilityExactVerdicts})
	statuses := []Terminal{"", StatusVerified, Terminal("violated"), StatusInconclusive, StatusResourceStop, StatusUnsupported, StatusInvalidInput, StatusCancelled, StatusIsolationRefused, StatusBackendFailure, StatusContractInvalid, "unknown"}
	evidences := []Evidence{"", EvidenceExact, EvidenceApproximate, "certified-sound", "oracle-exception-reduced", "unknown"}
	traces := []TraceAssurance{"", TraceNotApplicable, "replayed", "trace-absent-approved", "replay-unavailable-approved", "unknown"}
	verdicts := []Verdict{"", VerdictVerified, "violated"}
	acks := [][]Capability{nil, {CapabilityExactVerdicts}, {"unknown"}}
	provenances := []Provenance{{}, exactResult(r).Provenance}
	stops := []*ResourceStop{nil, {Resource: ResourceStates, Phase: PhaseExecution, Limit: 1, Consumed: 1}}
	for _, status := range statuses {
		for _, evidence := range evidences {
			for _, trace := range traces {
				for _, verdict := range verdicts {
					for _, acknowledged := range acks {
						for _, provenance := range provenances {
							for _, stop := range stops {
								p := exactResult(r)
								p.Status, p.Evidence, p.Trace, p.Verdict, p.Acknowledged, p.Provenance, p.ResourceStop = status, evidence, trace, verdict, acknowledged, provenance, stop
								valid := (status == StatusVerified && evidence == EvidenceExact && trace == TraceNotApplicable && verdict == VerdictVerified && stop == nil) ||
									(status == StatusInconclusive && (evidence == EvidenceExact || evidence == EvidenceApproximate) && trace == TraceNotApplicable && verdict == "" && stop == nil) ||
									(status == StatusResourceStop && evidence == "" && trace == TraceNotApplicable && verdict == "" && stop != nil)
								valid = valid && len(acknowledged) == 1 && acknowledged[0] == CapabilityExactVerdicts && provenance == exactResult(r).Provenance
								got := Validate(r, h, p)
								if valid {
									if got.Status() != status || got.Reason() != "" || !reflect.DeepEqual(got.Result(), p) {
										t.Fatalf("valid status=%q evidence=%q trace=%q verdict=%q ack=%q provenance=%+v stop=%+v got=%+v reason=%q", status, evidence, trace, verdict, acknowledged, provenance, stop, got.Result(), got.Reason())
									}
									continue
								}
								want := Result{Status: StatusContractInvalid, Identity: r.Identity()}
								if got.Status() != StatusContractInvalid || got.Reason() == "" || !reflect.DeepEqual(got.Result(), want) {
									t.Fatalf("invalid status=%q evidence=%q trace=%q verdict=%q ack=%q provenance=%+v stop=%+v got=%+v reason=%q", status, evidence, trace, verdict, acknowledged, provenance, stop, got.Result(), got.Reason())
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestValidateChecksTraceAssuranceBeforeResultRow(t *testing.T) {
	r := testRequest(t)
	p := exactResult(r)
	p.Evidence = Evidence("not-exact")
	p.Trace = TraceAssurance("replayed")
	got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), p)
	if got.Reason() != ReasonCode(ReasonInvalidTraceAssurance) {
		t.Fatalf("reason = %q, want %q", got.Reason(), ReasonInvalidTraceAssurance)
	}
}

func TestResourceStopBindsEveryResourceLimit(t *testing.T) {
	var nonce [16]byte
	nonce[0] = 1
	r, err := NewRequest(RequestSpec{ContractVersion: ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: fixtureModel(), Property: []byte("p"), Nonce: nonce, Capabilities: []Capability{CapabilityExactVerdicts}, Bounds: Bounds{States: 2, Depth: 3, Milliseconds: 4, MemoryMB: 5}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		resource Resource
		limit    uint64
	}{{ResourceStates, 2}, {ResourceDepth, 3}, {ResourceMilliseconds, 4}, {ResourceMemoryMB, 5}} {
		p := resourceStopResult(r, PhaseExecution, tc.limit)
		p.ResourceStop.Resource, p.ResourceStop.Limit = tc.resource, tc.limit
		if got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), p); got.Status() != StatusResourceStop {
			t.Fatalf("resource %q: %q", tc.resource, got.Status())
		}
		p.ResourceStop.Limit++
		if got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), p); got.Reason() != ReasonCode(ReasonInvalidResourceStop) {
			t.Fatalf("resource %q wrong limit: %q", tc.resource, got.Reason())
		}
	}
}

func TestResourceStopRejectsInvalidFieldsForPayloadAndHost(t *testing.T) {
	r := testRequest(t)
	for _, tc := range []struct {
		name   string
		mutate func(*ResourceStop)
	}{
		{"unknown resource", func(s *ResourceStop) { s.Resource = "unknown" }},
		{"wrong limit", func(s *ResourceStop) { s.Limit = 0 }},
		{"unknown phase", func(s *ResourceStop) { s.Phase = "unknown" }},
		{"admission above limit", func(s *ResourceStop) { s.Phase, s.Consumed = PhaseAdmission, 2 }},
		{"execution below limit", func(s *ResourceStop) { s.Consumed = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stop := ResourceStop{Resource: ResourceStates, Phase: PhaseExecution, Limit: 1, Consumed: 1}
			tc.mutate(&stop)
			payload := resourceStopResult(r, stop.Phase, stop.Consumed)
			payload.ResourceStop = &stop
			if got := Validate(r, testHost(t, r, []Capability{CapabilityExactVerdicts}), payload); got.Reason() != ReasonCode(ReasonInvalidResourceStop) {
				t.Fatalf("payload reason = %q, want %q", got.Reason(), ReasonInvalidResourceStop)
			}
			if _, err := NewHostFacts(r, HostFactsSpec{Terminal: HostResourceStop, ResourceStop: &stop}); err == nil {
				t.Fatal("accepted invalid resource stop host fact")
			}
		})
	}
}
