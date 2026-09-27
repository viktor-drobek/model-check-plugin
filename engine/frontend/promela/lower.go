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
//     its exit location (the closing brace) carries the `end` label, which
//     for a claim means "the claim terminated" — a violation in the
//     product search (explore/cycle.go). The model gets an `ltl` property
//     `never` without formula for it; a model with accept labels and no
//     claim gets `accept`; a model with progress labels gets `progress`.

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
	// chanObj / chanArr are the channel objects this instance declares
	// locally: a scalar maps to its index in Model.Channels, an array to the
	// length of the byte array of ids that names its elements (G5).
	chanObj map[string]int
	chanArr map[string]int
	// localChans lists the indices of the channel objects the instance owns,
	// so that its `-end-` edge can empty them (a process releases its
	// channels when it dies).
	localChans []int
	// initAssigns are the initialisers of a dynamic instance's locals. A
	// dynamic instance starts with everything at zero (its dormant slot must
	// be indistinguishable from the slot of a process that has died, or the
	// two would be counted as different states), so its initialisers run as
	// part of the `run` step, in the new process's own scope — which is also
	// what lets `byte maximum = mynumber` read a parameter, as SPIN does.
	initAssigns []ir.Assign

	nodes  []node
	labels map[string]int
	// defined holds the labels a statement actually carries; gotos holds
	// every `goto` with its position. A goto to a label that no statement
	// carries is rejected (SPIN: "undefined label"): silently dropping the
	// jump would change the model into a different one that happens to
	// verify, which is the one thing a checker must never do.
	defined                           map[string]bool
	gotos                             []gotoRef
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

// gotoRef is one `goto` and where it was written.
type gotoRef struct {
	label string
	pos   Pos
}

type lowerer struct {
	mod      *Module
	file     string
	maxProcs int
	mtype    map[string]int64
	globals  map[string]*ir.Var
	chans    map[string]*ir.Channel
	chanCap  map[string]int
	// gChanObj / gChanArr are the global channel objects: a scalar maps to
	// its index in Model.Channels, an array to its length (its elements are
	// named by a byte array of channel ids with the same name).
	gChanObj map[string]int
	gChanArr map[string]int
	model    *ir.Model
	insts    []*instance
	// pools maps a proctype name to the instances a `run` may start, in
	// allocation order; byRun holds the dedicated instance of a `run` the
	// frontend could see is taken at most once.
	pools     map[string][]*instance
	byRun     map[*RunStmt]*instance
	dynamic   map[string]bool
	warnings  []string
	printfs   int
	cur       *instance
	curAtomic []int
	curDStep  []int
	blockID   int
	// declDepth is the nesting of `{ }` blocks inside the process body. A
	// declaration at depth 0 is not a step (its value is the variable's
	// initial value); a declaration in a nested scope IS a step that assigns
	// the initial value, which is what pan generates ("y = 0", "z = 5") —
	// probe in inline.go. An inline expansion is such a nested scope.
	declDepth int
	// current block context
}

type seqCtx struct {
	breakTo  int  // node of the enclosing do's exit, -1 if none
	isOption bool // the sequence is an if/do option (lone goto/break rule)
	// nextLine is the line of the statement that follows the current one;
	// pan attributes the initialisation step of a nested declaration to it.
	nextLine int
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

// DefaultMaxProcs is the size of the instance pool the lowering gives a
// proctype whose `run` may be taken more than once (inside a loop or an
// option, or in a process that itself exists many times). It is a declared
// bound: exceeding it stops the search with an inconclusive verdict, never
// with a wrong one. SPIN's own bound is 255 processes in total.
const DefaultMaxProcs = 8

// Lower translates the module.
func Lower(mod *Module, file, name string) (*Lowered, *Error) {
	return LowerWith(mod, file, name, DefaultMaxProcs)
}

// LowerWith translates the module with an explicit process-pool bound.
func LowerWith(mod *Module, file, name string, maxProcs int) (*Lowered, *Error) {
	m, w, pl, err := lower(mod, file, name, maxProcs)
	if err != nil {
		return nil, err
	}
	return &Lowered{Model: m, Warnings: w, PanLines: pl}, nil
}

func lower(mod *Module, file, name string, maxProcs int) (*ir.Model, []string, map[string][]int, *Error) {
	if maxProcs <= 0 {
		maxProcs = DefaultMaxProcs
	}
	l := &lowerer{mod: mod, file: file, maxProcs: maxProcs, mtype: map[string]int64{}, globals: map[string]*ir.Var{},
		chans: map[string]*ir.Channel{}, chanCap: map[string]int{},
		gChanObj: map[string]int{}, gChanArr: map[string]int{},
		pools: map[string][]*instance{}, byRun: map[*RunStmt]*instance{}, dynamic: map[string]bool{}}
	m := &ir.Model{Schema: ir.Schema, Name: name, Origin: &ir.Origin{File: file, Name: name}}
	l.model = m

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
	seenGlobals := map[string]bool{}
	for _, d := range mod.Globals {
		if d.Type == "chan" && d.Chan != nil {
			if _, dup := l.chans[d.Name]; dup || l.globals[d.Name] != nil || l.mtype[d.Name] != 0 {
				return nil, nil, nil, semanticErr(file, d.Pos.Line, d.Pos.Col, "%s declared twice", d.Name)
			}
			ids, err := l.declChanObjects(d, d.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			if d.Len > 0 {
				// The elements of a channel array are named by a byte array
				// of channel ids, so that `q[i]` with a computed index is an
				// ordinary array read that yields a channel.
				l.gChanArr[d.Name] = d.Len
				m.Globals = append(m.Globals, ir.Var{Name: d.Name, Type: ir.Byte, Len: d.Len, Init: ids,
					Origin: &ir.Origin{File: file, Line: d.Pos.Line, Name: d.Name}})
			} else {
				l.gChanObj[d.Name] = int(ids[0]) - 1
			}
			continue
		}
		v, err := l.varDecl(d, -1)
		if err != nil {
			return nil, nil, nil, err
		}
		// l.globals is filled after this loop (the slice moves as it grows),
		// so the duplicate check needs its own set: without it a second
		// global of the same name reached ir.Validate and came back as
		// "internal: lowered IR does not validate", which reads as an engine
		// fault for what is a fault in the model.
		if seenGlobals[v.Name] || l.mtype[v.Name] != 0 || l.chans[v.Name] != nil {
			return nil, nil, nil, semanticErr(file, d.Pos.Line, d.Pos.Col, "redeclaration of %s: the name is already a global variable, channel or mtype constant (SPIN: \"redeclaration of '%s'\")", d.Name, d.Name)
		}
		seenGlobals[v.Name] = true
		m.Globals = append(m.Globals, *v)
	}
	// re-point channel pointers after appends
	for i := range m.Channels {
		l.chans[m.Channels[i].Name] = &m.Channels[i]
	}
	for i := range m.Globals {
		l.globals[m.Globals[i].Name] = &m.Globals[i]
	}

	// pass 1: instances in pid order — the static ones first, in textual
	// order (SPIN's pid order), then the pools a `run` draws from.
	byName := map[string]*Proctype{}
	for _, pt := range mod.Procs {
		byName[pt.Name] = pt
	}
	// A proctype name lives in the same namespace as the mtype constants,
	// the globals and the channels: SPIN keeps one symbol table, and a
	// proctype that shares a name with any of them is refused there. Letting
	// it through here would leave `run P()` and the constant `P` meaning
	// different things under one identifier.
	for _, pt := range mod.Procs {
		if err := l.checkProctypeName(pt, seenGlobals); err != nil {
			return nil, nil, nil, err
		}
	}
	if mod.Never != nil {
		if err := l.checkProctypeName(mod.Never, seenGlobals); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, pt := range mod.Procs {
		for k := 0; k < pt.Active; k++ {
			l.insts = append(l.insts, &instance{pt: pt, pid: len(l.insts)})
		}
	}
	// Every `run` of the module, in the textual order of the proctypes that
	// contain them, so that the instance order is a function of the source.
	type runSite struct {
		stmt *RunStmt
		in   *Proctype
	}
	var sites []runSite
	for _, pt := range mod.Procs {
		var runs []*RunStmt
		collectRuns(pt.Body.Items, &runs)
		for _, r := range runs {
			sites = append(sites, runSite{r, pt})
		}
	}
	for _, s := range sites {
		if byName[s.stmt.Proc] == nil || byName[s.stmt.Proc].IsInit {
			return nil, nil, nil, semanticErr(file, s.stmt.Pos.Line, s.stmt.Pos.Col, "run of undeclared proctype %s", s.stmt.Proc)
		}
		l.dynamic[s.stmt.Proc] = true
	}
	// A run statement needs a pool rather than one dedicated instance when
	// it can be taken more than once: inside an if/do option, or in a
	// proctype that itself exists more than once (several `active`
	// instances, or one created by `run`).
	shared := map[string]bool{}
	for _, s := range sites {
		if s.stmt.InLoop || s.in.Active > 1 || l.dynamic[s.in.Name] {
			shared[s.stmt.Proc] = true
		}
	}
	for _, pt := range mod.Procs {
		if !l.dynamic[pt.Name] {
			continue
		}
		n := 0
		if shared[pt.Name] {
			n = maxProcs
		} else {
			for _, s := range sites {
				if s.stmt.Proc == pt.Name {
					n++
				}
			}
		}
		for k := 0; k < n; k++ {
			in := &instance{pt: pt, pid: len(l.insts), runCreated: true}
			l.insts = append(l.insts, in)
			l.pools[pt.Name] = append(l.pools[pt.Name], in)
		}
	}
	for _, s := range sites {
		pt := byName[s.stmt.Proc]
		if len(s.stmt.Args) != len(pt.Params) {
			return nil, nil, nil, semanticErr(file, s.stmt.Pos.Line, s.stmt.Pos.Col, "run %s: %d argument(s) for %d parameter(s)", s.stmt.Proc, len(s.stmt.Args), len(pt.Params))
		}
	}
	if !anyShared(shared) {
		// Every run is taken at most once: give each its own instance, in
		// the textual order of the run statements, which is what G1 did and
		// what keeps those models' vectors unchanged.
		used := map[string]int{}
		for _, s := range sites {
			pool := l.pools[s.stmt.Proc]
			l.byRun[s.stmt] = pool[used[s.stmt.Proc]]
			used[s.stmt.Proc]++
		}
	} else {
		for _, s := range sites {
			if !shared[s.stmt.Proc] {
				pool := l.pools[s.stmt.Proc]
				l.byRun[s.stmt] = pool[0]
			}
		}
	}
	if len(l.insts) == 0 {
		return nil, nil, nil, semanticErr(file, 0, 0, "the model has no active process and no init")
	}
	if len(l.insts) > 254 {
		return nil, nil, nil, semanticErr(file, 0, 0, "%d process instances exceed the 254 the engine can name; lower --max-procs", len(l.insts))
	}
	if mod.Never != nil {
		l.insts = append(l.insts, &instance{pt: mod.Never, pid: len(l.insts), claim: true})
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
	hasDynamic := false
	for _, in := range l.insts {
		if in.runCreated {
			hasDynamic = true
		}
	}
	for k, in := range l.insts {
		for _, pr := range in.runs {
			e := &in.proc.Edges[in.edgeMap[pr.edge]]
			e.Run.Entry = pr.target.final[pr.target.find(pr.target.entry)]
			e.Run.Init = pr.target.initAssigns
		}
		if in.endEdge >= 0 && in.edgeMap[in.endEdge] >= 0 {
			edge := &in.proc.Edges[in.edgeMap[in.endEdge]]
			if hasDynamic {
				// With a live-process table the rule is pan's own, stated
				// once: only the youngest live process may leave the vector.
				edge.Guard = ir.Youngest(k)
				edge.Leave = true
			} else {
				var conj []*ir.Expr
				for j := k + 1; j < len(l.insts); j++ {
					o := l.insts[j]
					if o.claim {
						continue
					}
					// j is "not alive" when dead. A process whose end is
					// unreachable never dies: constant false.
					var dead *ir.Expr
					if d, ok := o.final[o.find(o.deadNode)]; ok {
						dead = ir.Binary("eq", ir.PC(j), ir.Const(int64(d)))
					} else {
						dead = ir.Const(0)
					}
					conj = append(conj, dead)
				}
				if len(conj) > 0 {
					edge.Guard = ir.And(conj...)
				}
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
	hasAccept, hasProgress := false, false
	for _, in := range l.insts {
		if in.claim {
			continue
		}
		for _, loc := range in.proc.Locations {
			for _, lb := range loc.Labels {
				switch lb {
				case ir.Accept:
					hasAccept = true
				case ir.Progress:
					hasProgress = true
				}
			}
		}
	}
	switch {
	case mod.Never != nil:
		m.Properties = append(m.Properties, ir.Property{ID: "never", Kind: ir.KindLTL,
			Text:   fmt.Sprintf("the never claim (line %d) accepts no run: no acceptance cycle through its accept labels and no run to its end (SPIN: pan -a)", mod.Never.Pos.Line),
			Origin: &ir.Origin{File: file, Line: mod.Never.Pos.Line, Name: "never"}})
	case hasAccept:
		m.Properties = append(m.Properties, ir.Property{ID: "accept", Kind: ir.KindLTL,
			Text: "no acceptance cycle through an accept label of a process (SPIN: pan -a)"})
	}
	if hasProgress {
		m.Properties = append(m.Properties, ir.Property{ID: "progress", Kind: ir.KindProgress,
			Text: "no non-progress cycle: every infinite run visits a progress label infinitely often (SPIN: pan -l)"})
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

// labelNameTaken names what already owns an identifier a statement wants to
// use as a label, or "" when nothing does. SPIN keeps one symbol table, so
// a label may not share a name with a variable, a channel, an mtype
// constant or a proctype; letting one through would leave `goto L` and
// `L = 1` meaning different things under one identifier.
func (l *lowerer) labelNameTaken(in *instance, lb string) string {
	switch {
	case in.declared[lb]:
		return "a variable of " + in.pt.Name
	case l.globals[lb] != nil:
		return "a global variable"
	case l.chans[lb] != nil:
		return "a channel"
	case l.mtype[lb] != 0:
		return "an mtype constant"
	}
	for _, pt := range l.mod.Procs {
		if pt.Name == lb {
			return "a proctype"
		}
	}
	return ""
}

// checkProctypeName refuses a proctype whose name is already taken by an
// mtype constant, a global variable or a channel.
func (l *lowerer) checkProctypeName(pt *Proctype, globals map[string]bool) *Error {
	what := ""
	switch {
	case l.mtype[pt.Name] != 0:
		what = "an mtype constant"
	case l.chans[pt.Name] != nil:
		what = "a channel"
	case globals[pt.Name]:
		what = "a global variable"
	}
	if what == "" {
		return nil
	}
	return semanticErr(l.file, pt.Pos.Line, pt.Pos.Col,
		"proctype %s: the name is already %s, and SPIN keeps one namespace for both", pt.Name, what)
}

func anyShared(m map[string]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}

// collectRuns walks the whole body, options included: since G5 a `run` may
// stand anywhere.
func collectRuns(items []Item, out *[]*RunStmt) {
	for _, it := range items {
		switch s := it.Stmt.(type) {
		case *RunStmt:
			*out = append(*out, s)
		case *Block:
			collectRuns(s.Items, out)
		case *If:
			for _, o := range s.Options {
				collectRuns(o, out)
			}
		case *Do:
			for _, o := range s.Options {
				collectRuns(o, out)
			}
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
	// byte, mtype, pid, and chan — a channel-typed value is a channel id,
	// which is a byte (G5).
	return ir.Byte
}

// declChanObjects creates the ir.Channels of a `chan` declaration with a
// `[cap] of { … }` initialiser: one for a scalar, Len for an array (named
// "q[0]", "q[1]", … as pan prints them). It returns their ids.
func (l *lowerer) declChanObjects(d *VarDecl, base string) ([]int64, *Error) {
	n := d.Len
	if n == 0 {
		n = 1
	}
	var ids []int64
	for k := 0; k < n; k++ {
		name := base
		if d.Len > 0 {
			name = fmt.Sprintf("%s[%d]", base, k)
		}
		if _, dup := l.chans[name]; dup {
			return nil, semanticErr(l.file, d.Pos.Line, d.Pos.Col, "%s declared twice", name)
		}
		ch := ir.Channel{Name: name, Capacity: d.Chan.Cap, Origin: &ir.Origin{File: l.file, Line: d.Pos.Line, Name: name}}
		for _, f := range d.Chan.Fields {
			ch.Fields = append(ch.Fields, irType(f))
		}
		l.model.Channels = append(l.model.Channels, ch)
		ids = append(ids, ir.ChanID(len(l.model.Channels)-1))
		l.chanCap[name] = d.Chan.Cap
	}
	// The slice may have been reallocated: re-point every pointer.
	for i := range l.model.Channels {
		l.chans[l.model.Channels[i].Name] = &l.model.Channels[i]
	}
	return ids, nil
}

// varDecl builds an ir.Var; pid >= 0 for locals (for _pid folding).
func (l *lowerer) varDecl(d *VarDecl, pid int) (*ir.Var, *Error) {
	v := &ir.Var{Name: d.Name, Type: irType(d.Type), Len: d.Len, Origin: &ir.Origin{File: l.file, Line: d.Pos.Line, Name: d.Name}}
	if d.Init != nil && l.cur != nil && l.cur.runCreated && pid >= 0 {
		e, err := l.expr(d.Init, pid, false)
		if err != nil {
			return nil, err
		}
		if d.Len > 0 {
			for i := 0; i < d.Len; i++ {
				l.cur.initAssigns = append(l.cur.initAssigns, ir.Assign{Var: d.Name, Index: ir.Const(int64(i)), Value: e})
			}
		} else {
			l.cur.initAssigns = append(l.cur.initAssigns, ir.Assign{Var: d.Name, Value: e})
		}
		return v, nil
	}
	if d.Init != nil {
		e, err := l.expr(d.Init, pid, true)
		if err != nil {
			return nil, err
		}
		val, ok := constValue(e)
		if !ok {
			return nil, outside(l.file, d.Pos.Line, d.Pos.Col, "non-constant initialiser", fmt.Sprintf("%s is initialised with an expression over variables; only constants and _pid are accepted (a process created by run may also read its parameters)", d.Name))
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
	in.defined = map[string]bool{}
	in.declared = map[string]bool{}
	in.final = map[int]int{}
	in.endEdge = -1
	pt := in.pt
	in.chanObj, in.chanArr = map[string]int{}, map[string]int{}
	in.proc = ir.Process{Name: in.name, Claim: in.claim, Dynamic: in.runCreated,
		Origin: &ir.Origin{File: l.file, Line: pt.Pos.Line, Name: pt.Name}}
	// parameters
	in.scopes = []map[string]bool{{}}
	for _, d := range pt.Params {
		v, err := l.varDecl(d, in.pid)
		if err != nil {
			return err
		}
		if _, err := l.declare(v, d.Pos); err != nil {
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
	}
	// For a process, `end` on the exit location marks a valid end state;
	// for a claim it marks the claim's termination (a violation).
	in.nodes[in.endNode].labels = append(in.nodes[in.endNode].labels, ir.End)
	if in.claim {
		in.nodes[in.endNode].name = "-end-"
	}
	if err := l.lowerSeq(pt.Body.Items, in.entry, in.endNode, seqCtx{breakTo: -1}); err != nil {
		return err
	}
	for _, g := range in.gotos {
		if !in.defined[g.label] {
			return semanticErr(l.file, g.pos.Line, g.pos.Col,
				"undefined label %s: %s has no statement labelled %s, so the goto has no target (SPIN reports the same)", g.label, pt.Name, g.label)
		}
	}
	if !in.claim {
		// A dynamic instance returns to its dormant location, so that the
		// same pool slot can be started again: for pan the process has left
		// the vector, and "dormant" is how this engine writes that.
		to := in.deadNode
		if in.runCreated {
			to = in.dormant
		}
		e := ir.Edge{From: in.endNode, To: to, Text: "-end-", ClearChans: in.localChans,
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
	if pt.Provided != nil {
		pv, err := l.expr(pt.Provided, in.pid, false)
		if err != nil {
			return err
		}
		in.proc.Provided = pv
	}
	in.proc.Locals = in.locals
	return nil
}

// declare adds a local; fresh is false when the variable already exists and
// this declaration only re-initialises it.
//
// SPIN's rule, established by probing SPIN 6.5.2 shape by shape rather than
// guessed (steps/g5-addendum2-confirmation.md §2): a declaration is an
// error when the name is *visible* where it stands — declared in the same
// scope, or in a scope still open around it (an enclosing block, the
// proctype body, a parameter, a global) — and legal when the only earlier
// declaration was in a scope that has since closed. Only `{ }` opens a
// scope: the options of an `if`/`do` do not, so two options declaring one
// name are a redeclaration for SPIN and are one here too.
//
// For the legal case — two sibling blocks, or two expansions of an inline
// that declares a variable — SPIN keeps one variable and re-initialises it
// at each declaration, and so does this frontend (the initialisation is the
// step lowerStmt emits). The types must then agree: two different types
// under one name would need two slots, which the flat per-process set of
// locals cannot give.
func (l *lowerer) declare(v *ir.Var, at Pos) (bool, *Error) {
	in := l.cur
	if in.visible(v.Name) {
		return false, semanticErr(l.file, at.Line, at.Col,
			"redeclaration of %s: the name is already declared in this scope or in one still open around it (SPIN: \"redeclaration of '%s'\"); only a block that has closed frees the name",
			v.Name, v.Name)
	}
	if l.globals[v.Name] != nil {
		return false, semanticErr(l.file, at.Line, at.Col,
			"redeclaration of %s: it is already a global variable, and a local of that name would shadow it (SPIN refuses the same)", v.Name)
	}
	if l.chans[v.Name] != nil || l.mtype[v.Name] != 0 {
		return false, semanticErr(l.file, at.Line, at.Col, "%s is already a channel or mtype constant", v.Name)
	}
	if in.defined[v.Name] {
		return false, semanticErr(l.file, at.Line, at.Col,
			"%s is already a label of %s: one identifier cannot be both a variable and a control location (SPIN refuses the same)", v.Name, in.pt.Name)
	}
	if in.declared[v.Name] {
		// A closed sibling scope used the name: one variable, re-initialised.
		old := fresh0(in, v.Name)
		if old.Type != v.Type || old.Len != v.Len {
			return false, semanticErr(l.file, at.Line, at.Col,
				"%s is declared twice in %s with different types (%s and %s); the engine keeps one flat set of locals per process",
				v.Name, in.pt.Name, old.Type, v.Type)
		}
		in.scopes[len(in.scopes)-1][v.Name] = true
		return false, nil
	}
	in.declared[v.Name] = true
	in.scopes[len(in.scopes)-1][v.Name] = true
	in.locals = append(in.locals, *v)
	return true, nil
}

// fresh0 is the declaration of a local by name.
func fresh0(in *instance, name string) *ir.Var {
	for i := range in.locals {
		if in.locals[i].Name == name {
			return &in.locals[i]
		}
	}
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
		for li, lb := range it.Labels {
			at := it.Pos
			if li < len(it.LabelPos) {
				at = it.LabelPos[li]
			}
			if what := l.labelNameTaken(in, lb); what != "" {
				return semanticErr(l.file, at.Line, at.Col,
					"label %s: the name is already %s, and SPIN keeps one namespace for both (it reports \"bad label-name %s\")", lb, what, lb)
			}
			if in.defined[lb] {
				// Two statements of one process carrying the same label:
				// SPIN refuses it ("label L redeclared"), and folding the
				// two locations into one — which is what the alias below
				// would do — would silently check a different model.
				return semanticErr(l.file, at.Line, at.Col,
					"label %s redeclared: %s already has a statement labelled %s (SPIN reports the same)", lb, in.pt.Name, lb)
			}
			in.defined[lb] = true
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
		c := ctx
		if i+1 < len(items) {
			c.nextLine = items[i+1].Pos.Line
		}
		if err := l.lowerStmt(it, pos[i], pos[i+1], c, i == 0 && ctx.isOption); err != nil {
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
		l.declDepth++
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
		l.declDepth--
		in.scopes = in.scopes[:len(in.scopes)-1]
		return err
	case *Decl:
		if s.Var.Type == "chan" && s.Var.Chan != nil {
			if in.runCreated {
				return outside(l.file, s.Var.Pos.Line, s.Var.Pos.Col, "channel declared inside a process created by run",
					fmt.Sprintf("%s owns channel %s, and this engine version gives a channel to a fixed instance, not to each incarnation of a pool slot; declare the channel globally and pass it as a parameter", in.pt.Name, s.Var.Name))
			}
			// A channel local to a process is one channel per instance: it
			// is named "<instance>.<name>" in the IR, and the name the body
			// uses stands for its id.
			base := in.name + "." + s.Var.Name
			ids, err := l.declChanObjects(s.Var, base)
			if err != nil {
				return err
			}
			for _, id := range ids {
				in.localChans = append(in.localChans, int(id)-1)
			}
			if s.Var.Len > 0 {
				in.chanArr[s.Var.Name] = s.Var.Len
				v := &ir.Var{Name: s.Var.Name, Type: ir.Byte, Len: s.Var.Len, Init: ids,
					Origin: &ir.Origin{File: l.file, Line: s.Var.Pos.Line, Name: s.Var.Name}}
				if _, err := l.declare(v, s.Var.Pos); err != nil {
					return err
				}
			} else {
				in.chanObj[s.Var.Name] = int(ids[0]) - 1
			}
			return l.alias(from, to, it.Pos)
		}
		v, err := l.varDecl(s.Var, pid)
		if err != nil {
			return err
		}
		fresh, err := l.declare(v, s.Var.Pos)
		if err != nil {
			return err
		}
		if l.declDepth == 0 {
			return l.alias(from, to, it.Pos)
		}
		// A declaration in a nested scope is a step that sets the initial
		// value (pan does the same, and attributes it to the line of the
		// statement that follows it).
		if !fresh {
			// The name is already a local of the process: SPIN keeps one
			// variable and re-initialises it here.
			v = fresh0(in, v.Name)
		}
		e := ir.Edge{From: from, To: to}
		var texts []string
		for i := 0; i < v.Count(); i++ {
			val := int64(0)
			if i < len(v.Init) {
				val = v.Init[i]
			}
			a := ir.Assign{Var: v.Name, Value: ir.Const(val)}
			target := v.Name
			if v.Len > 0 {
				a.Index = ir.Const(int64(i))
				target = fmt.Sprintf("%s[%d]", v.Name, i)
			}
			e.Effect = append(e.Effect, a)
			texts = append(texts, fmt.Sprintf("%s = %d", target, val))
		}
		line := it.Pos.Line
		if ctx.nextLine > 0 {
			line = ctx.nextLine
		}
		e.Text = strings.Join(texts, "; ")
		e.Origin = &ir.Origin{File: l.file, Line: line, Name: e.Text}
		l.cur.edges = append(l.cur.edges, e)
		l.cur.panLines = append(l.cur.panLines, line)
		return nil
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
		in.gotos = append(in.gotos, gotoRef{s.Label, it.Pos})
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
		name, sel, err := l.chanRef(s.Chan, s.Index, pid, it.Pos)
		if err != nil {
			return err
		}
		if err := l.checkChanUse(name, len(s.Args), "!", it.Pos); err != nil {
			return err
		}
		op := &ir.ChanOp{Chan: name, Sel: sel}
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
		name, sel, err := l.chanRef(s.Chan, s.Index, pid, it.Pos)
		if err != nil {
			return err
		}
		if err := l.checkChanUse(name, len(s.Args), "?", it.Pos); err != nil {
			return err
		}
		op := &ir.RecvOp{Chan: name, Sel: sel}
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
		pool := l.pools[s.Proc]
		if len(pool) == 0 {
			return semanticErr(l.file, it.Pos.Line, it.Pos.Col, "internal: run statement without an instance pool")
		}
		// Every `run` draws from the pool of its proctype, and the explorer
		// takes the first instance that is still dormant. Giving a run that
		// is taken at most once its *own* instance would be simpler, and G1
		// did that — but it breaks the invariant the whole encoding rests
		// on ("the k-th live instance of a proctype is the k-th slot of its
		// pool"), and with it the agreement with pan: if the first instance
		// dies before the second `run` fires, pan starts the new process at
		// the pid the dead one freed, while a dedicated second instance
		// keeps the two apart and counts states pan counts once. K3's
		// mutation campaign found exactly that (two `run`s with equal
		// arguments: 14 states here against pan's 12).
		target := pool[0]
		r := &ir.RunOp{Proc: pool[0].pid}
		for _, q := range pool {
			r.Pool = append(r.Pool, q.pid)
		}
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
			// The pid of the process just started is the last of the live
			// table, whichever pool slot it took.
			a.Value = ir.Binary("sub", ir.NrPr(), ir.Const(1))
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
		// `xr`/`xs` are hints for a later partial-order reduction and change
		// nothing in the search. On a channel that is only known in a state
		// (a parameter) there is no channel object to attach them to, so the
		// hint is dropped with a warning rather than guessed at.
		for _, c := range s.Chans {
			name, _, err := l.chanRef(c, nil, pid, it.Pos)
			if err != nil {
				return err
			}
			if name == "" {
				l.warnings = append(l.warnings, fmt.Sprintf("%s %s (line %d): the channel is only known while the process runs, so the hint is recorded on no channel; it changes nothing in this engine version", s.Kind, c, it.Pos.Line))
				continue
			}
			ch := l.chans[name]
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

// warnOnce records a warning unless the same text is already there, so
// that a construct used many times is reported once.
func (l *lowerer) warnOnce(msg string) {
	for _, w := range l.warnings {
		if w == msg {
			return
		}
	}
	l.warnings = append(l.warnings, msg)
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

// chanRef resolves a channel reference `name` or `name[index]` to either
// the name of a channel object (the static case, unchanged since G1) or a
// selector expression yielding a channel id (the dynamic case of G5:
// channel arrays, channel-typed variables, parameters and message fields).
// Exactly one of the two results is non-empty.
func (l *lowerer) chanRef(name string, index Expr, pid int, at Pos) (string, *ir.Expr, *Error) {
	in := l.cur
	if in == nil {
		return "", nil, semanticErr(l.file, at.Line, at.Col, "channel %s used outside a process", name)
	}
	if index == nil {
		if ci, ok := in.chanObj[name]; ok {
			return l.model.Channels[ci].Name, nil, nil
		}
		if ci, ok := l.gChanObj[name]; ok && !in.declared[name] {
			return l.model.Channels[ci].Name, nil, nil
		}
	} else {
		if _, ok := in.chanArr[name]; ok {
			e, err := l.expr(&IndexExpr{Name: name, Index: index, Pos: at}, pid, false)
			return "", e, err
		}
		if _, ok := l.gChanArr[name]; ok && !in.declared[name] {
			e, err := l.expr(&IndexExpr{Name: name, Index: index, Pos: at}, pid, false)
			return "", e, err
		}
	}
	// Not a channel object: it must be a channel-typed variable holding an
	// id — a local, a parameter, a struct field or an array element.
	var src Expr = &VarRef{Name: name, Pos: at}
	if index != nil {
		src = &IndexExpr{Name: name, Index: index, Pos: at}
	}
	v, _ := l.resolve(name)
	if v == nil {
		if _, ok := in.chanArr[name]; ok {
			return "", nil, semanticErr(l.file, at.Line, at.Col, "channel array %s used without an index", name)
		}
		if _, ok := l.gChanArr[name]; ok {
			return "", nil, semanticErr(l.file, at.Line, at.Col, "channel array %s used without an index", name)
		}
		return "", nil, semanticErr(l.file, at.Line, at.Col, "undeclared channel %s", name)
	}
	e, err := l.expr(src, pid, false)
	return "", e, err
}

// checkChanUse checks the arity of a channel operation when the channel is
// known statically; for a dynamic channel the check is made in the state,
// by the explorer, because only there is the channel known.
func (l *lowerer) checkChanUse(name string, nargs int, op string, at Pos) *Error {
	if name == "" {
		return nil
	}
	ch := l.chans[name]
	if ch.Capacity == 0 && len(l.curDStep) > 0 {
		return outside(l.file, at.Line, at.Col, "rendezvous operation inside d_step", "outside the subset")
	}
	if nargs != len(ch.Fields) {
		return semanticErr(l.file, at.Line, at.Col, "%s%s…: %d value(s) for %d field(s)", name, op, nargs, len(ch.Fields))
	}
	return nil
}

// chanCapOf is the capacity of a statically known channel.
func (l *lowerer) chanCapOf(name string) int { return l.chans[name].Capacity }

// resolve finds a variable name: the process's visible locals shadow
// globals. It returns the declaration and whether it is a local.
//
//nolint:unparam // the bool is part of the contract even where unused
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
	case *NrPrExpr:
		return ir.NrPr(), nil
	case *PCValueExpr:
		pe, err := l.expr(x.Proc, pid, constOnly)
		if err != nil {
			return nil, err
		}
		n, ok := constValue(pe)
		if !ok {
			return nil, outside(l.file, x.Pos.Line, x.Pos.Col, "pc_value with a computed process number",
				"this engine version reads pc_value only of a process known when the model is built (a constant or _pid)")
		}
		if n < 0 || n >= int64(len(l.insts)) {
			return nil, semanticErr(l.file, x.Pos.Line, x.Pos.Col, "pc_value(%d): the model has %d process instances", n, len(l.insts))
		}
		l.warnOnce(fmt.Sprintf("pc_value (line %d): the value is this engine's control-location numbering, which is built differently from pan's internal state numbers; a model whose behaviour depends on the number behaves differently here than under SPIN", x.Pos.Line))
		return ir.PC(int(n)), nil
	case *ChanExpr:
		name, sel, err := l.chanRef(x.Chan, x.Index, pid, x.Pos)
		if err != nil {
			return nil, err
		}
		if name != "" {
			ln := ir.Len(name)
			cap := int64(l.chanCapOf(name))
			switch x.Fn {
			case "len":
				return ln, nil
			case "empty":
				return ir.Binary("eq", ln, ir.Const(0)), nil
			case "nempty":
				return ir.Binary("gt", ln, ir.Const(0)), nil
			case "full":
				return ir.Binary("eq", ln, ir.Const(cap)), nil
			default: // nfull
				return ir.Binary("lt", ln, ir.Const(cap)), nil
			}
		}
		// The channel is only known in a state: its length and its capacity
		// are read there too.
		ln := ir.CLen(sel)
		switch x.Fn {
		case "len":
			return ln, nil
		case "empty":
			return ir.Binary("eq", ln, ir.Const(0)), nil
		case "nempty":
			return ir.Binary("gt", ln, ir.Const(0)), nil
		case "full":
			return ir.CFull(sel), nil
		default: // nfull
			return ir.Unary("not", ir.CFull(sel)), nil
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
		// A channel used as a value is its id (G5): that is what a
		// channel-typed variable, parameter or message field carries.
		if l.cur != nil {
			if ci, ok := l.cur.chanObj[x.Name]; ok {
				return ir.Const(ir.ChanID(ci)), nil
			}
		}
		if ci, ok := l.gChanObj[x.Name]; ok && (l.cur == nil || !l.cur.declared[x.Name]) {
			return ir.Const(ir.ChanID(ci)), nil
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
