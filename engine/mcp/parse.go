package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/frontend/petri"
	"modelcheck/ir"
)

// FileRef names a file the client wants read instead of inline input.
type FileRef struct {
	Kind string `json:"kind" jsonschema:"promela | petri | ir"`
	Path string `json:"path" jsonschema:"path of the file; must lie under a directory given by --allow-read"`
}

// ParseIn is the input of mc_parse. Exactly one of promela, petri, ir, file.
type ParseIn struct {
	SessionID string            `json:"session_id,omitempty" jsonschema:"existing session to parse into; omitted = a new session is created"`
	Promela   string            `json:"promela,omitempty" jsonschema:"Promela source text (subset of plan 14 §5.2)"`
	Petri     map[string]any    `json:"petri,omitempty" jsonschema:"P/T net as JSON (frontend/petri/schema.json)"`
	IR        any               `json:"ir,omitempty" jsonschema:"IR JSON (mcd-ir/1); validated and re-emitted canonically"`
	Defines   map[string]string `json:"defines,omitempty" jsonschema:"preprocessor defines for Promela (name = value)"`
	File      *FileRef          `json:"file,omitempty" jsonschema:"read this file instead of an inline input; allowed only under an --allow-read prefix"`
}

// OriginEntry is one row of the mapping table: an IR element and the user's
// file, line and name for it.
type OriginEntry struct {
	Element string `json:"element" jsonschema:"path of the IR element, e.g. globals[0] or processes[0].edges[3]"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Name    string `json:"name,omitempty" jsonschema:"the user's own name for the element"`
}

// ParseOut is the answer of mc_parse.
type ParseOut struct {
	SessionID string        `json:"session_id"`
	Outcome   string        `json:"outcome" jsonschema:"ir | rejected | not-executed — a parse outcome (exactly one of the three), not a verification status"`
	IR        any           `json:"ir,omitempty" jsonschema:"the IR (mcd-ir/1), present when outcome is ir"`
	IRPath    string        `json:"ir_path,omitempty" jsonschema:"session file holding the canonical IR"`
	Origins   []OriginEntry `json:"origins,omitempty" jsonschema:"mapping table IR element → user file/line/name"`
	Warnings  []string      `json:"warnings"`
	Rejection *Rejection    `json:"rejection,omitempty" jsonschema:"present when outcome is rejected"`
	Reason    string        `json:"reason,omitempty" jsonschema:"present when outcome is not-executed: names the missing capability"`
}

func (s *Server) parse(ctx context.Context, req *sdk.CallToolRequest, in ParseIn) (*sdk.CallToolResult, *ParseOut, error) {
	given := 0
	for _, ok := range []bool{in.Promela != "", in.Petri != nil, in.IR != nil, in.File != nil} {
		if ok {
			given++
		}
	}
	if given != 1 {
		return nil, nil, errors.New("exactly one of promela, petri, ir or file must be given")
	}
	// Policy first: a file is read only under an allowed prefix. No session
	// is created for a refused request.
	var src modelSource
	if in.File != nil {
		switch in.File.Kind {
		case "promela", "petri", "ir":
		default:
			return nil, nil, fmt.Errorf("file.kind must be promela, petri or ir, got %q", in.File.Kind)
		}
		real, err := s.allowedRead(in.File.Path)
		if err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(real)
		if err != nil {
			return nil, nil, err
		}
		src = modelSource{kind: in.File.Kind, source: real, data: data}
	}
	sess, err := s.session(in.SessionID, true)
	if err != nil {
		return nil, nil, err
	}
	timer := begin(sess, "mc_parse")
	out := &ParseOut{SessionID: sess.ID, Warnings: []string{}}
	var artifacts []string
	defer func() { timer.end(err, nil, artifacts) }()

	if in.File == nil {
		switch {
		case in.Promela != "":
			src = modelSource{kind: "promela", source: "inline", data: []byte(in.Promela)}
		case in.Petri != nil:
			data, e := json.Marshal(in.Petri)
			if e != nil {
				err = e
				return nil, nil, err
			}
			src = modelSource{kind: "petri", source: "inline", data: data}
		default:
			data, e := json.Marshal(in.IR)
			if e != nil {
				err = e
				return nil, nil, err
			}
			src = modelSource{kind: "ir", source: "inline", data: data}
		}
	}

	var m *ir.Model
	var rej *Rejection
	switch src.kind {
	case "promela":
		if s.cfg.Promela == nil {
			out.Outcome = "not-executed"
			out.Reason = "promela frontend not available in this build (engine/frontend/promela, step G1, is not linked into this server); pass the model as petri or ir"
			return nil, out, nil
		}
		m, rej, err = s.cfg.Promela(string(src.data), in.Defines, src.source)
		if err != nil {
			return nil, nil, err
		}
	case "petri":
		m, rej = parsePetri(src)
	default:
		var e error
		m, e = ir.UnmarshalJSON(src.data)
		if e != nil {
			rej = &Rejection{Kind: "ir", Construct: "ir", File: fileOf(src), Reason: e.Error()}
		}
	}
	if rej != nil {
		out.Outcome = "rejected"
		out.Rejection = rej
		return nil, out, nil
	}
	if err = s.adopt(sess, m, src); err != nil {
		return nil, nil, err
	}
	sess.mu.Lock()
	out.IRPath = filepath.Join(sess.Dir, sess.modelInput.Path)
	canon := sess.modelBytes
	sess.mu.Unlock()
	artifacts = []string{filepath.Base(out.IRPath)}
	var asAny any
	if err = json.Unmarshal(canon, &asAny); err != nil {
		return nil, nil, err
	}
	out.Outcome = "ir"
	out.IR = asAny
	out.Origins = origins(m)
	return nil, out, nil
}

func fileOf(src modelSource) string {
	if src.source == "inline" {
		return "inline:" + src.kind
	}
	return src.source
}

// parsePetri runs the Petri frontend and maps its error onto a Rejection.
func parsePetri(src modelSource) (*ir.Model, *Rejection) {
	name := "model"
	if src.source != "inline" {
		name = strings.TrimSuffix(filepath.Base(src.source), filepath.Ext(src.source))
	}
	net, err := petri.Parse(src.data, name)
	if err != nil {
		var pe *petri.Error
		if errors.As(err, &pe) {
			construct := pe.Path
			if pe.Kind == "unsupported-input" {
				if i := strings.Index(pe.Message, ":"); i > 0 {
					construct = pe.Message[:i]
				}
			}
			return nil, &Rejection{Kind: pe.Kind, Construct: construct, File: fileOf(src), Reason: pe.Message}
		}
		return nil, &Rejection{Kind: "schema", Construct: "document", File: fileOf(src), Reason: err.Error()}
	}
	return net.ToIR(fileOf(src)), nil
}

// origins flattens the Origin fields of m into the mapping table, in IR
// order.
func origins(m *ir.Model) []OriginEntry {
	var out []OriginEntry
	add := func(el string, o *ir.Origin) {
		if o != nil {
			out = append(out, OriginEntry{Element: el, File: o.File, Line: o.Line, Name: o.Name})
		}
	}
	add("model", m.Origin)
	for i := range m.Globals {
		add(fmt.Sprintf("globals[%d]", i), m.Globals[i].Origin)
	}
	for i := range m.Channels {
		add(fmt.Sprintf("channels[%d]", i), m.Channels[i].Origin)
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		add(fmt.Sprintf("processes[%d]", p), pr.Origin)
		for i := range pr.Locals {
			add(fmt.Sprintf("processes[%d].locals[%d]", p, i), pr.Locals[i].Origin)
		}
		for i := range pr.Locations {
			add(fmt.Sprintf("processes[%d].locations[%d]", p, i), pr.Locations[i].Origin)
		}
		for i := range pr.Edges {
			add(fmt.Sprintf("processes[%d].edges[%d]", p, i), pr.Edges[i].Origin)
		}
	}
	for i := range m.Properties {
		add(fmt.Sprintf("properties[%d]", i), m.Properties[i].Origin)
	}
	if out == nil {
		out = []OriginEntry{}
	}
	return out
}
