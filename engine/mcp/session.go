package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"modelcheck/ir"
	"modelcheck/report"
)

// MCPSchema identifies the shape of the tool inputs and outputs of this
// package. It changes when a tool's input or output changes incompatibly.
const MCPSchema = "mcd-mcp/1"

// Sessions owns the base directory and the live sessions. One session is
// one directory <base>/<id>; the id is what every tool answer carries.
type Sessions struct {
	base    string
	cleanup bool

	mu      sync.Mutex
	byID    map[string]*Session
	order   []string // creation order, for deterministic listing
	counter int
	params  ServerParams
}

// ServerParams is the server configuration a manifest records (NFR-002).
type ServerParams struct {
	DefaultBudget Budget   `json:"default_budget"`
	Ceiling       Budget   `json:"ceiling"`
	Concurrency   int      `json:"concurrency"`
	AllowRead     []string `json:"allow_read"`
}

// NewSessions creates (if needed) the base directory. base "" means a fresh
// temporary directory.
func NewSessions(base string, cleanup bool, params ServerParams) (*Sessions, error) {
	if base == "" {
		d, err := os.MkdirTemp("", "mcd-sessions-")
		if err != nil {
			return nil, err
		}
		base = d
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	if params.AllowRead == nil {
		params.AllowRead = []string{}
	}
	return &Sessions{base: abs, cleanup: cleanup, byID: map[string]*Session{}, params: params}, nil
}

// Base is the absolute base directory.
func (ss *Sessions) Base() string { return ss.base }

// New creates a session directory. Ids are time-stamped and counted so
// that two servers sharing a base directory cannot collide silently: the
// directory is created with Mkdir, which fails if it exists, and the counter
// advances until it succeeds.
func (ss *Sessions) New() (*Session, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := time.Now().UTC()
	for attempt := 0; attempt < 10000; attempt++ {
		ss.counter++
		id := fmt.Sprintf("s%s-%04d", now.Format("20060102-150405"), ss.counter)
		dir := filepath.Join(ss.base, id)
		if err := os.Mkdir(dir, 0o755); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, err
		}
		s := &Session{ID: id, Dir: dir}
		s.manifest = Manifest{
			SessionID: id,
			Created:   now.Format(time.RFC3339),
			Engine: EngineInfo{
				Name: report.EngineName, Version: report.EngineVersion,
				IRSchema: ir.Schema, ReportSchema: report.ReportSchema, MCPSchema: MCPSchema,
			},
			Server: ss.params,
			Inputs: []ManifestInput{},
			Calls:  []Call{},
		}
		ss.byID[id] = s
		ss.order = append(ss.order, id)
		return s, nil
	}
	return nil, errors.New("could not allocate a session directory")
}

// Get returns the session with id, or an error naming it as unknown.
func (ss *Sessions) Get(id string) (*Session, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, ok := ss.byID[id]
	if !ok {
		return nil, fmt.Errorf("unknown session %q", id)
	}
	return s, nil
}

// Close removes the session directories if cleanup was requested at start.
func (ss *Sessions) Close() error {
	if !ss.cleanup {
		return nil
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	var first error
	for _, id := range ss.order {
		if err := os.RemoveAll(filepath.Join(ss.base, id)); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Session is one client session: a directory, a manifest and the parsed
// model of the session when there is one.
type Session struct {
	ID  string
	Dir string

	mu       sync.Mutex
	manifest Manifest
	seq      map[string]int
	// model is the IR most recently produced by mc_parse in this session; the
	// other tools use it when the call carries no IR of its own.
	model      *ir.Model
	modelBytes []byte // canonical JSON of model (ir.MarshalJSON)
	modelInput ManifestInput
	// defines is the #define table of a Promela model parsed in this
	// session, for the atoms of ltl formulas (nil otherwise).
	defines map[string]string
	// cexs maps counterexample ids to their metadata.
	cexs []cexEntry
}

type cexEntry struct {
	ID       string
	Property string
	Role     string // counterexample | witness
	Path     string // relative to Dir
}

// Path resolves rel inside the session directory through the guard.
func (s *Session) Path(rel string) (string, error) {
	p, err := Resolve(s.Dir, rel)
	if err != nil {
		return "", fmt.Errorf("%v (session directory %s)", err, s.Dir)
	}
	return p, nil
}

// WriteFile is the single file writer of the server: every file the tools
// produce (IR, reports, traces, manifest) goes through it, and it goes
// through Resolve. The only directories created elsewhere are the base
// directory (NewSessions) and the session directory itself (New), both at
// server-chosen locations. It returns the absolute path written.
func (s *Session) WriteFile(rel string, data []byte) (string, error) {
	p, err := s.Path(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// ReadFile reads a session file through the same guard.
func (s *Session) ReadFile(rel string) ([]byte, error) {
	p, err := s.Path(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// next returns "<prefix>-<n><ext>", counting per prefix within the session.
func (s *Session) next(prefix, ext string) string {
	if s.seq == nil {
		s.seq = map[string]int{}
	}
	s.seq[prefix]++
	return fmt.Sprintf("%s-%d%s", prefix, s.seq[prefix], ext)
}

// --- manifest -----------------------------------------------------------------

// Manifest is the reproducibility record of a session (FR-012, NFR-002):
// versions, input hashes, server parameters, and every tool call with its
// parameters, seed, timing and artefacts. It is written to manifest.json
// after every call and returned by mc_manifest.
type Manifest struct {
	SessionID string          `json:"session_id"`
	Created   string          `json:"created" jsonschema:"RFC 3339 UTC time the session was created"`
	Engine    EngineInfo      `json:"engine"`
	Server    ServerParams    `json:"server" jsonschema:"budget defaults, ceilings, concurrency and read allow-list the server was started with"`
	Inputs    []ManifestInput `json:"inputs"`
	Calls     []Call          `json:"calls"`
}

type EngineInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	IRSchema     string `json:"ir_schema"`
	ReportSchema string `json:"report_schema"`
	MCPSchema    string `json:"mcp_schema"`
}

// ManifestInput is one model input of the session.
type ManifestInput struct {
	Kind   string `json:"kind" jsonschema:"promela | petri | ir"`
	Source string `json:"source" jsonschema:"inline, or the client-named file that was read"`
	SHA256 string `json:"sha256" jsonschema:"sha256 of the input: the file's bytes for a file, the source text for promela, the canonical JSON (sorted keys) of an inline object for petri/ir"`
	Path   string `json:"path,omitempty" jsonschema:"session file holding the canonical IR produced from it"`
}

// Call is one tool call of the session.
type Call struct {
	N          int      `json:"n"`
	Tool       string   `json:"tool"`
	Started    string   `json:"started" jsonschema:"RFC 3339 UTC"`
	DurationMS int64    `json:"duration_ms" jsonschema:"for the mc_manifest call that renders the manifest, the time until rendering"`
	Outcome    string   `json:"outcome" jsonschema:"ok | error; error means a tool failure (isError), never a verification status"`
	Error      string   `json:"error,omitempty"`
	Params     *Params  `json:"params,omitempty"`
	Artifacts  []string `json:"artifacts" jsonschema:"session files produced by the call"`
}

// Params records what a call was given after server policy was applied.
type Params struct {
	Search        string  `json:"search,omitempty"`
	Fairness      string  `json:"fairness,omitempty"`
	POR           bool    `json:"por,omitempty"`
	BudgetApplied *Budget `json:"budget_applied,omitempty"`
	Seed          *int64  `json:"seed,omitempty"`
	Steps         int     `json:"steps,omitempty"`
	Mode          string  `json:"mode,omitempty"`
	TimeLimitMS   int64   `json:"time_limit_ms,omitempty"`
}

// beginCall appends a call entry and returns its index.
func (s *Session) beginCall(tool string, started time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.manifest.Calls) + 1
	s.manifest.Calls = append(s.manifest.Calls, Call{N: n, Tool: tool, Started: started.UTC().Format(time.RFC3339), Outcome: "ok", Artifacts: []string{}})
	return n - 1
}

// endCall finalises the entry and rewrites manifest.json.
func (s *Session) endCall(i int, started time.Time, err error, params *Params, artifacts []string) {
	s.mu.Lock()
	c := &s.manifest.Calls[i]
	c.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		c.Outcome = "error"
		c.Error = err.Error()
	}
	c.Params = params
	if artifacts != nil {
		c.Artifacts = append(c.Artifacts, artifacts...)
	}
	data, _ := s.manifestJSONLocked()
	s.mu.Unlock()
	s.WriteFile("manifest.json", data)
}

func (s *Session) manifestJSONLocked() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	err := enc.Encode(s.manifest)
	return buf.Bytes(), err
}

// snapshot returns a copy of the manifest (for mc_manifest) and its JSON.
func (s *Session) snapshot() (Manifest, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.manifest
	m.Inputs = append([]ManifestInput{}, s.manifest.Inputs...)
	m.Calls = append([]Call{}, s.manifest.Calls...)
	data, _ := s.manifestJSONLocked()
	return m, data
}

func (s *Session) addInput(in ManifestInput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest.Inputs = append(s.manifest.Inputs, in)
}
