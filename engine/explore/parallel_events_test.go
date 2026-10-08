package explore

import (
	"context"
	"sort"
	"strings"
	"testing"

	"modelcheck/ir"
)

// Unit tests of the event order (performance plan 5, §2.7), by direct
// construction: events are built out of order, handed to the merge as the
// workers would hand them, and the statuses that come out are those of the
// sequential code applying the same events in the order of its steps.

func TestParKeyOrder(t *testing.T) {
	keys := []parKey{
		{u: 2, q: 1, slot: 0}, {u: 1, q: 9, slot: 2, sub: 3}, {u: 1, q: 9, slot: 2, sub: 1}, {u: 1, q: 9, slot: 1},
		{u: 1, q: 9, slot: 0}, {u: 1, q: parQEnd}, {u: 1, q: 2, slot: 2}, {u: 0, q: 0, slot: 2, sub: 7},
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	want := []parKey{
		{u: 0, q: 0, slot: 2, sub: 7}, {u: 1, q: 2, slot: 2}, {u: 1, q: 9, slot: 0}, {u: 1, q: 9, slot: 1},
		{u: 1, q: 9, slot: 2, sub: 1}, {u: 1, q: 9, slot: 2, sub: 3}, {u: 1, q: parQEnd}, {u: 2, q: 1, slot: 0},
	}
	for i := range keys {
		if keys[i] != want[i] {
			t.Fatalf("position %d: %+v, want %+v", i, keys[i], want[i])
		}
	}
	if (parKey{u: 3}).less(parKey{u: 3}) {
		t.Fatal("a key is less than itself")
	}
}

// eventRig is a finished parallel run of a small model whose properties are
// all undecided (they hold or are never reached), two workers' event slots,
// and the ids of a few stored states to hang traces on.
type eventRig struct {
	s   *search
	r   *parRun
	ids []uint32 // stored states, ascending
}

func newEventRig(t *testing.T, extra ...ir.Property) *eventRig {
	t.Helper()
	m := mutexFlaw()
	m.Properties = []ir.Property{
		{ID: "a", Kind: ir.KindInvariant, Expr: ir.Binary("lt", ir.Ref("cnt"), ir.Const(100)), Text: "c < 100"},
		{ID: "b", Kind: ir.KindInvariant, Expr: ir.Binary("lt", ir.Ref("cnt"), ir.Const(100)), Text: "c < 100 (b)"},
		{ID: "dead", Kind: ir.KindDeadlock},
		{ID: "assert", Kind: ir.KindAssert},
	}
	m.Properties = append(m.Properties, extra...)
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := s.parallelBFS(2, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.stop = ""
	r.nActive = len(r.workers)
	s.undecided = len(s.res.Outcomes)
	for i := range s.res.Outcomes {
		s.res.Outcomes[i].Status = ""
	}
	rig := &eventRig{s: s, r: r}
	r.set.each(func(id uint32, _ []byte) { rig.ids = append(rig.ids, id) })
	if len(rig.ids) < 6 {
		t.Fatalf("only %d states", len(rig.ids))
	}
	return rig
}

// give hands an event to a worker as the expansion or the checks would.
func (g *eventRig) give(worker int, e *parEvent) { g.r.workers[worker].keep(e) }

func (g *eventRig) status(id string) Outcome {
	for _, o := range g.s.res.Outcomes {
		if o.Property.ID == id {
			return o
		}
	}
	return Outcome{}
}

func (g *eventRig) merge(t *testing.T) {
	t.Helper()
	if err := g.r.mergeEvents(); err != nil {
		t.Fatal(err)
	}
}

func (g *eventRig) propIndex(id string) int {
	for i, o := range g.s.res.Outcomes {
		if o.Property.ID == id {
			return i
		}
	}
	return -1
}

func TestParEventsTheSmallestKeyOfAPropertyWins(t *testing.T) {
	g := newEventRig(t)
	a := g.propIndex("a")
	// Two workers found the invariant false in different states; the one whose
	// key is smaller must give the trace, whatever the order they arrive in.
	g.give(0, &parEvent{key: parKey{u: g.ids[3], q: 2, slot: 2}, kind: evInvariant, prop: a, id: g.ids[5], byID: true})
	g.give(1, &parEvent{key: parKey{u: g.ids[2], q: 7, slot: 2}, kind: evInvariant, prop: a, id: g.ids[4], byID: true})
	g.merge(t)
	o := g.status("a")
	if o.Status != Violated {
		t.Fatalf("a is %s", o.Status)
	}
	want, _ := g.r.traceTo(g.ids[4])
	if o.Trace.Summary != want.Summary {
		t.Fatalf("the trace is %q, the one of the smaller key is %q", o.Trace.Summary, want.Summary)
	}
}

func TestParEventsAnErrorEndsTheRunAndDropsWhatFollowsIt(t *testing.T) {
	g := newEventRig(t)
	a, b := g.propIndex("a"), g.propIndex("b")
	g.give(0, &parEvent{key: parKey{u: g.ids[1], q: 4, slot: 2}, kind: evInvariant, prop: a, id: g.ids[2], byID: true})
	g.give(1, &parEvent{key: parKey{u: g.ids[2], q: 1}, kind: evError, from: g.ids[2], text: "domain overflow: x"})
	g.give(0, &parEvent{key: parKey{u: g.ids[3], q: 1, slot: 2}, kind: evInvariant, prop: b, id: g.ids[4], byID: true})
	g.merge(t)
	if got := g.status("a").Status; got != Violated {
		t.Fatalf("a decided before the error keeps its verdict, got %s", got)
	}
	if got := g.status("b").Status; got != InvalidModel {
		t.Fatalf("b decided after the error must not stand: got %s", got)
	}
	if g.s.stop != "invalid model" || !g.s.invalid {
		t.Fatalf("stop %q invalid %v", g.s.stop, g.s.invalid)
	}
	for _, id := range []string{"dead", "assert"} {
		if got := g.status(id).Status; got != InvalidModel {
			t.Fatalf("%s is %s, want invalid-model", id, got)
		}
	}
}

func TestParEventsAnErrorOfADecidedPropertyIsDropped(t *testing.T) {
	g := newEventRig(t)
	a := g.propIndex("a")
	// a is decided at key 1, its expression fails to evaluate at a later state
	// (the sequential checkState does not evaluate a decided property).
	g.give(0, &parEvent{key: parKey{u: g.ids[1], q: 1, slot: 2}, kind: evInvariant, prop: a, id: g.ids[2], byID: true})
	g.give(1, &parEvent{key: parKey{u: g.ids[2], q: 1, slot: 2}, kind: evCheckErr, prop: a, id: g.ids[3], byID: true, text: "index out of range"})
	g.merge(t)
	if got := g.status("a").Status; got != Violated {
		t.Fatalf("a is %s", got)
	}
	if g.s.stop != "" || g.s.invalid {
		t.Fatalf("the error of a decided property stopped the run: %q", g.s.stop)
	}
	// The same error before the decision applies.
	h := newEventRig(t)
	a = h.propIndex("a")
	h.give(0, &parEvent{key: parKey{u: h.ids[1], q: 1, slot: 2}, kind: evCheckErr, prop: a, id: h.ids[2], byID: true, text: "index out of range"})
	h.give(1, &parEvent{key: parKey{u: h.ids[2], q: 1, slot: 2}, kind: evInvariant, prop: a, id: h.ids[3], byID: true})
	h.merge(t)
	if got := h.status("a"); got.Status != InvalidModel || !strings.Contains(got.Reason, "index out of range") {
		t.Fatalf("a is %s (%s), want invalid-model", got.Status, got.Reason)
	}
}

func TestParEventsTwoPropertiesDecidedByOneStateAndTheSweep(t *testing.T) {
	g := newEventRig(t)
	g.s.opt.Sweep = true
	a, b := g.propIndex("a"), g.propIndex("b")
	g.give(0, &parEvent{key: parKey{u: g.ids[1], q: 3, slot: 2, sub: 0}, kind: evInvariant, prop: a, id: g.ids[2], byID: true})
	g.give(1, &parEvent{key: parKey{u: g.ids[1], q: 3, slot: 2, sub: 1}, kind: evInvariant, prop: b, id: g.ids[2], byID: true})
	g.merge(t)
	if g.status("a").Status != Violated || g.status("b").Status != Violated {
		t.Fatalf("a %s, b %s", g.status("a").Status, g.status("b").Status)
	}
	if g.s.stop != "" {
		t.Fatalf("a sweep stops on %q", g.s.stop)
	}
	// Without the sweep, deciding the last property stops the run and drops
	// the events after it.
	h := newEventRig(t, ir.Property{ID: "c", Kind: ir.KindInvariant, Expr: ir.Binary("lt", ir.Ref("cnt"), ir.Const(100)), Text: "c"})
	h.s.res.Outcomes[h.propIndex("dead")].Status, h.s.res.Outcomes[h.propIndex("assert")].Status = Verified, Verified
	h.s.undecided = 3
	h.give(0, &parEvent{key: parKey{u: h.ids[1], q: 3, slot: 2, sub: 0}, kind: evInvariant, prop: h.propIndex("a"), id: h.ids[2], byID: true})
	h.give(1, &parEvent{key: parKey{u: h.ids[1], q: 3, slot: 2, sub: 1}, kind: evInvariant, prop: h.propIndex("b"), id: h.ids[2], byID: true})
	h.give(0, &parEvent{key: parKey{u: h.ids[1], q: 3, slot: 2, sub: 2}, kind: evInvariant, prop: h.propIndex("c"), id: h.ids[2], byID: true})
	h.give(1, &parEvent{key: parKey{u: h.ids[4], q: 1}, kind: evError, from: h.ids[4], text: "boom"})
	h.merge(t)
	if h.s.stop != "all properties decided" || h.s.invalid {
		t.Fatalf("stop %q invalid %v: the error after the last decision must be dropped", h.s.stop, h.s.invalid)
	}
}

func TestParEventsADeadlockFollowsTheStepsOfItsState(t *testing.T) {
	g := newEventRig(t)
	// A failed assert in the last step of u and the deadlock of u (q = end):
	// the assert comes first. A budget between them stops the deadlock.
	g.give(0, &parEvent{key: parKey{u: g.ids[2], q: parQEnd}, kind: evDeadlock, from: g.ids[2]})
	g.give(1, &parEvent{key: parKey{u: g.ids[2], q: 6}, kind: evAssert, from: g.ids[2], chain: nil, final: nil, text: "x == 0"})
	g.give(1, &parEvent{key: parKey{u: g.ids[2], q: 8}, kind: evPool, from: g.ids[2], text: "process budget exhausted: boom"})
	g.merge(t)
	if got := g.status("assert").Status; got != Violated {
		t.Fatalf("assert is %s", got)
	}
	if got := g.status("dead").Status; got != "" {
		t.Fatalf("the deadlock after the budget stop must not be applied: %s", got)
	}
	if !g.s.budgetHit || !strings.HasPrefix(g.s.stop, "process budget") {
		t.Fatalf("stop %q", g.s.stop)
	}
}

func TestParEventsRetainTheFirstKeyOfAClass(t *testing.T) {
	g := newEventRig(t)
	w := g.r.workers[0]
	a := g.propIndex("a")
	k := func(u uint32) parKey { return parKey{u: u, q: 1, slot: 2} }
	if !w.wants(parClass(evInvariant, a), k(9)) {
		t.Fatal("nothing retained yet: the first candidate is wanted")
	}
	w.keep(&parEvent{key: k(9), kind: evInvariant, prop: a})
	if w.wants(parClass(evInvariant, a), k(10)) || w.wants(parClass(evInvariant, a), k(9)) {
		t.Fatal("a key that is not smaller than the retained one is wanted")
	}
	if !w.wants(parClass(evInvariant, a), k(3)) {
		t.Fatal("a smaller key is not wanted")
	}
	// Classes are independent: another property, the error of the same one.
	if !w.wants(parClass(evInvariant, g.propIndex("b")), k(10)) || !w.wants(parClass(evCheckErr, a), k(10)) || !w.wants(int(evAssert), k(10)) {
		t.Fatal("classes share a slot")
	}
	n := 0
	for i := range w.evs {
		if w.evs[i] != nil {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d events retained", n)
	}
}

// A model that fails an assert on every transition keeps one event per worker,
// not one per transition (scenario 31's retention rule).
func TestParallelRetentionIsBoundedByTheClasses(t *testing.T) {
	m := promelaModel(t, `byte x; byte y;
active proctype P() { do :: x = (x + 1) % 5; assert(0) :: y = (y + 1) % 5; assert(0) od }
active proctype Q() { do :: x = (x + 2) % 5; assert(0) :: y = (y + 2) % 5; assert(0) od }`)
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: Options{Sweep: true}, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := newParRun(s, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.run(); err != nil {
		t.Fatal(err)
	}
	if s.res.Outcomes[0].Status == "" && s.res.Outcomes[1].Status == "" {
		t.Fatal("the assert property was not decided")
	}
	for _, w := range r.workers {
		for i, e := range w.evs {
			if e != nil {
				t.Fatalf("event of class %d left in a worker after the merges", i)
			}
		}
	}
}

// The step counter makes the order of what happens in one expansion: a
// successor that violates an invariant is stored and checked (slot 2 of the step
// that made it) before the next call of nextEnabled, which fails to evaluate a
// guard, can say anything. If the call did not count as a step, its error would
// sort ahead of the invariant and the invariant would be invalid-model instead
// of violated, as sequentially (mutant: q not incremented for nextEnabled).
func TestParallelAStoredSuccessorIsCheckedBeforeTheNextGuardFails(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "check-then-fail",
		Globals: []ir.Var{byteVar("x"), byteVar("z")},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1"), Edges: []ir.Edge{
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, Text: "x = 1"},
			{From: 0, To: 1, Guard: ir.Binary("gt", ir.Binary("div", ir.Const(10), ir.Ref("z")), ir.Const(0)), Text: "10 / z > 0"},
		}}},
		Properties: []ir.Property{
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(1)), Text: "x != 1"},
			{ID: "deadlock", Kind: ir.KindDeadlock},
		}}
	seq := runOpts(t, m, Options{Mode: BFS, Sweep: true})
	if outcome(t, seq, "inv").Status != Violated || outcome(t, seq, "deadlock").Status != InvalidModel {
		t.Fatalf("breadth-first: inv %s, deadlock %s", outcome(t, seq, "inv").Status, outcome(t, seq, "deadlock").Status)
	}
	for _, kn := range parKnobSets {
		res := runOpts(t, m, Options{Workers: 1, Sweep: true, par: kn})
		if inv := outcome(t, res, "inv").Status; inv != Violated {
			t.Fatalf("knobs %+v: the invariant is %s, the sequential search says violated", kn, inv)
		}
		if d := outcome(t, res, "deadlock").Status; d != InvalidModel {
			t.Fatalf("knobs %+v: deadlock is %s", kn, d)
		}
	}
}

// The event of the state budget has the key of the record that could not be
// stored, so that every event with a later key is dropped: the budget stop is
// an event of the stream, not an effect applied after it (mutant: the budget
// event sorts last).
func TestParallelBudgetEventIsKeyedByTheRecordThatCouldNotBeStored(t *testing.T) {
	m := counters(4, 3)
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: Options{Budget: Budget{MaxStates: 2}}, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := newParRun(s, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.refreshDecided()
	w := r.workers[0]
	init := c.layout.Initial()
	w.store(parHash(init), init, parNoParent, 0)
	r.stored, w.added = 1, 0
	one := append([]byte(nil), init...)
	one[0]++
	if !w.storeCapped(parHash(one), one, 5, 3) { // the second state: allowed
		t.Fatal("the second state was refused under a budget of two")
	}
	two := append([]byte(nil), init...)
	two[1]++
	if w.storeCapped(parHash(two), two, 7, 9) {
		t.Fatal("a third state was stored under a budget of two")
	}
	ev := w.evs[evStateBudget]
	if ev == nil || ev.key != (parKey{u: 7, q: 9, slot: 1}) {
		t.Fatalf("budget event %+v, want the key of the record that could not be stored", ev)
	}
	// A state that is stored already is no new state and never counts.
	if !w.storeCapped(parHash(one), one, 8, 1) {
		t.Fatal("a state stored already was refused")
	}
}
