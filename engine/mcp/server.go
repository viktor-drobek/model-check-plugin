// Package mcp is the MCP server of plan 14 §6: the seven tools mc_parse,
// mc_simulate, mc_check, mc_explain, mc_lint_property, mc_estimate and
// mc_manifest over stdio, on the official Go SDK
// (github.com/modelcontextprotocol/go-sdk, mcp.AddTool with typed handlers;
// input and output schemas are inferred from the Go structs and their
// `jsonschema` tags).
//
// Three kinds of answers are kept apart (NFR-007: syntax / model / tool /
// resource errors are distinguishable):
//
//   - A tool failure — bad arguments, a policy refusal, an internal error —
//     is a Go error from the handler; the SDK turns it into an MCP result
//     with isError = true and a text message, and nothing is claimed about
//     the model.
//   - An input rejection — a frontend refuses the input (schema violation,
//     unsupported construct, IR that does not validate or compile) — is, for
//     the two tools that produce a result document (mc_parse, mc_check), a
//     structured answer with `outcome: "rejected"` and a Rejection
//     {kind, construct, file, line, reason}; no property gets a status. The
//     tools that produce no result document (mc_simulate, mc_lint_property,
//     mc_estimate) report a rejected inline IR as an isError result whose
//     text starts with "rejected input (<kind>):", so that the class stays
//     recognisable (see rejectedInput).
//   - A verification result carries, per property, exactly one status of the
//     11 §14 vocabulary and one evidence level, with the meanings fixed by
//     package report. Kinds ltl, ctl and progress are `not-executed` in G2
//     with a reason naming the missing capability and the step (G4/G5) that
//     brings it; the server never fabricates a verdict.
//
// Every tool answer names its session id. Large results are files under
// <base>/<session id>/ and the answer carries their paths; every file the
// tools produce goes through Session.WriteFile and therefore through
// Resolve, the guard that refuses names leaving the session directory
// (NFR-004). Budgets are applied inside the server (Clamp): a field left at
// 0 takes the default, a field above the ceiling is clamped and the answer
// says so; a per-call deadline is set from the applied time budget;
// concurrent mc_check and mc_estimate runs are bounded by a semaphore.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/ir"
	"modelcheck/report"
)

// ToolNames are the seven tools, in the order they are registered.
var ToolNames = []string{"mc_parse", "mc_simulate", "mc_check", "mc_explain", "mc_lint_property", "mc_estimate", "mc_manifest"}

// PromelaFrontend parses Promela source. It returns a model, or a
// Rejection for input outside the subset, or an error for a tool failure.
// The G1 frontend is plugged in here by cmd/mcd when it is linked.
type PromelaFrontend func(src string, defines map[string]string, file string) (*ir.Model, *Rejection, error)

// Config is the server configuration; zero values take the defaults noted.
type Config struct {
	// SessionBase is the directory holding one subdirectory per session;
	// "" means a fresh temporary directory.
	SessionBase string
	// Cleanup removes the session directories on Close (opt-in).
	Cleanup bool
	// AllowRead lists directories whose files mc_parse may read when the
	// client names them; empty means inline inputs only.
	AllowRead []string
	// Default is the budget used for fields the client leaves at 0.
	Default Budget
	// Ceiling bounds client budgets; 0 in a field means no ceiling.
	Ceiling Budget
	// Concurrency bounds simultaneous mc_check/mc_estimate runs (default 2).
	Concurrency int
	// Promela is nil when no Promela frontend is linked in this build.
	Promela PromelaFrontend
}

func (c *Config) normalize() error {
	if c.Default == (Budget{}) {
		c.Default = DefaultBudget
	}
	c.Default = within(c.Default, c.Ceiling)
	if c.Concurrency <= 0 {
		c.Concurrency = 2
	}
	var allow []string
	for _, d := range c.AllowRead {
		abs, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return fmt.Errorf("--allow-read %q: %w", d, err)
		}
		allow = append(allow, real)
	}
	c.AllowRead = allow
	return nil
}

// Server wraps the SDK server, the sessions and the policy.
type Server struct {
	cfg      Config
	sessions *Sessions
	sem      chan struct{}
	sdk      *sdk.Server
}

// New builds a server with the seven tools registered.
func New(cfg Config) (*Server, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	ss, err := NewSessions(cfg.SessionBase, cfg.Cleanup, ServerParams{
		DefaultBudget: cfg.Default, Ceiling: cfg.Ceiling, Concurrency: cfg.Concurrency, AllowRead: cfg.AllowRead,
	})
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, sessions: ss, sem: make(chan struct{}, cfg.Concurrency)}
	s.sdk = sdk.NewServer(&sdk.Implementation{Name: report.EngineName, Version: report.EngineVersion}, &sdk.ServerOptions{
		Instructions: "Model-check engine (plan 14 §6). Typically call mc_parse first; it returns a session id that the other tools take (they also accept an inline `ir`). " +
			"Statuses per property: verified, violated, inconclusive, unknown, not-executed, invalid-model; evidence: exhaustive, bounded, approximate, unknown.",
	})
	s.register()
	return s, nil
}

func (s *Server) register() {
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_parse", Description: "Translate a model (Promela source, Petri-net JSON, or IR JSON; inline, or a file under an --allow-read prefix) into the IR. Answer: outcome ir (with the IR, its session file and the origin table), rejected (structured rejection with construct/file/line/reason), or not-executed (frontend missing in this build). Creates a session unless session_id is given."}, s.parse)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_simulate", Description: "Run the model step by step from the initial state: mode random (seeded, reproducible) or guided (a list of edge ids). Returns the trace with per-step variable changes and why it stopped (steps, deadlock, terminated, edge not enabled, edges exhausted, assert failed, invalid-model)."}, s.simulate)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_check", Description: "Check properties (invariant, deadlock, reach; ltl/ctl/progress are not-executed until G4/G5) by DFS or BFS within a budget the server clamps to its ceiling. Per property: status, evidence, counters, complete, counterexample/witness reference, or reason. The full report is a session file."}, s.check)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_explain", Description: "Explain a counterexample or witness by id: prefix steps with per-step variable diffs and the user's names for the commands, the loop part (empty in G2), and the final state."}, s.explain)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_lint_property", Description: "Lint a boolean state expression against the IR: atoms, undefined atoms, type check, class (safety for invariant, reachability for reach), X-free and temporal flags, vacuity notes (constant expressions)."}, s.lint)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_estimate", Description: "Estimate the state space by partial breadth-first exploration within a time limit: states visited, states per second, states per depth level, growth rate and a projection marked approximate. Not a verification result."}, s.estimate)
	sdk.AddTool(s.sdk, &sdk.Tool{Name: "mc_manifest", Description: "Return the reproducibility manifest of a session: engine and schema versions, server parameters, input hashes, and every tool call with parameters, seed, timing and artefacts."}, s.manifest)
}

// SDK exposes the underlying server (tests connect it to an in-memory
// transport).
func (s *Server) SDK() *sdk.Server { return s.sdk }

// Sessions exposes the session registry (tests inspect directories).
func (s *Server) Sessions() *Sessions { return s.sessions }

// Run serves one stdio connection until the client disconnects or ctx ends.
func (s *Server) Run(ctx context.Context) error {
	defer s.sessions.Close()
	return s.sdk.Run(ctx, &sdk.StdioTransport{})
}

// Connect serves a session over t (in-memory transport in tests).
func (s *Server) Connect(ctx context.Context, t sdk.Transport) (*sdk.ServerSession, error) {
	return s.sdk.Connect(ctx, t, nil)
}

// Close applies the opt-in cleanup.
func (s *Server) Close() error { return s.sessions.Close() }

// acquire takes a slot of the concurrency semaphore or fails when ctx ends
// first.
func (s *Server) acquire(ctx context.Context) (func(), error) {
	select {
	case s.sem <- struct{}{}:
		return func() { <-s.sem }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for a free check slot: %w", ctx.Err())
	}
}

// Rejection is a frontend's refusal of an input (NFR-007 syntax/model
// class), never a tool failure.
type Rejection struct {
	Kind      string `json:"kind" jsonschema:"schema | unsupported-input | ir — the class of refusal"`
	Construct string `json:"construct" jsonschema:"the construct or schema path refused"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Reason    string `json:"reason"`
}

// rejectedInput is the tool-error form of a Rejection, for tools that have
// no result document to carry a structured one. The fixed prefix keeps the
// NFR-007 class (input refused by a frontend) distinguishable from other
// tool failures.
func rejectedInput(r *Rejection) error {
	return fmt.Errorf("rejected input (%s): %s", r.Kind, r.Reason)
}

// session returns the named session or, when id is empty and create is
// set, a new one.
func (s *Server) session(id string, create bool) (*Session, error) {
	if id != "" {
		return s.sessions.Get(id)
	}
	if !create {
		return nil, errors.New("session_id is required")
	}
	return s.sessions.New()
}

// modelSource is where a model came from, for the manifest and the report.
type modelSource struct {
	kind   string // petri | ir | promela
	source string // "inline" or the file read
	data   []byte // bytes as received (hashed)
}

func (ms modelSource) input() report.Input {
	sum := sha256.Sum256(ms.data)
	return report.Input{Kind: ms.kind, Path: ms.source, SHA256: hex.EncodeToString(sum[:])}
}

// modelFor returns the model a tool works on: the inline IR when given
// (validated, recorded as an input of the session), else the session's
// parsed model. A validation failure is a Rejection.
func (s *Server) modelFor(sess *Session, inline any) (*ir.Model, *Rejection, error) {
	if inline == nil {
		sess.mu.Lock()
		m := sess.model
		sess.mu.Unlock()
		if m == nil {
			return nil, nil, errors.New("no model: pass `ir` inline or call mc_parse in this session first")
		}
		return m, nil, nil
	}
	data, err := json.Marshal(inline)
	if err != nil {
		return nil, nil, fmt.Errorf("ir: %v", err)
	}
	m, err := ir.UnmarshalJSON(data)
	if err != nil {
		return nil, &Rejection{Kind: "ir", Construct: "ir", Reason: err.Error()}, nil
	}
	if err := s.adopt(sess, m, modelSource{kind: "ir", source: "inline", data: data}); err != nil {
		return nil, nil, err
	}
	return m, nil, nil
}

// adopt makes m the session's model, writes its canonical JSON and records
// the input in the manifest. It returns the session path of the IR file.
func (s *Server) adopt(sess *Session, m *ir.Model, src modelSource) error {
	canon, err := ir.MarshalJSON(m)
	if err != nil {
		return err
	}
	sess.mu.Lock()
	rel := sess.next("ir", ".json")
	sess.mu.Unlock()
	if _, err := sess.WriteFile(rel, canon); err != nil {
		return err
	}
	in := src.input()
	mi := ManifestInput{Kind: in.Kind, Source: in.Path, SHA256: in.SHA256, Path: rel}
	sess.mu.Lock()
	sess.model, sess.modelBytes, sess.modelInput = m, canon, mi
	sess.mu.Unlock()
	sess.addInput(mi)
	return nil
}

// allowedRead checks a client-named file against the --allow-read prefixes
// and returns its resolved path.
func (s *Server) allowedRead(path string) (string, error) {
	if len(s.cfg.AllowRead) == 0 {
		return "", fmt.Errorf("reading %q is not allowed: the server was started without --allow-read; pass the input inline", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("reading %q: %v", path, err)
	}
	for _, prefix := range s.cfg.AllowRead {
		rel, err := filepath.Rel(prefix, real)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("reading %q is not allowed: no --allow-read prefix covers it; pass the input inline", path)
}

// callTimer logs a call in the session manifest.
type callTimer struct {
	sess    *Session
	i       int
	started time.Time
}

func begin(sess *Session, tool string) *callTimer {
	now := time.Now()
	return &callTimer{sess: sess, i: sess.beginCall(tool, now), started: now}
}

func (c *callTimer) end(err error, params *Params, artifacts []string) {
	if c == nil {
		return
	}
	c.sess.endCall(c.i, c.started, err, params, artifacts)
}

// fileExists is a small helper for tests and handlers.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
