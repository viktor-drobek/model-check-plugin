// Package rex implements a small RE2-like regular-expression engine without
// depending on a ready-made regexp implementation.
package rex

import (
	"fmt"
	"unicode/utf8"
)

type nodeKind uint8

const (
	nEmpty nodeKind = iota
	nLiteral
	nAny
	nClass
	nConcat
	nAlt
	nStar
	nPlus
	nQuest
	nGroup
	nBOL
	nEOL
)

type node struct {
	kind  nodeKind
	rune  rune
	class charClass
	left  *node
	right *node
	group int
}

type charClass struct {
	neg    bool
	ranges [][2]rune
}

func (c charClass) match(r rune) bool {
	ok := false
	for _, pair := range c.ranges {
		if pair[0] <= r && r <= pair[1] {
			ok = true
			break
		}
	}
	return ok != c.neg
}

type parser struct {
	s      []rune
	pos    int
	groups int
}

func Parse(pattern string) (*node, int, error) {
	p := &parser{s: []rune(pattern)}
	n, err := p.expr()
	if err != nil {
		return nil, 0, err
	}
	if p.pos != len(p.s) {
		return nil, 0, fmt.Errorf("syntax: unexpected %q", p.s[p.pos])
	}
	return n, p.groups, nil
}

func (p *parser) expr() (*node, error) {
	left, err := p.concat()
	if err != nil {
		return nil, err
	}
	for p.peek('|') {
		p.pos++
		right, err := p.concat()
		if err != nil {
			return nil, err
		}
		left = &node{kind: nAlt, left: left, right: right}
	}
	return left, nil
}

func (p *parser) concat() (*node, error) {
	var out *node
	for p.pos < len(p.s) && p.s[p.pos] != ')' && p.s[p.pos] != '|' {
		n, err := p.repeat()
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = n
		} else {
			out = &node{kind: nConcat, left: out, right: n}
		}
	}
	if out == nil {
		return &node{kind: nEmpty}, nil
	}
	return out, nil
}

func (p *parser) repeat() (*node, error) {
	n, err := p.atom()
	if err != nil {
		return nil, err
	}
	if p.pos == len(p.s) {
		return n, nil
	}
	switch p.s[p.pos] {
	case '*':
		p.pos++
		return &node{kind: nStar, left: n}, nil
	case '+':
		p.pos++
		return &node{kind: nPlus, left: n}, nil
	case '?':
		p.pos++
		return &node{kind: nQuest, left: n}, nil
	case '{':
		return nil, fmt.Errorf("unsupported_syntax: bounded repetitions")
	}
	return n, nil
}

func (p *parser) atom() (*node, error) {
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("syntax: unexpected end")
	}
	r := p.s[p.pos]
	p.pos++
	switch r {
	case '(':
		if p.peek('?') {
			return nil, fmt.Errorf("unsupported_syntax: inline construct")
		}
		p.groups++
		g := p.groups
		n, err := p.expr()
		if err != nil {
			return nil, err
		}
		if !p.peek(')') {
			return nil, fmt.Errorf("syntax: unclosed group")
		}
		p.pos++
		return &node{kind: nGroup, left: n, group: g}, nil
	case ')', '|', '*', '+', '?':
		return nil, fmt.Errorf("syntax: unexpected %q", r)
	case '.':
		return &node{kind: nAny}, nil
	case '^':
		return &node{kind: nBOL}, nil
	case '$':
		return &node{kind: nEOL}, nil
	case '[':
		return p.class()
	case '\\':
		return p.escape()
	default:
		return &node{kind: nLiteral, rune: r}, nil
	}
}

func (p *parser) escape() (*node, error) {
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("invalid_escape: trailing slash")
	}
	r := p.s[p.pos]
	p.pos++
	switch r {
	case 'd':
		return &node{kind: nClass, class: charClass{ranges: [][2]rune{{'0', '9'}}}}, nil
	case 'D':
		return &node{kind: nClass, class: charClass{neg: true, ranges: [][2]rune{{'0', '9'}}}}, nil
	case 'w':
		return &node{kind: nClass, class: charClass{ranges: [][2]rune{{'0', '9'}, {'A', 'Z'}, {'a', 'z'}, {'_', '_'}}}}, nil
	case 's':
		return &node{kind: nClass, class: charClass{ranges: [][2]rune{{' ', ' '}, {'\t', '\t'}, {'\n', '\n'}, {'\r', '\r'}}}}, nil
	case 'n':
		return &node{kind: nLiteral, rune: '\n'}, nil
	case 't':
		return &node{kind: nLiteral, rune: '\t'}, nil
	default:
		return &node{kind: nLiteral, rune: r}, nil
	}
}

func (p *parser) class() (*node, error) {
	c := charClass{}
	if p.peek('^') {
		c.neg = true
		p.pos++
	}
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("invalid_class: unclosed class")
	}
	for p.pos < len(p.s) && p.s[p.pos] != ']' {
		a := p.s[p.pos]
		p.pos++
		if a == '\\' {
			if p.pos >= len(p.s) {
				return nil, fmt.Errorf("invalid_escape: class")
			}
			a = p.s[p.pos]
			p.pos++
		}
		b := a
		if p.pos+1 < len(p.s) && p.s[p.pos] == '-' && p.s[p.pos+1] != ']' {
			p.pos++
			b = p.s[p.pos]
			p.pos++
			if a > b {
				return nil, fmt.Errorf("invalid_class: reversed range")
			}
		}
		c.ranges = append(c.ranges, [2]rune{a, b})
	}
	if !p.peek(']') {
		return nil, fmt.Errorf("invalid_class: unclosed class")
	}
	p.pos++
	return &node{kind: nClass, class: c}, nil
}
func (p *parser) peek(want rune) bool { return p.pos < len(p.s) && p.s[p.pos] == want }

type op uint8

const (
	iRune op = iota
	iAny
	iClass
	iSplit
	iSave
	iBOL
	iEOL
	iMatch
)

type inst struct {
	op              op
	r               rune
	class           charClass
	out, out1, slot int
}
type Program struct {
	code          []inst
	start, groups int
}
type patch struct{ pc, which int }
type frag struct {
	start int
	out   []patch
}

func (p *Program) emit(i inst) int { p.code = append(p.code, i); return len(p.code) - 1 }
func (p *Program) patch(out []patch, to int) {
	for _, x := range out {
		if x.which == 0 {
			p.code[x.pc].out = to
		} else {
			p.code[x.pc].out1 = to
		}
	}
}
func appendOut(a, b []patch) []patch { return append(append([]patch{}, a...), b...) }
func Compile(pattern string) (*Program, error) {
	n, groups, err := Parse(pattern)
	if err != nil {
		return nil, err
	}
	p := &Program{groups: groups}
	f := p.compile(n)
	m := p.emit(inst{op: iMatch})
	p.patch(f.out, m)
	p.start = f.start
	return p, nil
}
func (p *Program) compile(n *node) frag {
	switch n.kind {
	case nEmpty:
		pc := p.emit(inst{op: iSplit})
		return frag{pc, []patch{{pc, 0}}}
	case nLiteral:
		pc := p.emit(inst{op: iRune, r: n.rune})
		return frag{pc, []patch{{pc, 0}}}
	case nAny:
		pc := p.emit(inst{op: iAny})
		return frag{pc, []patch{{pc, 0}}}
	case nClass:
		pc := p.emit(inst{op: iClass, class: n.class})
		return frag{pc, []patch{{pc, 0}}}
	case nBOL:
		pc := p.emit(inst{op: iBOL})
		return frag{pc, []patch{{pc, 0}}}
	case nEOL:
		pc := p.emit(inst{op: iEOL})
		return frag{pc, []patch{{pc, 0}}}
	case nConcat:
		a, b := p.compile(n.left), p.compile(n.right)
		p.patch(a.out, b.start)
		return frag{a.start, b.out}
	case nAlt:
		a, b := p.compile(n.left), p.compile(n.right)
		pc := p.emit(inst{op: iSplit, out: a.start, out1: b.start})
		return frag{pc, appendOut(a.out, b.out)}
	case nStar:
		a := p.compile(n.left)
		pc := p.emit(inst{op: iSplit, out: a.start})
		p.patch(a.out, pc)
		return frag{pc, []patch{{pc, 1}}}
	case nPlus:
		a := p.compile(n.left)
		pc := p.emit(inst{op: iSplit, out: a.start})
		p.patch(a.out, pc)
		return frag{a.start, []patch{{pc, 1}}}
	case nQuest:
		a := p.compile(n.left)
		pc := p.emit(inst{op: iSplit, out: a.start})
		return frag{pc, appendOut(a.out, []patch{{pc, 1}})}
	case nGroup:
		before := p.emit(inst{op: iSave, slot: 2 * n.group})
		a := p.compile(n.left)
		after := p.emit(inst{op: iSave, slot: 2*n.group + 1})
		p.code[before].out = a.start
		p.patch(a.out, after)
		return frag{before, []patch{{after, 0}}}
	}
	panic("unknown node")
}

type Match struct {
	Matched bool     `json:"matched"`
	Span    [2]int   `json:"span"`
	Groups  [][2]int `json:"groups"`
}
type thread struct {
	pc   int
	caps []int
}

func newCaps(n int) []int {
	c := make([]int, 2*(n+1))
	for i := range c {
		c[i] = -1
	}
	return c
}
func cloneCaps(c []int) []int { out := make([]int, len(c)); copy(out, c); return out }
func (p *Program) add(list *[]thread, seen map[int]bool, pc, pos int, caps []int, text []byte) {
	if pc < 0 || pc >= len(p.code) || seen[pc] {
		return
	}
	seen[pc] = true
	in := p.code[pc]
	switch in.op {
	case iSplit:
		p.add(list, seen, in.out, pos, caps, text)
		p.add(list, seen, in.out1, pos, caps, text)
	case iSave:
		c := cloneCaps(caps)
		c[in.slot] = pos
		p.add(list, seen, in.out, pos, c, text)
	case iBOL:
		if pos == 0 {
			p.add(list, seen, in.out, pos, caps, text)
		}
	case iEOL:
		if pos == len(text) {
			p.add(list, seen, in.out, pos, caps, text)
		}
	default:
		*list = append(*list, thread{pc, caps})
	}
}

// GroupCount reports the number of explicit capturing groups.
func (p *Program) GroupCount() int { return p.groups }

func (p *Program) Find(text []byte) Match {
	for start := 0; start <= len(text); {
		caps := newCaps(p.groups)
		caps[0] = start
		curr := []thread{}
		p.add(&curr, map[int]bool{}, p.start, start, caps, text)
		for pos := start; ; {
			if pos >= len(text) {
				for _, t := range curr {
					if p.code[t.pc].op == iMatch {
						c := cloneCaps(t.caps)
						c[1] = pos
						return Match{true, [2]int{start, pos}, pairs(c, p.groups)}
					}
				}
				break
			}
			r, w := utf8.DecodeRune(text[pos:])
			if r == utf8.RuneError && w == 0 {
				break
			}
			next := []thread{}
			higherCanAdvance := false
			for _, t := range curr {
				in := p.code[t.pc]
				if in.op == iMatch {
					if !higherCanAdvance {
						c := cloneCaps(t.caps)
						c[1] = pos
						return Match{true, [2]int{start, pos}, pairs(c, p.groups)}
					}
					continue
				}
				ok := in.op == iAny || (in.op == iRune && in.r == r) || (in.op == iClass && in.class.match(r))
				if ok {
					higherCanAdvance = true
					p.add(&next, map[int]bool{}, in.out, pos+w, t.caps, text)
				}
			}
			curr = next
			if len(curr) == 0 {
				break
			}
			pos += w
		}
		if start == len(text) {
			break
		}
		_, w := utf8.DecodeRune(text[start:])
		start += w
	}
	return Match{Matched: false, Span: [2]int{-1, -1}, Groups: emptyGroups(p.groups)}
}
func pairs(c []int, groups int) [][2]int {
	out := make([][2]int, groups+1)
	out[0] = [2]int{c[0], c[1]}
	for i := 1; i <= groups; i++ {
		out[i] = [2]int{c[2*i], c[2*i+1]}
	}
	return out
}
func emptyGroups(groups int) [][2]int {
	out := make([][2]int, groups+1)
	for i := range out {
		out[i] = [2]int{-1, -1}
	}
	return out
}
func (p *Program) FindAll(text []byte, limit int) []Match {
	if limit <= 0 {
		limit = 10000
	}
	var out []Match
	at := 0
	for len(out) < limit && at <= len(text) {
		m := p.Find(text[at:])
		if !m.Matched {
			break
		}
		for i := range m.Groups {
			if m.Groups[i][0] >= 0 {
				m.Groups[i][0] += at
				m.Groups[i][1] += at
			}
		}
		m.Span[0] += at
		m.Span[1] += at
		out = append(out, m)
		if m.Span[0] == m.Span[1] {
			if m.Span[1] >= len(text) {
				break
			}
			_, w := utf8.DecodeRune(text[m.Span[1]:])
			at = m.Span[1] + w
		} else {
			at = m.Span[1]
		}
	}
	return out
}
