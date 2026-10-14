// Command rex exposes the synthetic engine through a line-oriented JSON protocol.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"syntheticre2/internal/rex"
)

type request struct {
	Op         string `json:"op"`
	Pattern    string `json:"pattern,omitempty"`
	PatternB64 string `json:"pattern_b64,omitempty"`
	Text       string `json:"text,omitempty"`
	TextB64    string `json:"text_b64,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type response struct {
	Schema  string      `json:"schema"`
	OK      bool        `json:"ok"`
	Error   *errBody    `json:"error,omitempty"`
	Match   *rex.Match  `json:"match,omitempty"`
	Matches []rex.Match `json:"matches,omitempty"`
	Groups  int         `json:"group_count,omitempty"`
}
type errBody struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func decode(plain, b64 string) ([]byte, error) {
	if b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	return []byte(plain), nil
}
func one(r request) response {
	pat, err := decode(r.Pattern, r.PatternB64)
	if err != nil {
		return response{Schema: "rex/v1", Error: &errBody{"syntax", err.Error()}}
	}
	p, err := rex.Compile(string(pat))
	if err != nil {
		return response{Schema: "rex/v1", Error: &errBody{"syntax", err.Error()}}
	}
	if r.Op == "compile" {
		return response{Schema: "rex/v1", OK: true, Groups: p.GroupCount()}
	}
	text, err := decode(r.Text, r.TextB64)
	if err != nil {
		return response{Schema: "rex/v1", Error: &errBody{"syntax", err.Error()}}
	}
	switch r.Op {
	case "find", "match":
		m := p.Find(text)
		return response{Schema: "rex/v1", OK: true, Match: &m}
	case "find_all":
		return response{Schema: "rex/v1", OK: true, Matches: p.FindAll(text, r.Limit)}
	default:
		return response{Schema: "rex/v1", Error: &errBody{"syntax", "unknown op"}}
	}
}
func main() { run(os.Stdin, os.Stdout) }
func run(in io.Reader, out io.Writer) {
	s := bufio.NewScanner(in)
	s.Buffer(make([]byte, 1024), 20<<20)
	e := json.NewEncoder(out)
	for s.Scan() {
		var r request
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			_ = e.Encode(response{Schema: "rex/v1", Error: &errBody{"syntax", err.Error()}})
			continue
		}
		_ = e.Encode(one(r))
	}
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
