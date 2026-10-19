package modelcheck_test

import (
	"fmt"

	"github.com/cucumber/godog"

	"modelcheck/backend"
	"modelcheck/ir"
)

func init() {
	stepRegistrars = append(stepRegistrars, registerG0BackendSteps)
}

type g0BackendWorld struct {
	request    backend.Request
	host       backend.HostFacts
	result     backend.Validated
	hostErr    error
	requestErr error
}

func registerG0BackendSteps(sc *godog.ScenarioContext) {
	w := &g0BackendWorld{}

	sc.Step(`^a valid M3\.0a backend request$`, func() error {
		var nonce [16]byte
		nonce[0] = 1
		request, err := backend.NewRequest(backend.RequestSpec{
			ContractVersion: backend.ContractVersion,
			BackendID:       "fixture",
			BackendVersion:  "v1",
			Model: &ir.Model{
				Schema: ir.Schema,
				Name:   "fixture",
				Processes: []ir.Process{{
					Name:      "P",
					Locations: []ir.Location{{Name: "end", Labels: []ir.Label{ir.End}}},
					Initial:   0,
				}},
			},
			Property:     []byte("fixture-property/1"),
			Nonce:        nonce,
			Capabilities: []backend.Capability{backend.CapabilityExactVerdicts},
			Bounds:       backend.Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1},
		})
		if err != nil {
			return fmt.Errorf("new backend request: %w", err)
		}
		host, err := backend.NewHostFacts(request, backend.HostFactsSpec{
			Terminal:              backend.HostNone,
			AvailableCapabilities: []backend.Capability{backend.CapabilityExactVerdicts},
		})
		if err != nil {
			return fmt.Errorf("new host facts: %w", err)
		}
		w.request, w.host = request, host
		return nil
	})

	sc.Step(`^I validate an exact verified backend result$`, func() error {
		w.result = backend.Validate(w.request, w.host, backend.Result{
			Status:         backend.StatusVerified,
			Identity:       w.request.Identity(),
			BackendID:      "fixture",
			BackendVersion: "v1",
			Acknowledged:   []backend.Capability{backend.CapabilityExactVerdicts},
			Provenance: backend.Provenance{
				BackendID: "fixture", BackendVersion: "v1", Implementation: "fixture", BuildIdentity: "fixture-build",
			},
			Evidence: backend.EvidenceExact,
			Trace:    backend.TraceNotApplicable,
			Verdict:  backend.VerdictVerified,
		})
		return nil
	})

	sc.Step(`^I create backend failure host facts with arbitrary text$`, func() error {
		_, w.hostErr = backend.NewHostFacts(w.request, backend.HostFactsSpec{
			AvailableCapabilities: []backend.Capability{backend.CapabilityExactVerdicts},
			Terminal:              backend.HostBackendFailure,
			BackendFailureReason:  backend.BackendFailureReason("open /private/host/path: permission denied"),
		})
		return nil
	})

	sc.Step(`^I validate the permitted backend row "([^"]+)"$`, func(row string) error {
		result := backend.Result{Status: backend.StatusVerified, Identity: w.request.Identity(), BackendID: "fixture", BackendVersion: "v1", Acknowledged: []backend.Capability{backend.CapabilityExactVerdicts}, Provenance: backend.Provenance{BackendID: "fixture", BackendVersion: "v1", Implementation: "fixture", BuildIdentity: "fixture-build"}, Evidence: backend.EvidenceExact, Trace: backend.TraceNotApplicable, Verdict: backend.VerdictVerified}
		switch row {
		case "inconclusive":
			result.Status, result.Verdict = backend.StatusInconclusive, ""
		case "resource":
			result.Status, result.Evidence, result.Verdict = backend.StatusResourceStop, "", ""
			result.ResourceStop = &backend.ResourceStop{Resource: backend.ResourceStates, Phase: backend.PhaseExecution, Limit: 1, Consumed: 1}
		case "verified":
		default:
			return fmt.Errorf("unknown backend row %q", row)
		}
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})

	sc.Step(`^I validate the host terminal "([^"]+)"$`, func(terminal string) error {
		spec := backend.HostFactsSpec{Terminal: backend.HostNone, AvailableCapabilities: []backend.Capability{backend.CapabilityExactVerdicts}}
		switch terminal {
		case "invalid-input":
			spec.Terminal, spec.InvalidInputReason = backend.HostInvalidInput, backend.InvalidInputReasonMalformedModel
		case "cancelled":
			spec.Terminal = backend.HostCancelled
		case "isolation-refused":
			spec.Terminal = backend.HostIsolationRefused
		case "backend-failure":
			spec.Terminal, spec.BackendFailureReason = backend.HostBackendFailure, backend.BackendFailureReasonExecution
		case "resource-stop":
			spec.Terminal, spec.ResourceStop = backend.HostResourceStop, &backend.ResourceStop{Resource: backend.ResourceStates, Phase: backend.PhaseAdmission, Limit: 1}
		default:
			return fmt.Errorf("unknown host terminal %q", terminal)
		}
		h, err := backend.NewHostFacts(w.request, spec)
		if err != nil {
			return err
		}
		w.result = backend.Validate(w.request, h, backend.Result{})
		return nil
	})

	sc.Step(`^I validate a backend result with (evidence|trace) "([^"]+)"$`, func(field, value string) error {
		result := backend.Result{Status: backend.StatusVerified, Identity: w.request.Identity(), BackendID: "fixture", BackendVersion: "v1", Acknowledged: []backend.Capability{backend.CapabilityExactVerdicts}, Provenance: backend.Provenance{BackendID: "fixture", BackendVersion: "v1", Implementation: "fixture", BuildIdentity: "fixture-build"}, Evidence: backend.EvidenceExact, Trace: backend.TraceNotApplicable, Verdict: backend.VerdictVerified}
		if field == "evidence" {
			result.Evidence = backend.Evidence(value)
		} else {
			result.Trace = backend.TraceAssurance(value)
		}
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})

	validResult := func() backend.Result {
		return backend.Result{Status: backend.StatusVerified, Identity: w.request.Identity(), BackendID: "fixture", BackendVersion: "v1", Acknowledged: []backend.Capability{backend.CapabilityExactVerdicts}, Provenance: backend.Provenance{BackendID: "fixture", BackendVersion: "v1", Implementation: "fixture", BuildIdentity: "fixture-build"}, Evidence: backend.EvidenceExact, Trace: backend.TraceNotApplicable, Verdict: backend.VerdictVerified}
	}
	sc.Step(`^I validate a backend result with a mismatched identity$`, func() error {
		result := validResult()
		result.Identity = "wrong"
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate a backend result with a mismatched (backend ID|backend version)$`, func(field string) error {
		result := validResult()
		if field == "backend ID" {
			result.BackendID = "other"
		} else {
			result.BackendVersion = "v2"
		}
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate a backend result without provenance$`, func() error {
		result := validResult()
		result.Provenance = backend.Provenance{}
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate without the requested backend capability$`, func() error {
		h, err := backend.NewHostFacts(w.request, backend.HostFactsSpec{Terminal: backend.HostNone})
		if err != nil {
			return err
		}
		w.result = backend.Validate(w.request, h, backend.Result{})
		return nil
	})
	sc.Step(`^I validate without the requested backend capability and a nonzero payload$`, func() error {
		h, err := backend.NewHostFacts(w.request, backend.HostFactsSpec{Terminal: backend.HostNone})
		if err != nil {
			return err
		}
		w.result = backend.Validate(w.request, h, validResult())
		return nil
	})
	sc.Step(`^I validate a backend result with an invalid capability acknowledgement$`, func() error {
		result := validResult()
		result.Acknowledged = nil
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate an approximate inconclusive backend result$`, func() error {
		result := validResult()
		result.Status, result.Evidence, result.Verdict = backend.StatusInconclusive, backend.EvidenceApproximate, ""
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate a violated backend result$`, func() error {
		result := validResult()
		result.Status, result.Verdict = backend.Terminal("violated"), ""
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I create an M3\.0a request with invalid (nonce|states|depth|milliseconds|memory)$`, func(field string) error {
		var nonce [16]byte
		nonce[0] = 1
		bounds := backend.Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}
		if field == "nonce" {
			nonce = [16]byte{}
		}
		switch field {
		case "states":
			bounds.States = 0
		case "depth":
			bounds.Depth = 0
		case "milliseconds":
			bounds.Milliseconds = 0
		case "memory":
			bounds.MemoryMB = 0
		}
		_, w.requestErr = backend.NewRequest(backend.RequestSpec{
			ContractVersion: backend.ContractVersion, BackendID: "fixture", BackendVersion: "v1",
			Model:    &ir.Model{Schema: ir.Schema, Name: "fixture", Processes: []ir.Process{{Name: "P", Locations: []ir.Location{{Name: "end", Labels: []ir.Label{ir.End}}}}}},
			Property: []byte("fixture-property/1"), Nonce: nonce, Capabilities: []backend.Capability{backend.CapabilityExactVerdicts}, Bounds: bounds,
		})
		return nil
	})
	sc.Step(`^I validate a backend result with invalid resource-stop (resource|limit|phase|admission consumption|execution consumption)$`, func(field string) error {
		result := validResult()
		result.Status, result.Evidence, result.Verdict = backend.StatusResourceStop, "", ""
		stop := &backend.ResourceStop{Resource: backend.ResourceStates, Phase: backend.PhaseExecution, Limit: 1, Consumed: 1}
		switch field {
		case "resource":
			stop.Resource = "unknown"
		case "limit":
			stop.Limit = 0
		case "phase":
			stop.Phase = "unknown"
		case "admission consumption":
			stop.Phase, stop.Consumed = backend.PhaseAdmission, 2
		case "execution consumption":
			stop.Consumed = 0
		}
		result.ResourceStop = stop
		w.result = backend.Validate(w.request, w.host, result)
		return nil
	})
	sc.Step(`^I validate a cancelled host terminal with a payload$`, func() error {
		h, err := backend.NewHostFacts(w.request, backend.HostFactsSpec{Terminal: backend.HostCancelled})
		if err != nil {
			return err
		}
		w.result = backend.Validate(w.request, h, validResult())
		return nil
	})
	sc.Step(`^I create an M3.0a request with a malformed model$`, func() error {
		var nonce [16]byte
		nonce[0] = 1
		_, w.requestErr = backend.NewRequest(backend.RequestSpec{ContractVersion: backend.ContractVersion, BackendID: "fixture", BackendVersion: "v1", Model: &ir.Model{Schema: ir.Schema}, Property: []byte("p"), Nonce: nonce, Capabilities: []backend.Capability{backend.CapabilityExactVerdicts}, Bounds: backend.Bounds{States: 1, Depth: 1, Milliseconds: 1, MemoryMB: 1}})
		return nil
	})

	sc.Step(`^the backend contract status is "([^"]+)"$`, func(want string) error {
		if got := string(w.result.Status()); got != want {
			return fmt.Errorf("backend contract status is %q, want %q", got, want)
		}
		return nil
	})

	sc.Step(`^the backend host facts are rejected$`, func() error {
		if w.hostErr == nil {
			return fmt.Errorf("arbitrary backend failure text was accepted")
		}
		return nil
	})
	sc.Step(`^the backend request is rejected$`, func() error {
		if w.requestErr == nil {
			return fmt.Errorf("malformed request was accepted")
		}
		return nil
	})
}
