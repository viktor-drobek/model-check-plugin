package promela

import (
	"fmt"
	"strings"

	"modelcheck/ir"
)

// Lowering: AST → ir.Model, one IR edge per Promela statement, encoded so
// that the explorer stores the same states as pan (SPIN 6.5.2,
// `spin -a -o1 -o2 -o3`, `-DNOREDUCE`) — a correspondence checked model by
// model, not proved: each decision below was fixed by a probe against pan
// and the whole is checked by tools/pandiff on the chapter 2–3 corpus
// (steps/g1-confirmation.md):
//
//   - Control locations are the points *between* statements; every
//     statement is one edge. `if`/`do` have no edge of their own: the
//     first statement of each option leaves the if/do node. `goto` and
//     `break` after a statement are folded into that statement's target
//     (node aliasing); a lone `goto`/`break` option is its own edge, as in
//     pan ("goto L", "goto :b0").
//   - Declarations are not steps; a local initialiser is applied when the
//     process is created (`byte me = _pid + 1` sets Init). `printf` is a
//     no-op step (pan stores the state after it).
//   - `atomic`: an edge whose target is inside the block keeps exclusive
//     control (Edge.Atomic); the block's last edge does not. `d_step`: the
//     same with Edge.DStep, so the explorer runs the block as one step.
//   - Process termination is pan's `-end-` transition: the body's exit
//     location carries the `end` label (a valid end state) and one edge
//     `-end-` leads to a dead location; it is enabled only when no
//     younger process is alive (pan removes only the last process of the
//     vector) and it zeroes the locals (the dead process leaves the
//     vector). Younger = higher pid; alive = not dormant and not dead.
//   - pids: `active` instances and `init` in textual order, then processes
//     created by `run` in the textual order of the run statements of init
//     (the parser admits run only as a straight-line statement of init,
//     so this order is the execution order). Instance names are
//     `<proctype>:<pid>`. `_pid` is folded to the instance's pid.
//   - mtype values: within one declaration the last name is 1, counting
//     up backwards; a later declaration continues above the earlier one.
//   - A never claim becomes a process with Claim = true and no `-end-`;
//     the explorer stores it and does not run it (G4).

type node struct {
	atomic []int // enclosing atomic block ids (innermost last)
	dstep  []int // enclosing d_step block ids
	labels []ir.Label
	name   string
	origin *ir.Origin
	rep    int // alias representative (self if not aliased)
}

type instance struct {
	pt         *Proctype
	pid        int
	name       string
	runCreated bool
	claim      bool

	nodes                             []node
	labels                            map[string]int
	edges                             []ir.Edge // From/To are raw node ids until finalise
	panLines                          []int     // parallel to edges
	finalPanLines                     []int     // parallel to proc.Edges
	edgeMap                           []int     // raw edge index → final index (-1: pruned)
	locals                            []ir.Var
	scopes                            []map[string]bool // block scopes: names visible
	declared                          map[string]bool   // all local names of the process
	entry, endNode, deadNode, dormant int
	final                             map[int]int // raw representative → location index
	proc                              ir.Process
	runs                              []pendingRun
	endEdge                           int // index of the -end- edge in edges, -1 for claims
}

type pendingRun struct {
	edge   int
	target *instance
}

type lowerer struct {
	mod       *Module
	file      string
	mtype     map[string]int64
	globals   map[string]*ir.Var
	chans     map[string]*ir.Channel
	chanCap   map[string]int
	insts     []*instance
	byRun     map[*RunStmt]*instance
	warnings  []string
	printfs   int
	cur       *instance
	curAtomic []int
	curDStep  []int
	blockID   int
	// current block context
}

type seqCtx struct {
	breakTo  int  // node of the enclosing do's exit, -1 if none
	isOption bool // the sequence is an if/do option (lone goto/break rule)
	// optLine is the source line of the first statement of the first option
	// of the enclosing if/do: pan attributes the first statement of every
	// option to that line (see Lowered.PanLines).
	optLine int
}

// Lowered is the result of Lower.
type Lowered struct {
	Model    *ir.Model
	Warnings []string
	// PanLines gives, per process instance name, the source line pan -d
	// would print for each IR edge, in edge order. It differs from
	// Edge.Origin.Line only for the first statement of an if/do option,
	// which pan attributes to the line of the first option. It exists for
	// the statement-table comparison of tools/pandiff; counterexamples use
	// the true lines.
	PanLines map[string][]int
}

// Lower translates the module.
func Lower(mod *Module, file, name string) (*Lowered, *Error) {
	m, w, pl, err := lower(mod, file, name)
	if err != nil {
		return nil, err
	}
	return &Lowered{Model: m, Warnings: w, PanLines: pl}, nil
}

func lower(mod *Module, file, name string) (*ir.Model, []string, map[string][]int, *Error) {
	l := &lowerer{mod: mod, file: file, mtype: map[string]int64{}, globals: map[string]*ir.Var{},
		chans: map[string]*ir.Channel{}, chanCap: map[string]int{}, byRun: map[*RunStmt]*instance{}}
	m := &ir.Model{Schema: ir.Schema, Name: name, Origin: &ir.Origin{File: file, Name: name}}

	// mtype values
	base := int64(0)
	for _, decl := range mod.Mtypes {
		n := int64(len(decl))
		for i, nm := range decl {
			if _, dup := l.mtype[nm]; dup {
				return nil, nil, nil, semanticErr(file, 0, 0, "mtype %s declared twice", nm)
			}
			l.mtype[nm] = base + n - int64(i)
		}
		base += n
	}
	if base > 255 {
		return nil, nil, nil, semanticErr(file, 0, 0, "%d mtype values exceed 255", base)
	}

	// globals and channels
	for _, d := range mod.Globals {
		if d.Type == "chan" {
			if _, dup := l.chans[d.Name]; dup || l.globals[d.Name] != nil || l.mtype[d.Name] != 0 {
				return nil, nil, nil, semanticErr(file, d.Pos.Line, d.Pos.Col, "%s declared twice", d.Name)
			}
			ch := ir.Channel{Name: d.Name, Capacity: d.Chan.Cap, Origin: &ir.Origin{File: file, Line: d.Pos.Line, Name: d.Name}}
			for _, f := range d.Chan.Fields {
				ch.Fields = append(ch.Fields, irType(f))
			}
			m.Channels = append(m.Channels, ch)
			l.chans[d.Name] = &m.Channels[len(m.Channels)-1]
			l.chanCap[d.Name] = d.Chan.Cap
			continue
		}
		v, err := l.varDecl(d, -1)
		if err != nil {
			return nil, nil, nil, err
		}
		if l.globals[v.Name] != nil || l.mtype[v.Name] != 0 || l.chans[v.Name] != nil {
			return nil, nil, nil, semanticErr(file, d.Pos.Line, d.Pos.Col, "%s declared twice", d.Name)
		}
		m.Globals = append(m.Globals, *v)
	}
	// re-point channel pointers after appends
	for i := range m.Channels {
		l.chans[m.Channels[i].Name] = &m.Channels[i]
	}
	for i := range m.Globals {
		l.globals[m.Globals[i].Name] = &m.Globals[i]
	}

	// pass 1: instances in pid order
	byName := map[string]*Proctype{}
	for _, pt := range mod.Procs {
		byName[pt.Name] = pt
	}
	var initPT *Proctype
	for _, pt := range mod.Procs {
		for k := 0; k < pt.Active; k++ {
			l.insts = append(l.insts, &instance{pt: pt, pid: len(l.insts)})
		}
		if pt.IsInit {
			initPT = pt
		}
	}
	if initPT != nil {
		var runs []*RunStmt
		collectRuns(initPT.Body.Items, &runs)
		for _, r := range runs {
			pt := byName[r.Proc]
			if pt == nil || pt.IsInit {
				return nil, nil, nil, semanticErr(file, r.Pos.Line, r.Pos.Col, "run of undeclared proctype %s", r.Proc)
			}
			if len(r.Args) != len(pt.Params) {
				return nil, nil, nil, semanticErr(file, r.Pos.Line, r.Pos.Col, "run %s: %d argument(s) for %d parameter(s)", r.Proc, len(r.Args), len(pt.Params))
			}
			in := &instance{pt: pt, pid: len(l.insts), runCreated: true}
			l.insts = append(l.insts, in)
			l.byRun[r] = in
		}
	}
	if len(l.insts) == 0 {
		return nil, nil, nil, semanticErr(file, 0, 0, "the model has no active process and no init")
	}
	if mod.Never != nil {
		l.insts = append(l.insts, &instance{pt: mod.Never, pid: len(l.insts), claim: true})
		l.warnings = append(l.warnings, fmt.Sprintf("never claim (line %d) parsed and stored as a claim process; not executed in this engine version — safety properties only; the claim's product with the system is G4", mod.Never.Pos.Line))
	}
	for _, in := range l.insts {
		in.name = fmt.Sprintf("%s:%d", in.pt.Name, in.pid)
		if in.claim {
			in.name = "never"
		}
	}

	// pass 2: bodies
	for _, in := range l.insts {
		if err := l.lowerInstance(in); err != nil {
			return nil, nil, nil, err
		}
	}
	// finalise numbering, then cross-process references
	for _, in := range l.insts {
		l.finalise(in)
	}
	for k, in := range l.insts {
		for _, pr := range in.runs {
			e := &in.proc.Edges[in.edgeMap[pr.edge]]
			e.Run.Entry = pr.target.final[pr.target.find(pr.target.entry)]
		}
		if in.endEdge >= 0 && in.edgeMap[in.endEdge] >= 0 {
			var conj []*ir.Expr
			for j := k + 1; j < len(l.insts); j++ {
				o := l.insts[j]
				if o.claim {
					continue
				}
				// j is "not alive" when dead or (run-created) dormant. A
				// process whose end is unreachable never dies: constant false.
				var dead *ir.Expr
				if d, ok := o.final[o.find(o.deadNode)]; ok {
					dead = ir.Binary("eq", ir.PC(j), ir.Const(int64(d)))
				} else {
					dead = ir.Const(0)
				}
				if o.runCreated {
					dead = ir.Binary("or", dead, ir.Binary("eq", ir.PC(j), ir.Const(int64(o.final[o.find(o.dormant)]))))
				}
				conj = append(conj, dead)
			}
			if len(conj) > 0 {
				in.proc.Edges[in.edgeMap[in.endEdge]].Guard = ir.And(conj...)
			}
		}
		m.Processes = append(m.Processes, in.proc)
	}

	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock,
		Text: "no invalid end state: every state without an enabled transition has all processes terminated (SPIN: invalid end states)"}}
	hasAssert := false
	for _, in := range l.insts {
		if in.claim {
			continue
		}
		for _, e := range in.proc.Edges {
			if e.Assert != nil {
				hasAssert = true
			}
		}
	}
	if hasAssert {
		m.Properties = append(m.Properties, ir.Property{ID: "assert", Kind: ir.KindAssert, Text: "no assert statement fails (SPIN: assertion violations)"})
	}
	if l.printfs > 0 {
		l.warnings = append(l.warnings, fmt.Sprintf("printf: %d statement(s) kept as no-op steps so that state counts match SPIN; their output is not produced", l.printfs))
	}
	if err := ir.Validate(m); err != nil {
		return nil, nil, nil, semanticErr(file, 0, 0, "internal: lowered IR does not validate: %v", err)
	}
	panLines := map[string][]int{}
	for _, in := range l.insts {
		panLines[in.name] = in.finalPanLines
	}
	return m, l.warnings, panLines, nil
}

func collectRuns(items []Item, out *[]*RunStmt) {
	for _, it := range items {
		switch s := it.Stmt.(type) {
		case *RunStmt:
			*out = append(*out, s)
		case *Block:
			collectRuns(s.Items, out)
		}
	}
}

func irType(t string) ir.Type {
	switch t {
	case "bit":
		return ir.Bit
	case "bool":
		return ir.Bool
	case "short":
		return ir.Short
	case "int":
		return ir.Int
	}
	return ir.Byte // byte, mtype, pid
}

// varDecl builds an ir.Var; pid >= 0 for locals (for _pid folding).
func (l *lowerer) varDecl(d *VarDecl, pid int) (*ir.Var, *Error) {
	v := &ir.Var{Name: d.Name, Type: irType(d.Type), Len: d.Len, Origin: &ir.Origin{File: l.file, Line: d.Pos.Line, Name: d.Name}}
	if d.Init != nil {
		e, err := l.expr(d.Init, pid, true)
		if err != nil {
			return nil, err
		}
		val, ok := constValue(e)
		if !ok {
			return nil, outside(l.file, d.Pos.Line, d.Pos.Col, "non-constant initialiser", fmt.Sprintf("%s is initialised with an expression over variables; only constants and _pid are accepted", d.Name))
		}
		min, max := v.Domain()
		if val < min || val > max {
			return nil, semanticErr(l.file, d.Pos.Line, d.Pos.Col, "initial value %d of %s outside the %s domain [%d, %d]", val, d.Name, d.Type, min, max)
		}
		if d.Len > 0 {
			// SPIN initialises every element with the scalar initialiser.
			for i := 0; i < d.Len; i++ {
				v.Init = append(v.Init, val)
			}
		} else {
			v.Init = []int64{val}
		}
	}
	return v, nil
}

// constValue folds an ir expression without variable reads.
func constValue(e *ir.Expr) (int64, bool) {
	if e == nil {
		return 0, false
	}
	switch e.Op {
	case "const":
		return e.Value, true
	case "var", "index", "len", "timeout", "pc":
		return 0, false
	}
	var vals []int64
	for _, a := range e.Args {
		v, ok := constValue(a)
		if !ok {
			return 0, false
		}
		vals = append(vals, v)
	}
	b := func(x bool) int64 {
		if x {
			return 1
		}
		return 0
	}
	switch e.Op {
	case "neg":
		return -vals[0], true
	case "not":
		return b(vals[0] == 0), true
	case "add":
		return vals[0] + vals[1], true
	case "sub":
		return vals[0] - vals[1], true
	case "mul":
		return vals[0] * vals[1], true
	case "div":
		if vals[1] == 0 {
			return 0, false
		}
		return vals[0] / vals[1], true
	case "mod":
		if vals[1] == 0 {
			return 0, false
		}
		return vals[0] % vals[1], true
	case "eq":
		return b(vals[0] == vals[1]), true
	case "ne":
		return b(vals[0] != vals[1]), true
	case "lt":
		return b(vals[0] < vals[1]), true
	case "le":
		return b(vals[0] <= vals[1]), true
	case "gt":
		return b(vals[0] > vals[1]), true
	case "ge":
		return b(vals[0] >= vals[1]), true
	case "and":
		return b(vals[0] != 0 && vals[1] != 0), true
	case "or":
		return b(vals[0] != 0 || vals[1] != 0), true
	}
	return 0, false
}

// ---- per instance -----------------------------------------------------------------

func (l *lowerer) newNode() int {
	in := l.cur
	n := node{rep: len(in.nodes)}
	n.atomic = append(n.atomic, l.curAtomic...)
	n.dstep = append(n.dstep, l.curDStep...)
	in.nodes = append(in.nodes, n)
	return n.rep
}

func (in *instance) find(n int) int {
	for i := 0; i < len(in.nodes); i++ {
		r := in.nodes[n].rep
		if r == n {
			return n
		}
		n = r
	}
	return n // cycle: caller has checked
}

// alias makes a stand for b: control at a is control at b.
func (l *lowerer) alias(a, b int, at Pos) *Error {
	in := l.cur
	a, b = in.find(a), in.find(b)
	if a == b {
		return nil
	}
	// merge labels and name into the representative
	in.nodes[b].labels = append(in.nodes[b].labels, in.nodes[a].labels...)
	if in.nodes[b].name == "" {
		in.nodes[b].name, in.nodes[b].origin = in.nodes[a].name, in.nodes[a].origin
	}
	in.nodes[a].rep = b
	// cycle check
	seen := map[int]bool{}
	for n := b; ; n = in.nodes[n].rep {
		if in.nodes[n].rep == n {
			break
		}
		if seen[n] {
			return semanticErr(l.file, at.Line, at.Col, "a goto chain that never reaches a statement (empty loop)")
		}
		seen[n] = true
	}
	return nil
}

func (l *lowerer) labelNode(name string) int {
	in := l.cur
	if n, ok := in.labels[name]; ok {
		return n
	}
	n := l.newNode()
	in.labels[name] = n
	return n
}

func (l *lowerer) lowerInstance(in *instance) *Error {
	l.cur = in
	l.curAtomic, l.curDStep = nil, nil
	in.labels = map[string]int{}
	in.declared = map[string]bool{}
	in.final = map[int]int{}
	in.endEdge = -1
	pt := in.pt
	in.proc = ir.Process{Name: in.name, Claim: in.claim, Origin: &ir.Origin{File: l.file, Line: pt.Pos.Line, Name: pt.Name}}
	// parameters
	in.scopes = []map[string]bool{{}}
	for _, d := range pt.Params {
		v, err := l.varDecl(d, in.pid)
		if err != nil {
			return err
		}
		if err := l.declare(v, d.Pos); err != nil {
			return err
		}
	}
	in.proc.Params = len(pt.Params)
	if in.runCreated {
		in.dormant = l.newNode()
		in.nodes[in.dormant].name = "-dormant-"
	}
	in.entry = l.newNode()
	in.endNode = l.newNode()
	if !in.claim {
		in.deadNode = l.newNode()
		in.nodes[in.deadNode].name = "-dead-"
		in.nodes[in.endNode].labels = append(in.nodes[in.endNode].labels, ir.End)
	}
	if err := l.lowerSeq(pt.Body.Items, in.entry, in.endNode, seqCtx{breakTo: -1}); err != nil {
		return err
	}
	if !in.claim {
		e := ir.Edge{From: in.endNode, To: in.deadNode, Text: "-end-",
			Origin: &ir.Origin{File: l.file, Line: pt.End.Line, Name: "-end-"}}
		for _, v := range in.locals {
			if v.Len > 0 {
				for i := 0; i < v.Len; i++ {
					e.Effect = append(e.Effect, ir.Assign{Var: v.Name, Index: ir.Const(int64(i)), Value: ir.Const(0)})
				}
			} else {
				e.Effect = append(e.Effect, ir.Assign{Var: v.Name, Value: ir.Const(0)})
			}
		}
		in.endEdge = len(in.edges)
		in.edges = append(in.edges, e)
		in.panLines = append(in.panLines, pt.End.Line)
	}
	in.proc.Locals = in.locals
	return nil
}

func (l *lowerer) declare(v *ir.Var, at Pos) *Error {
	in := l.cur
	if in.declared[v.Name] {
		return outside(l.file, at.Line, at.Col, "redeclaration of a local variable in another block",
			fmt.Sprintf("%s is declared twice in %s; the engine keeps one flat set of locals per process", v.Name, in.pt.Name))
	}
	if l.chans[v.Name] != nil || l.mtype[v.Name] != 0 {
		return semanticErr(l.file, at.Line, at.Col, "%s is already a channel or mtype constant", v.Name)
	}
	in.declared[v.Name] = true
	in.scopes[len(in.scopes)-1][v.Name] = true
	in.locals = append(in.locals, *v)
	return nil
}

func (in *instance) visible(name string) bool {
	for i := len(in.scopes) - 1; i >= 0; i-- {
		if in.scopes[i][name] {
			return true
		}
	}
	return false
}

func (l *lowerer) lowerSeq(items []Item, from, to int, ctx seqCtx) *Error {
	in := l.cur
	if len(items) == 0 {
		return l.alias(from, to, Pos{})
	}
	pos := make([]int, len(items)+1)
	pos[0] = from
	for i := 1; i < len(items); i++ {
		pos[i] = l.newNode()
	}
	pos[len(items)] = to
	for i, it := range items {
		for _, lb := range it.Labels {
			n := pos[i]
			if existing, ok := in.labels[lb]; ok {
				if in.find(existing) != in.find(n) {
					// allocated by a forward goto: it becomes this position
					r := in.find(existing)
					in.nodes[r].atomic = append([]int(nil), l.curAtomic...)
					in.nodes[r].dstep = append([]int(nil), l.curDStep...)
					if err := l.alias(n, r, it.Pos); err != nil {
						return err
					}
					if i == 0 {
						pos[0] = r
					} else {
						pos[i] = r
					}
					n = r
				}
			} else {
				in.labels[lb] = n
			}
			r := in.find(n)
			if in.nodes[r].name == "" {
				in.nodes[r].name = lb
				in.nodes[r].origin = &ir.Origin{File: l.file, Line: it.Pos.Line, Name: lb}
			}
			switch {
			case strings.HasPrefix(lb, "end"):
				in.nodes[r].labels = append(in.nodes[r].labels, ir.End)
			case strings.HasPrefix(lb, "progress"):
				in.nodes[r].labels = append(in.nodes[r].labels, ir.Progress)
			case strings.HasPrefix(lb, "accept"):
				in.nodes[r].labels = append(in.nodes[r].labels, ir.Accept)
			}
		}
	}
	for i, it := range items {
		if err := l.lowerStmt(it, pos[i], pos[i+1], ctx, i == 0 && ctx.isOption); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) origin(it Item) *ir.Origin {
	return &ir.Origin{File: l.file, Line: it.Pos.Line, Name: l.mod.Text.Render(it.From, it.To)}
}

func (l *lowerer) addEdge(it Item, e ir.Edge, panLine int) {
	e.Origin = l.origin(it)
	e.Text = e.Origin.Name
	l.cur.edges = append(l.cur.edges, e)
	l.cur.panLines = append(l.cur.panLines, panLine)
}

func (l *lowerer) lowerStmt(it Item, from, to int, ctx seqCtx, first bool) *Error {
	in := l.cur
	pid := in.pid
	panLine := it.Pos.Line
	if first && ctx.optLine > 0 {
		panLine = ctx.optLine
	}
	switch s := it.Stmt.(type) {
	case *Block:
		if s.Kind == "decls" {
			for _, d := range s.Items {
				if err := l.lowerStmt(d, from, from, ctx, false); err != nil {
					return err
				}
			}
			return l.alias(from, to, it.Pos)
		}
		in.scopes = append(in.scopes, map[string]bool{})
		saveA, saveD := l.curAtomic, l.curDStep
		switch s.Kind {
		case "atomic":
			l.blockID++
			l.curAtomic = append(append([]int(nil), l.curAtomic...), l.blockID)
		case "d_step":
			l.blockID++
			l.curDStep = append(append([]int(nil), l.curDStep...), l.blockID)
		}
		// A block that opens an option passes the option context on: pan
		// attributes its first statement to the first option's line, and a
		// lone goto/break inside it is its own transition.
		err := l.lowerSeq(s.Items, from, to, seqCtx{breakTo: ctx.breakTo, isOption: first, optLine: ctx.optLine})
		l.curAtomic, l.curDStep = saveA, saveD
		in.scopes = in.scopes[:len(in.scopes)-1]
		return err
	case *Decl:
		v, err := l.varDecl(s.Var, pid)
		if err != nil {
			return err
		}
		if err := l.declare(v, s.Var.Pos); err != nil {
			return err
		}
		return l.alias(from, to, it.Pos)
	case *If:
		for _, opt := range s.Options {
			if err := l.lowerSeq(opt, from, to, seqCtx{breakTo: ctx.breakTo, isOption: true, optLine: s.Options[0][0].Pos.Line}); err != nil {
				return err
			}
		}
		return nil
	case *Do:
		for _, opt := range s.Options {
			if err := l.lowerSeq(opt, from, from, seqCtx{breakTo: to, isOption: true, optLine: s.Options[0][0].Pos.Line}); err != nil {
				return err
			}
		}
		return nil
	case *Goto:
		target := l.labelNode(s.Label)
		if first {
			l.addEdge(it, ir.Edge{From: from, To: target}, panLine)
			return nil
		}
		return l.alias(from, target, it.Pos)
	case *Break:
		if ctx.breakTo < 0 {
			return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "break outside a do loop")
		}
		if first {
			l.addEdge(it, ir.Edge{From: from, To: ctx.breakTo}, panLine)
			return nil
		}
		return l.alias(from, ctx.breakTo, it.Pos)
	case *Else:
		l.addEdge(it, ir.Edge{From: from, To: to, Else: true}, panLine)
		return nil
	case *Assert:
		e, err := l.expr(s.Cond, pid, false)
		if err != nil {
			return err
		}
		l.addEdge(it, ir.Edge{From: from, To: to, Assert: e}, panLine)
		return nil
	case *Printf:
		for _, a := range s.Args {
			if _, err := l.expr(a, pid, false); err != nil {
				return err // names in printf arguments are checked, as SPIN does
			}
		}
		l.printfs++
		l.addEdge(it, ir.Edge{From: from, To: to}, panLine)
		return nil
	case *Assign:
		a, err := l.lvalue(s.Target, pid)
		if err != nil {
			return err
		}
		switch s.Op {
		case "++", "--":
			read := ir.Ref(a.Var)
			if a.Index != nil {
				read = ir.Index(a.Var, a.Index)
			}
			op := "add"
			if s.Op == "--" {
				op = "sub"
			}
			a.Value = ir.Binary(op, read, ir.Const(1))
		default:
			v, err := l.expr(s.Value, pid, false)
			if err != nil {
				return err
			}
			a.Value = v
		}
		l.addEdge(it, ir.Edge{From: from, To: to, Effect: []ir.Assign{a}}, panLine)
		return nil
	case *Send:
		ch, err := l.channel(s.Chan, it.Pos)
		if err != nil {
			return err
		}
		if ch.Capacity == 0 && len(l.curDStep) > 0 {
			return outside(l.file, it.Pos.Line, it.Pos.Col, "rendezvous operation inside d_step", "outside the subset")
		}
		if len(s.Args) != len(ch.Fields) {
			return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "%s!…: %d value(s) for %d field(s)", s.Chan, len(s.Args), len(ch.Fields))
		}
		op := &ir.ChanOp{Chan: s.Chan}
		for _, a := range s.Args {
			e, err := l.expr(a, pid, false)
			if err != nil {
				return err
			}
			op.Args = append(op.Args, e)
		}
		l.addEdge(it, ir.Edge{From: from, To: to, Send: op}, panLine)
		return nil
	case *Recv:
		ch, err := l.channel(s.Chan, it.Pos)
		if err != nil {
			return err
		}
		if ch.Capacity == 0 && len(l.curDStep) > 0 {
			return outside(l.file, it.Pos.Line, it.Pos.Col, "rendezvous operation inside d_step", "outside the subset")
		}
		if len(s.Args) != len(ch.Fields) {
			return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "%s?…: %d argument(s) for %d field(s)", s.Chan, len(s.Args), len(ch.Fields))
		}
		op := &ir.RecvOp{Chan: s.Chan}
		for _, a := range s.Args {
			var ra ir.RecvArg
			switch {
			case a.Ignore:
			case a.Match != nil:
				e, err := l.expr(a.Match, pid, false)
				if err != nil {
					return err
				}
				ra.Match = e
			case a.Var != nil:
				if a.Var.Index == nil {
					if v, ok := l.mtype[a.Var.Name]; ok && !in.visible(a.Var.Name) && l.globals[a.Var.Name] == nil {
						ra.Match = ir.Const(v)
						break
					}
				}
				lv, err := l.lvalue(a.Var, pid)
				if err != nil {
					return err
				}
				ra.Var, ra.Index = lv.Var, lv.Index
			}
			op.Args = append(op.Args, ra)
		}
		l.addEdge(it, ir.Edge{From: from, To: to, Recv: op}, panLine)
		return nil
	case *RunStmt:
		target := l.byRun[s]
		if target == nil {
			return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "internal: run statement without an instance")
		}
		r := &ir.RunOp{Proc: target.pid}
		for _, a := range s.Args {
			e, err := l.expr(a, pid, false)
			if err != nil {
				return err
			}
			r.Args = append(r.Args, e)
		}
		e := ir.Edge{From: from, To: to, Run: r}
		if s.Target != nil {
			a, err := l.lvalue(s.Target, pid)
			if err != nil {
				return err
			}
			a.Value = ir.Const(int64(target.pid))
			e.Effect = []ir.Assign{a}
		}
		in.runs = append(in.runs, pendingRun{edge: len(in.edges), target: target})
		l.addEdge(it, e, panLine)
		return nil
	case *ExprStmt:
		e, err := l.expr(s.X, pid, false)
		if err != nil {
			return err
		}
		if v, ok := constValue(e); ok && v != 0 {
			e = nil // skip / true: always enabled
		}
		l.addEdge(it, ir.Edge{From: from, To: to, Guard: e}, panLine)
		return nil
	case *XrXs:
		for _, c := range s.Chans {
			ch, err := l.channel(c, it.Pos)
			if err != nil {
				return err
			}
			if s.Kind == "xr" {
				ch.XR = appendUnique(ch.XR, in.pt.Name)
			} else {
				ch.XS = appendUnique(ch.XS, in.pt.Name)
			}
		}
		return l.alias(from, to, it.Pos)
	}
	return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "internal: unknown statement %T", it.Stmt)
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

func (l *lowerer) channel(name string, at Pos) (*ir.Channel, *Error) {
	ch := l.chans[name]
	if ch == nil {
		if l.globals[name] != nil || l.cur.declared[name] {
			return nil, semanticErr(l.file, at.Line, at.Col, "%s is a variable, not a channel", name)
		}
		return nil, semanticErr(l.file, at.Line, at.Col, "undeclared channel %s", name)
	}
	return ch, nil
}

// resolve finds a variable name: the process's visible locals shadow
// globals. It returns the declaration and whether it is a local.
func (l *lowerer) resolve(name string) (*ir.Var, bool) {
	if l.cur != nil && l.cur.visible(name) {
		for i := range l.cur.locals {
			if l.cur.locals[i].Name == name {
				return &l.cur.locals[i], true
			}
		}
	}
	if g := l.globals[name]; g != nil {
		return g, false
	}
	return nil, false
}

func (l *lowerer) lvalue(lv *LValue, pid int) (ir.Assign, *Error) {
	v, _ := l.resolve(lv.Name)
	if v == nil {
		if _, ok := l.mtype[lv.Name]; ok {
			return ir.Assign{}, semanticErr(l.file, lv.Pos.Line, lv.Pos.Col, "cannot assign to mtype constant %s", lv.Name)
		}
		if lv.Name == "_pid" {
			return ir.Assign{}, semanticErr(l.file, lv.Pos.Line, lv.Pos.Col, "cannot assign to _pid")
		}
		return ir.Assign{}, semanticErr(l.file, lv.Pos.Line, lv.Pos.Col, "undeclared variable %s", lv.Name)
	}
	a := ir.Assign{Var: lv.Name}
	if lv.Index != nil {
		if v.Len == 0 {
			return ir.Assign{}, semanticErr(l.file, lv.Pos.Line, lv.Pos.Col, "%s is not an array", lv.Name)
		}
		ix, err := l.expr(lv.Index, pid, false)
		if err != nil {
			return ir.Assign{}, err
		}
		a.Index = ix
	} else if v.Len > 0 {
		return ir.Assign{}, semanticErr(l.file, lv.Pos.Line, lv.Pos.Col, "array %s used without an index", lv.Name)
	}
	return a, nil
}

var binOps = map[string]string{
	"||": "or", "&&": "and", "==": "eq", "!=": "ne", "<": "lt", "<=": "le", ">": "gt", ">=": "ge",
	"+": "add", "-": "sub", "*": "mul", "/": "div", "%": "mod",
}

// expr lowers an expression in process pid (-1: global scope). In
// initialisers (constOnly) variables are not resolved, only constants and
// _pid.
func (l *lowerer) expr(e Expr, pid int, constOnly bool) (*ir.Expr, *Error) {
	switch x := e.(type) {
	case *Num:
		return ir.Const(x.Val), nil
	case *TimeoutExpr:
		return ir.Timeout(), nil
	case *Unary:
		a, err := l.expr(x.X, pid, constOnly)
		if err != nil {
			return nil, err
		}
		if x.Op == "!" {
			return ir.Unary("not", a), nil
		}
		return ir.Unary("neg", a), nil
	case *Binary:
		a, err := l.expr(x.X, pid, constOnly)
		if err != nil {
			return nil, err
		}
		b, err := l.expr(x.Y, pid, constOnly)
		if err != nil {
			return nil, err
		}
		return ir.Binary(binOps[x.Op], a, b), nil
	case *ChanExpr:
		ch, err := l.channel(x.Chan, x.Pos)
		if err != nil {
			return nil, err
		}
		ln := ir.Len(x.Chan)
		switch x.Fn {
		case "len":
			return ln, nil
		case "empty":
			return ir.Binary("eq", ln, ir.Const(0)), nil
		case "nempty":
			return ir.Binary("gt", ln, ir.Const(0)), nil
		case "full":
			return ir.Binary("eq", ln, ir.Const(int64(ch.Capacity))), nil
		default: // nfull
			return ir.Binary("lt", ln, ir.Const(int64(ch.Capacity))), nil
		}
	case *VarRef:
		if x.Name == "_pid" {
			if pid < 0 {
				return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "_pid outside a process")
			}
			return ir.Const(int64(pid)), nil
		}
		if !constOnly {
			if v, _ := l.resolve(x.Name); v != nil {
				if v.Len > 0 {
					return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "array %s used without an index", x.Name)
				}
				return ir.Ref(x.Name), nil
			}
		}
		if v, ok := l.mtype[x.Name]; ok {
			return ir.Const(v), nil
		}
		if l.chans[x.Name] != nil {
			return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "channel %s used as a value", x.Name)
		}
		if constOnly {
			return nil, outside(l.file, x.Pos.Line, x.Pos.Col, "non-constant initialiser", fmt.Sprintf("initialiser refers to %s", x.Name))
		}
		return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "undeclared variable %s", x.Name)
	case *IndexExpr:
		if constOnly {
			return nil, outside(l.file, x.Pos.Line, x.Pos.Col, "non-constant initialiser", fmt.Sprintf("initialiser refers to %s", x.Name))
		}
		v, _ := l.resolve(x.Name)
		if v == nil {
			return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "undeclared variable %s", x.Name)
		}
		if v.Len == 0 {
			return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "%s is not an array", x.Name)
		}
		ix, err := l.expr(x.Index, pid, constOnly)
		if err != nil {
			return nil, err
		}
		return ir.Index(x.Name, ix), nil
	}
	return nil, semanticErr(l.file, 0, 0, "internal: unknown expression %T", e)
}

// finalise resolves aliases, drops the locations pan would drop
// (unreachable in the process's own automaton — SPIN prunes them, so a
// looping proctype has no `-end-` transition) and numbers the rest densely.
func (l *lowerer) finalise(in *instance) {
	// reachability over representatives, from the entry
	out := map[int][]int{}
	for _, e := range in.edges {
		f := in.find(e.From)
		out[f] = append(out[f], in.find(e.To))
	}
	reach := map[int]bool{}
	stack := []int{in.find(in.entry)}
	if in.runCreated {
		reach[in.find(in.dormant)] = true
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if reach[n] {
			continue
		}
		reach[n] = true
		stack = append(stack, out[n]...)
	}
	order := []int{}
	for n := range in.nodes {
		r := in.find(n)
		if !reach[r] {
			continue
		}
		if _, ok := in.final[r]; !ok {
			in.final[r] = len(order)
			order = append(order, r)
		}
	}
	for _, r := range order {
		nd := &in.nodes[r]
		loc := ir.Location{Name: nd.name, Origin: nd.origin}
		seen := map[ir.Label]bool{}
		for _, lb := range nd.labels {
			if !seen[lb] {
				seen[lb] = true
				loc.Labels = append(loc.Labels, lb)
			}
		}
		in.proc.Locations = append(in.proc.Locations, loc)
	}
	in.proc.Initial = in.final[in.find(in.entry)]
	if in.runCreated {
		in.proc.Initial = in.final[in.find(in.dormant)]
	}
	in.edgeMap = make([]int, len(in.edges))
	for i := range in.edges {
		e := in.edges[i]
		from, to := in.find(e.From), in.find(e.To)
		if !reach[from] {
			in.edgeMap[i] = -1
			continue
		}
		e.From, e.To = in.final[from], in.final[to]
		tgt := &in.nodes[to]
		e.Atomic = len(tgt.atomic) > 0
		e.DStep = len(tgt.dstep) > 0
		in.edgeMap[i] = len(in.proc.Edges)
		in.proc.Edges = append(in.proc.Edges, e)
		in.finalPanLines = append(in.finalPanLines, in.panLines[i])
	}
	if in.proc.Edges == nil {
		in.proc.Edges = []ir.Edge{}
	}
}
