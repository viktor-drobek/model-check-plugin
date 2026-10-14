// Command re2loggen writes deterministic RE2-compatible JSONL fixtures.
// It is oracle tooling; production packages never import regexp.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"syntheticre2/internal/rex"
)

type req struct {
	Op      string `json:"op"`
	Pattern string `json:"pattern"`
	Text    string `json:"text"`
	Limit   int    `json:"limit,omitempty"`
}
type out struct {
	Schema  string      `json:"schema"`
	OK      bool        `json:"ok"`
	Match   *rex.Match  `json:"match,omitempty"`
	Matches []rex.Match `json:"matches,omitempty"`
	Groups  int         `json:"group_count,omitempty"`
	Error   *struct {
		Class   string `json:"class"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
type rec struct {
	Meta     map[string]any `json:"meta,omitempty"`
	Request  req            `json:"request,omitempty"`
	Expected out            `json:"expected,omitempty"`
}

func oracle(q req) out {
	r, e := regexp.Compile(q.Pattern)
	if e != nil {
		return out{Schema: "rex/v1", Error: &struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		}{"syntax", e.Error()}}
	}
	if q.Op == "compile" {
		return out{Schema: "rex/v1", OK: true, Groups: r.NumSubexp()}
	}
	one := func(ix []int) rex.Match {
		if ix == nil {
			return rex.Match{Matched: false, Span: [2]int{-1, -1}, Groups: empty(r.NumSubexp())}
		}
		g := make([][2]int, r.NumSubexp()+1)
		for i := range g {
			g[i] = [2]int{ix[2*i], ix[2*i+1]}
		}
		return rex.Match{Matched: true, Span: g[0], Groups: g}
	}
	if q.Op == "find_all" {
		var a []rex.Match
		for _, ix := range r.FindAllStringSubmatchIndex(q.Text, q.Limit) {
			a = append(a, one(ix))
		}
		return out{Schema: "rex/v1", OK: true, Matches: a}
	}
	m := one(r.FindStringSubmatchIndex(q.Text))
	return out{Schema: "rex/v1", OK: true, Match: &m}
}
func empty(n int) [][2]int {
	a := make([][2]int, n+1)
	for i := range a {
		a[i] = [2]int{-1, -1}
	}
	return a
}
func main() {
	outPath := flag.String("out", "testdata/re2logs/basic.jsonl", "output")
	flag.Parse()
	cases := []req{{"find", "a|bc", "xxbc", 0}, {"find", "(a+)(b)", "aaab", 0}, {"find", "ab*c", "ac", 0}, {"find", "[a-c]+", "xxabc", 0}, {"find", "^a.$", "ab", 0}, {"find", "\\d+", "x42", 0}, {"find_all", "a*", "bb", 10}, {"find", "z+", "abc", 0}}
	f, e := os.Create(*outPath)
	if e != nil {
		panic(e)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	b, _ := json.Marshal(rec{Meta: map[string]any{"schema": "re2log/v1", "oracle": "go-regexp RE2-compatible fixture generator", "semantic_mode": "leftmost_first", "note": "C++ RE2 is installed for external oracle use; these initial frozen cases use Go's RE2-derived oracle subset."}})
	fmt.Fprintln(w, string(b))
	for _, q := range cases {
		b, _ = json.Marshal(rec{Request: q, Expected: oracle(q)})
		fmt.Fprintln(w, string(b))
	}
}
