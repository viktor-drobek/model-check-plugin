package explore

import (
	"fmt"
	"runtime/debug"
	"slices"
	"sync/atomic"

	"modelcheck/cex"
	"modelcheck/ir"
)

// Parallel exploration of the safety search (performance plan 5).
//
// The search is a level-synchronous breadth-first search over a partitioned
// visited set (parvisited.go). The frontier of a level is the list of the
// ranges of states that the previous level appended to the partitions, in
// ascending partition order, so the frontier order is the ascending state id
// and needs no memory of its own. The frontier is consumed in groups of a
// bounded number of states, and every group goes through two phases with a
// barrier between them:
//
//  1. expand: the workers take segments of the group (a few hundred states)
//     and expand each state exactly as the sequential breadth-first search
//     does, calling the same nextEnabled and fire; for every successor that is
//     a stored state they append a record (hash, parent id, step counter,
//     vector) to the segment's buffer, then order the segment's records by
//     partition. Nothing is written to the set.
//  2. insert: the workers take partitions. The owner of a partition walks the
//     segments of the group in frontier order and, in each, the records of its
//     partition in expansion order, and stores what is new; the checks of the
//     properties run on every new state. Only the owner touches a partition in
//     this phase.
//
// So the arrival order within a partition, hence every id, every parent and
// the next frontier, is a function of the model alone: no quantity that a
// worker count, a segment size or a clock can change reaches a result. A group
// smaller than a threshold is expanded and inserted at once on the calling
// goroutine, state by state, in global record order, which gives the same ids
// because the record order restricted to one partition is the same.
//
// What the sequential search does as a side effect (decide a property, fail,
// hit a budget) is an event with a total key (parallel_events.go); at the end of
// a group the events are applied in key order through the code the sequential
// search uses. A counterexample is rebuilt after the fact from one parent id
// per state (parallel_trace.go).
//
// # Budgets
//
// The state budget is exact: while the stored states plus the records of a
// group cannot cross it the group goes through the parallel phases, and
// otherwise its records are inserted in record order on one goroutine, the
// first new state that cannot be stored is an event of the stream and ends the
// insertion, so the run stores the budget and not one state more and claims no
// verdict that lies after the stop point. The depth budget cuts at a layer: the
// layers up to the budget are expanded, the next one is stored (and checked)
// and not expanded. A layer counts hops between stored states, so an atomic
// sequence that runs through is one unit of depth and of the depth budget, and
// one that blocks part-way counts one unit per uninterrupted run (the state
// where its holder blocks is stored, and the continuation starts with the step
// of whichever process unblocks it), where the sequential breadth-first search
// counts each of its steps: on a model without atomic sequences the two cut at
// the same layer, and on one with them they may not.
// A d_step block is one move (fire runs its whole chain), so one unit in every
// search. The memory budget is
// checked, at the end of each group, on an estimate that is a function of the
// run (the stored set, the records of the largest group, the largest frontier).
// It is an estimate, not the resident memory: a run can overshoot the budget by
// up to about one group of records. A group whose records would not fit in the
// room the budget leaves is cut after the longest prefix of its states
// whose records do fit: the cut is a function of the counts of records alone,
// so the sequence of groups, and with it every stop point, is the same for any
// worker count, segment size or inline threshold. The time budget is looked at
// every few hundred states of an expansion and between the phases and groups;
// a group in flight when it expires is discarded whole, and the insertion phase
// of a group, once begun, runs to its end because the partitions are
// append-only.

const (
	// parSegmentStates is the number of states of a segment, the unit a worker
	// takes in the expansion phase. A scheduling unit: it enters no result.
	parSegmentStates = 256
	// parFirstGroupStates is the size of the first group of a run and
	// parMin/MaxGroupStates bound the later ones. A group is counted in states
	// and its size is computed from counts of the run alone, never from the
	// worker count or from timing, because everything tied to a group boundary
	// (budget stops, the discard of a group, the memory estimate) must be the
	// same for every worker count.
	parFirstGroupStates = 1024
	parMinGroupStates   = 256
	parMaxGroupStates   = 1 << 18
	// parGroupBytes is the target size of the records of a group.
	parGroupBytes = 16 << 20
	// parRecordCap is the most record bytes a group may hold.
	parRecordCap = 256 << 20
	// parInlineStates is the threshold below which a group is run inline, for one
	// worker; with more workers it is parInlineStates/workers, down to
	// parInlineFloor. Batching a group pays when enough workers share the
	// records, and costs a copy and a sort of the records that one worker, or a
	// narrow layer, does not make up for (measured: step 8 of the plan). It is a
	// scheduling choice and never changes a result.
	parInlineStates = 4096
	parInlineFloor  = 512
	// parRecordHeader is the size of a record's fixed part: the hash, the
	// parent id and the step counter.
	parRecordHeader = 16
	// parRangeBytes is one entry of the frontier (a partition and two bounds).
	parRangeBytes = 24
	// parCancelStates is how many states a worker expands between two looks at
	// the context.
	parCancelStates = 256
	// parFlushBytes is how many record bytes a worker accumulates before it adds
	// them to the group's shared total.
	parFlushBytes = 16 << 10
)

// MaxWorkers is the largest worker count Options.Workers accepts.
const MaxWorkers = 256

// Parallel is what a run records about the parallel search that Options.Workers
// asked for: whether it ran, and if not, why not.
type Parallel struct {
	// Requested is Options.Workers.
	Requested int `json:"requested_workers"`
	// Applied is true when the search ran in parallel; otherwise the run is
	// the sequential default, unchanged, and Reason says why.
	Applied bool `json:"applied"`
	// Workers is the number of workers that ran.
	Workers int `json:"workers,omitempty"`
	// Layers is the number of breadth-first layers that hold a stored state
	// and MaxLayerStates the size of the widest one: the shape of the graph,
	// which tells whether a parallel run could pay.
	Layers         int    `json:"layers,omitempty"`
	MaxLayerStates int    `json:"max_layer_states,omitempty"`
	Reason         string `json:"reason,omitempty"`
	// Note tells a reader what changes in the report of a parallel run.
	Note string `json:"note,omitempty"`
	// WorkerBytesEst is the memory of the compiled copies of the model and the
	// scratch of the workers, which the memory estimate of the run leaves out
	// because it depends on the worker count.
	WorkerBytesEst int64 `json:"worker_bytes_est,omitempty"`
}

// InternalError is a defect of the engine found while searching in parallel: a
// recovered panic of a worker, or a stored state that the re-derivation of a
// counterexample cannot reproduce. It is not the model's fault and not a
// rejection of the input: the run has no verdict.
type InternalError struct{ Msg string }

// Error is the message of the defect, prefixed with where it was found.
func (e *InternalError) Error() string { return "internal error in the parallel search: " + e.Msg }

// Reasons a parallel run is refused. The run is then the sequential default,
// unchanged, and Parallel.Reason carries the sentence.
const (
	parRefuseTemporal = "an ltl, progress or ctl property is decided by a nested depth-first search or a graph labelling that this version does not parallelise; the whole run was executed sequentially"
	parRefusePOR      = "partial-order reduction is depth-first only; with --por the reduced sequential search ran. Drop --por to search in parallel"
	parRefuseVisited  = "a caller-supplied visited set is for tests of the sequential search; the whole run was executed sequentially"
	parRefuseIdle     = "no property of the call was left to search for (every one of them was refused, see their reasons): nothing was searched"

	// parNote says what changes in the report of a parallel run.
	parNote = "breadth-first layers; depth, counterexample and stop point are those of the layered search (an atomic sequence that runs through is one unit of depth and of --budget-depth, one that blocks part-way one unit per uninterrupted run; --bfs counts every step), counters of a complete run equal the sequential ones"
)

// chooseParallel decides whether the run is the parallel search and records
// the decision in the result. It runs after the partial-order analysis, which
// reads the requested mode and so is not disturbed by Workers: the reduction
// wins where it applies, because it is depth-first.
//
// idle says that no search runs at all: every property of the call was refused
// (a property that reads the live-process table over a model that keeps none,
// tableread.go), so there is nothing to decide. The parallel search is then not
// applied, and the report says why instead of claiming workers that did no work.
func (s *search) chooseParallel(temporal, idle bool) bool {
	if s.opt.Workers <= 0 {
		return false
	}
	p := &Parallel{Requested: s.opt.Workers}
	s.res.Parallel = p
	switch {
	case temporal:
		p.Reason = parRefuseTemporal
	case s.por != nil:
		p.Reason = parRefusePOR
	case s.opt.NewVisited != nil:
		p.Reason = parRefuseVisited
	case idle:
		p.Reason = parRefuseIdle
	default:
		p.Applied = true
	}
	return p.Applied
}

// runParallel is the parallel search of a run chosen by chooseParallel: it
// leaves the counters, the stop reason and the shape of the graph in the
// result, for finish to turn into verdicts.
func (s *search) runParallel(watch []*ir.Expr) error {
	r, err := s.parallelBFS(s.opt.Workers, watch)
	if err != nil {
		return err
	}
	p := s.res.Parallel
	p.Workers, p.Layers, p.MaxLayerStates, p.Note, p.WorkerBytesEst = len(r.workers), r.layers, r.maxLayer, parNote, r.workerBytes()
	return nil
}

// parKnobs are controls for tests: the sizes that must not change a result,
// so that a test can force them to extremes. They are zero in production.
type parKnobs struct {
	segment int // states per segment; 0 = parSegmentStates
	group   int // states per group; 0 = adaptive
	inline  int // groups with fewer states run inline; 0 = parInlineStates, negative = never
	// cancelEvery is the number of states between two looks at the context; 0 =
	// parCancelStates.
	cancelEvery int
	// hook is called at the points of a group where a test wants to act: stage
	// "expand" (before the expansion phase of a batched group), "expand-mid" (by
	// a worker, after each segment), "between" (after the expansion phase,
	// before the clock is looked at), "insert" (before the insertion phase),
	// "insert-mid" (by a worker, per partition), "merge" (at the end of a group,
	// with the number of states stored) and "level" (at the end of a level). The
	// worker stages are called from several goroutines, with no count of states
	// (the set is being written then).
	hook func(stage string, states int)
}

func (k *parKnobs) segmentStates() int {
	if k != nil && k.segment > 0 {
		return k.segment
	}
	return parSegmentStates
}

func (k *parKnobs) cancelStates() int {
	if k != nil && k.cancelEvery > 0 {
		return k.cancelEvery
	}
	return parCancelStates
}

// inlineBelow is the number of states below which a group is run inline.
func (r *parRun) inlineBelow() int {
	if k := r.kn; k != nil && k.inline != 0 {
		if k.inline < 0 {
			return 0
		}
		return k.inline
	}
	return min(max(parInlineStates/len(r.workers), parInlineFloor), parInlineStates)
}

// parRange is a run of consecutive states of one partition.
type parRange struct{ part, lo, hi int }

// parSeg is a segment of a group: a run of states of one range and the records
// their expansion produced, ordered by partition (in expansion order within a
// partition) so that the owner of a partition reads its records in one piece.
// The same type holds the records of the one state an inline group expands at a
// time, in expansion order (buf, n).
type parSeg struct {
	part, lo, hi int
	sorted       []byte                    // the records, partition by partition
	start        [parPartitions + 1]uint32 // the records of partition p are start[p] to start[p+1]
	cnt          parCounts
	buf          []byte // inline group: the records of one state, in expansion order
	n            int    // inline group: how many
}

// parCounts are the counters of the expansions of one segment (or of one
// inline group).
type parCounts struct {
	trans, atomic, recs int64
}

// parNode is an intermediate state of an atomic sequence being expanded
// depth-first inside the expansion of one stored state.
type parNode struct {
	state []byte
	f     frame
}

// parWorker is one worker: its own compiled copy of the model (only
// ir.Layout.Timeout is mutable, and the copy keeps it private), its scratch,
// and what it produced in the current phase.
type parWorker struct {
	r       *parRun
	s       *search // a shell: the compiled copy and the cur/next buffers
	nodes   []parNode
	refs    []cex.Ref
	touched []uint8 // partitions that received their first state of the level
	inl     parCounts
	evs     []*parEvent // the retained event of each class, nil if none
	watched []*ir.Compiled
	cov     [][2]bool // per watch expression: ever true, ever false
	added   int       // states this worker stored since the last group boundary
	dirty   []uint8   // partitions this worker stored a state in during the group
	nEv     int       // how many of evs are set

	mode   parMode
	cnt    *parCounts
	target []byte
	found  []cex.Ref
	ticks  int // states expanded since the last look at the context

	// An inline group expands one state at a time into pend before it inserts
	// anything, so that a state whose records do not fit can be left out whole.
	pend parSeg
	// A worker expands a segment into stage, in expansion order, counting the
	// records per partition in hist, and then copies them partition by partition
	// into the segment.
	stage  []byte
	stageN int
	hist   [parPartitions + 1]uint32
	unsent int64 // record bytes written and not yet added to the group's shared total
	room   int64 // the record bytes still free in the group
	over   bool  // the records of the state do not fit in room
	skip   bool  // the state budget was crossed: the records of the rest of the group are not inserted
}

// parRun is one parallel search.
type parRun struct {
	s        *search
	kn       *parKnobs
	set      *partSet
	workers  []*parWorker
	stateLen int
	recBytes int
	recAcct  int64 // the bytes a record counts for in the estimate (the record itself)

	maxStates int   // the state budget, 0 = none
	maxMem    int64 // the memory budget, 0 = none
	maxDepthB int   // the depth budget, 0 = none

	frontier   []parRange
	fbuf       [2][]parRange // the frontier being consumed and the one being built
	fcur       int           // which of them is the frontier being consumed
	gbuf       []parRange    // the ranges of the group being run
	partBuf    []uint8
	nActive    int // workers that may have something to report from the group just run (1 for an inline group)
	levelWork  int // workers that stored a state in this level
	layer      int
	layers     int
	maxLayer   int
	layerSizes []int
	doneLayers int // layers whose expansion ran to the end
	segs       []*parSeg

	// The state of the properties at the start of the group, which the
	// workers read: decided properties are not evaluated again.
	decided      []bool
	openAssert   bool
	openDeadlock bool
	checking     bool

	pool *parPool
	// cancelled is set by a worker that sees the context expire while it
	// expands; the group is then discarded. overflow is set when the records of
	// a batched group, which the workers count as they write them, pass rcap.
	//
	// The workers add the bytes of their records to usedRecs in batches (parFlushBytes
	// and at the end of a segment), not record by record: one shared counter
	// written for every record is a cache line that every core fights for. The
	// batches add up to the same total, so whether a group overflows is still a
	// function of its total record bytes alone. The fields are padded apart so
	// that the counter's writes do not invalidate the line the flags are read from.
	_         [64]byte
	cancelled atomic.Bool
	overflow  atomic.Bool
	_         [64]byte
	usedRecs  atomic.Int64
	_         [64]byte
	rcap      int64

	stored        int // states stored at the last group boundary
	trans, atomic int64
	maxDepth      int
	prevStates    int64 // states and records of the last group, for the next group size
	prevRecs      int64
	maxGroupBytes int64 // the largest record bytes of a group so far
	maxFrontBytes int64 // the largest frontier so far
}

// hook calls the test hook, if there is one, from the coordinating goroutine
// between two barriers, with the number of states stored.
func (r *parRun) hook(stage string) {
	if r.kn != nil && r.kn.hook != nil {
		r.kn.hook(stage, r.set.Len())
	}
}

// workerHook calls the test hook from a worker, during a phase, when the set is
// being written or read by others: it is given no count of states.
func (r *parRun) workerHook(stage string) {
	if r.kn != nil && r.kn.hook != nil {
		r.kn.hook(stage, 0)
	}
}

func newParRun(s *search, workers int, watch []*ir.Expr) (*parRun, error) {
	if workers < 1 {
		workers = 1
	}
	r := &parRun{s: s, kn: s.opt.par, stateLen: s.c.layout.Size}
	r.recBytes = parRecordHeader + r.stateLen
	r.recAcct = int64(r.recBytes)
	r.maxStates, r.maxMem, r.maxDepthB = s.opt.Budget.MaxStates, s.opt.Budget.MaxMemBytes, s.opt.Budget.MaxDepth
	r.set = newPartSet(r.stateLen)
	for i := 0; i < workers; i++ {
		c := s.c
		if i > 0 {
			var err error
			if c, err = compile(s.c.m); err != nil {
				return nil, err
			}
		}
		w := &parWorker{r: r, s: &search{c: c, cur: make([]byte, r.stateLen), next: make([]byte, r.stateLen)}}
		w.evs = make([]*parEvent, int(nParEvKinds)+2*len(c.props))
		for _, e := range watch {
			ce, err := c.layout.Compile(e, -1)
			if err != nil {
				return nil, err
			}
			w.watched = append(w.watched, ce)
		}
		w.cov = make([][2]bool, len(watch))
		r.workers = append(r.workers, w)
	}
	return r, nil
}

// parallelBFS runs the parallel search on s with the given number of workers
// and leaves the counters in s.res and the stop reason in s.stop; the caller
// finishes the run as it does a sequential one. watch are the expressions
// whose truth is recorded over the stored states (Options.Watch).
func (s *search) parallelBFS(workers int, watch []*ir.Expr) (*parRun, error) {
	r, err := newParRun(s, workers, watch)
	if err != nil {
		return nil, err
	}
	r.startPool()
	defer r.closePool()
	if err := r.safely(r.run); err != nil {
		return nil, err
	}
	r.finalize()
	return r, nil
}

// safely runs fn, which is the work of the coordinating goroutine (a panic in a
// worker is recovered where it happens, in runJob), and turns a panic of it into
// an InternalError: a defect of the engine must end the call with an error and
// no verdict, not take the process down.
func (r *parRun) safely(fn func() error) (err error) {
	defer func() {
		if x := recover(); x != nil {
			err = &InternalError{Msg: fmt.Sprintf("the coordinator panicked: %v\n%s", x, debug.Stack())}
		}
	}()
	return fn()
}

func (r *parRun) run() error {
	s := r.s
	init := s.c.layout.Initial()
	w0 := r.workers[0]
	r.set.epoch = 1
	r.refreshDecided()
	// Step 0: the initial state and its checks, which belong to no expansion
	// and come first (explore.go: bfs does the same with checkState).
	w0.store(parHash(init), init, parNoParent, 0)
	r.nActive, r.levelWork = 1, 1
	r.accountGroup()
	if err := r.mergeEvents(); err != nil {
		return err
	}
	r.frontier = r.takeFrontier()
	for len(r.frontier) > 0 && s.stop == "" {
		size := 0
		for _, rg := range r.frontier {
			size += rg.hi - rg.lo
		}
		r.layers++
		r.layerSizes = append(r.layerSizes, size)
		if size > r.maxLayer {
			r.maxLayer = size
		}
		if f := int64(len(r.frontier)) * parRangeBytes; f > r.maxFrontBytes {
			r.maxFrontBytes = f
		}
		if r.maxDepthB > 0 && r.layer > r.maxDepthB {
			// Stored, and checked when they were stored, and not expanded: as
			// the sequential search leaves the states deeper than the budget. The
			// layer is a count of hops between stored states, so on a model with
			// atomic sequences this is not the layer the sequential breadth-first
			// search cuts at (it counts every step of an atomic sequence).
			s.truncated = size
			break
		}
		r.set.epoch++
		r.levelWork = 1
		cur := parCursor{}
		for !cur.done(r.frontier) && s.stop == "" {
			consumed, err := r.runGroup(r.takeGroup(&cur, r.groupStates()))
			if err != nil {
				return err
			}
			if consumed == 0 && s.stop == "" {
				// A group that takes no state and does not stop the run would
				// repeat for ever: it can only be a defect.
				return &InternalError{Msg: "a group consumed no state and did not stop the run"}
			}
			cur.advance(r.frontier, consumed)
		}
		if s.stop == "" {
			r.doneLayers++
		}
		r.frontier = r.takeFrontier()
		r.layer++
		r.hook("level")
	}
	if s.stop == "" {
		s.stop = "complete"
	}
	return nil
}

// takeFrontier builds the frontier of the next level from the partitions that
// the workers recorded as touched, in ascending partition order. The buffers of
// the frontier are reused, so a model with a million levels allocates nothing
// per level.
func (r *parRun) takeFrontier() []parRange {
	parts := r.partBuf[:0]
	for _, w := range r.workers[:r.levelWork] {
		parts = append(parts, w.touched...)
		w.touched = w.touched[:0]
	}
	slices.Sort(parts)
	r.partBuf = parts
	// The frontier being consumed is in fbuf[fcur]; build the next one in the
	// other buffer.
	next := 1 - r.fcur
	out := r.fbuf[next][:0]
	for _, p := range parts {
		pt := r.set.parts[p]
		if hi := pt.n; int(pt.levelLo) < hi {
			out = append(out, parRange{int(p), int(pt.levelLo), hi})
		}
	}
	r.fbuf[next], r.fcur = out, next
	return out
}

// parCursor is a position in the frontier: a range and an offset into it.
type parCursor struct{ idx, off int }

func (c *parCursor) done(f []parRange) bool { return c.idx >= len(f) }

// advance moves the cursor past n states.
func (c *parCursor) advance(f []parRange, n int) {
	for n > 0 && c.idx < len(f) {
		left := f[c.idx].hi - f[c.idx].lo - c.off
		if n < left {
			c.off += n
			return
		}
		n -= left
		c.idx++
		c.off = 0
	}
}

// takeGroup returns the next states of the frontier, at most limit of them, as
// ranges. It does not move the cursor: a group may take fewer states than it is
// offered (see runGroup).
func (r *parRun) takeGroup(c *parCursor, limit int) []parRange {
	out := r.gbuf[:0]
	idx, off := c.idx, c.off
	for limit > 0 && idx < len(r.frontier) {
		rg := r.frontier[idx]
		lo := rg.lo + off
		n := min(rg.hi-lo, limit)
		out = append(out, parRange{rg.part, lo, lo + n})
		limit -= n
		off += n
		if lo+n == rg.hi {
			idx++
			off = 0
		}
	}
	r.gbuf = out
	return out
}

// prefix returns the first n states of ranges.
func prefix(ranges []parRange, n int) []parRange {
	var out []parRange
	for _, rg := range ranges {
		if n <= 0 {
			break
		}
		k := min(rg.hi-rg.lo, n)
		out = append(out, parRange{rg.part, rg.lo, rg.lo + k})
		n -= k
	}
	return out
}

func countStates(ranges []parRange) int {
	n := 0
	for _, rg := range ranges {
		n += rg.hi - rg.lo
	}
	return n
}

// groupStates is the size of the next group: a function of the counts of the
// previous group alone.
func (r *parRun) groupStates() int {
	if g := r.kn; g != nil && g.group > 0 {
		return g.group
	}
	if r.prevStates == 0 {
		return parFirstGroupStates
	}
	if r.prevRecs == 0 {
		return parMaxGroupStates
	}
	g := int64(parGroupBytes) * r.prevStates / (r.prevRecs * r.recAcct)
	return int(min(max(g, parMinGroupStates), parMaxGroupStates))
}

// estimate is memory_bytes_est: the stored set, the records of the largest
// group and the largest frontier, every term a count times a constant, so it is
// a function of the run and not of the worker count or the capacity the runtime
// gave a slice. The compiled copies of the model and the scratch of the workers
// are not in it (Parallel.WorkerBytesEst has them).
func (r *parRun) estimate() int64 {
	return r.set.accounted() + r.maxGroupBytes + r.maxFrontBytes
}

// recordCap is the record bytes the next group may hold: the cap on a group and
// the room the memory budget leaves beside the stored set and the frontier.
func (r *parRun) recordCap() int64 {
	room := int64(parRecordCap)
	if r.maxMem > 0 {
		room = min(room, max(r.maxMem-r.set.accounted()-r.maxFrontBytes, 0))
	}
	return room
}

// groupStatus is how an attempt to run a group ended.
type groupStatus uint8

const (
	groupOK        groupStatus = iota
	groupOverflow              // the records of the group do not fit in the room
	groupDiscarded             // the clock ran out and the group was thrown away
	groupTail                  // the state budget can be crossed in this group: it is to be run inline
)

// runGroup expands and inserts the states of a group, then applies its events
// and looks at the memory budget and the clock. It returns how many of the
// states it was offered it took: all of them, unless their records would not
// fit under the cap, in which case it takes the longest prefix whose records do
// (the cut depends on counts of records alone, so it is the same for every
// worker count, segment size and inline threshold).
func (r *parRun) runGroup(ranges []parRange) (int, error) {
	r.refreshDecided()
	rcap := r.recordCap()
	forceInline := false
	for {
		states := countStates(ranges)
		var cnt parCounts
		var status groupStatus
		var err error
		consumed := states
		if forceInline || states < r.inlineBelow() {
			cnt, consumed, status = r.runInline(ranges, rcap)
		} else {
			cnt, status, err = r.runBatched(ranges, rcap)
		}
		if err != nil {
			return 0, err
		}
		switch status {
		case groupTail:
			// The group in which the state budget can be crossed is inserted in
			// global record order on one goroutine, which is what an inline group
			// does, so it is run again as one (the expansion is the same, so the
			// counters are).
			forceInline = true
			continue
		case groupDiscarded:
			r.s.budget("time budget exhausted")
			return 0, nil
		case groupOverflow:
			k, first := r.countPrefix(ranges, rcap)
			if k == 0 {
				r.s.budget(fmt.Sprintf("memory budget exhausted: the expansion of one state needs %d bytes of records", first))
				return 0, nil
			}
			ranges = prefix(ranges, k)
			continue
		}
		r.endGroup(cnt, consumed)
		return consumed, r.afterGroup()
	}
}

// endGroup books the counters of a group that went through.
func (r *parRun) endGroup(cnt parCounts, states int) {
	r.layerExpanded()
	r.trans += cnt.trans
	r.atomic += cnt.atomic
	r.prevStates, r.prevRecs = int64(states), cnt.recs
	if b := cnt.recs * r.recAcct; b > r.maxGroupBytes {
		r.maxGroupBytes = b
	}
	r.accountGroup()
}

// accountGroup books what the group stored: the count of states, and the bytes
// of the partitions it touched.
func (r *parRun) accountGroup() {
	for _, w := range r.workers[:r.nActive] {
		r.stored += w.added
		w.added = 0
		r.set.accountGroup(w.dirty)
		w.dirty = w.dirty[:0]
	}
	r.set.groupEpoch++
}

// afterGroup applies the events of the group, then looks at the clock and at
// the memory budget.
func (r *parRun) afterGroup() error {
	if err := r.mergeEvents(); err != nil {
		return err
	}
	r.hook("merge")
	if r.s.stop != "" {
		return nil
	}
	if r.s.ctx.Err() != nil {
		r.s.budget("time budget exhausted")
		return nil
	}
	if est := r.estimate(); r.maxMem > 0 && est > r.maxMem {
		r.s.budget(fmt.Sprintf("memory budget exhausted: estimate %d bytes exceeds %d", est, r.maxMem))
	}
	return nil
}

// layerExpanded records that the current layer has had a group expanded: the
// depth of the run is the greatest layer whose expansion has run. A layer counts
// stored-state hops, so an atomic sequence that runs through adds one to the
// depth (one that blocks part-way adds one per uninterrupted run), and the
// sequential --bfs adds its length (a d_step block is one move in both).
func (r *parRun) layerExpanded() {
	if r.layer > r.maxDepth {
		r.maxDepth = r.layer
	}
}

// runInline expands the group on the calling goroutine, one state at a time,
// and inserts the successors of each in global record order. A state whose
// records would push the group over the cap is left out with the rest (the
// group is the longest prefix that fits); if not even the first fits the status
// is groupOverflow.
func (r *parRun) runInline(ranges []parRange, rcap int64) (parCounts, int, groupStatus) {
	w := r.workers[0]
	r.nActive = 1
	w.inl = parCounts{}
	w.skip = false
	var used int64
	consumed := 0
	for _, rg := range ranges {
		for i := rg.lo; i < rg.hi; i++ {
			u := uint32(rg.part)<<parIndexBits | uint32(i)
			before := w.inl
			p := &w.pend
			p.buf, p.n = p.buf[:0], 0
			w.mode, w.cnt, w.room, w.over = pmPend, &w.inl, rcap-used, false
			w.walk(u)
			if w.over {
				w.inl = before
				w.dropEvents(u)
				if consumed == 0 {
					return parCounts{}, 0, groupOverflow
				}
				return w.inl, consumed, groupOK
			}
			used += int64(p.n) * r.recAcct
			if !w.skip {
				for rn := 0; rn < p.n; rn++ {
					rec := p.buf[rn*r.recBytes : (rn+1)*r.recBytes]
					if !w.storeCapped(leU64(rec), rec[parRecordHeader:], leU32(rec[8:]), leU32(rec[12:])) {
						w.skip = true
						break
					}
				}
			}
			consumed++
		}
	}
	return w.inl, consumed, groupOK
}

// runBatched expands the group's segments in parallel, then inserts the records
// by partition in parallel. The group is discarded whole (records, events and
// counts) when the clock ran out during or right after the expansion, because
// nothing already written to a partition can be withdrawn and the insertion
// phase, once begun, always runs to the end; and it overflows when its records
// pass the cap, which the workers count as they write them.
func (r *parRun) runBatched(ranges []parRange, rcap int64) (total parCounts, status groupStatus, err error) {
	segs := r.makeSegs(ranges)
	r.nActive, r.levelWork = len(r.workers), len(r.workers)
	r.cancelled.Store(false)
	r.overflow.Store(false)
	r.usedRecs.Store(0)
	r.rcap = rcap
	r.hook("expand")
	var next atomic.Int64
	r.set.reading = true
	err = r.phase(func(w *parWorker) {
		for !r.cancelled.Load() && !r.overflow.Load() {
			j := int(next.Add(1)) - 1
			if j >= len(segs) {
				return
			}
			w.expandSegment(segs[j])
			r.workerHook("expand-mid")
		}
	})
	r.set.reading = false
	if err != nil {
		return total, groupOK, err
	}
	r.hook("between")
	if r.cancelled.Load() || r.s.ctx.Err() != nil {
		r.forgetEvents()
		return total, groupDiscarded, nil
	}
	if r.overflow.Load() {
		r.forgetEvents()
		return total, groupOverflow, nil
	}
	for _, sg := range segs {
		total.trans += sg.cnt.trans
		total.atomic += sg.cnt.atomic
		total.recs += sg.cnt.recs
	}
	if r.maxStates > 0 && int64(r.stored)+total.recs > int64(r.maxStates) {
		r.forgetEvents()
		return total, groupTail, nil
	}
	r.hook("insert")
	var parts []int
	for p := 0; p < parPartitions; p++ {
		for _, sg := range segs {
			if sg.start[p+1] > sg.start[p] {
				parts = append(parts, p)
				break
			}
		}
	}
	next.Store(0)
	err = r.phase(func(w *parWorker) {
		for {
			k := int(next.Add(1)) - 1
			if k >= len(parts) {
				return
			}
			w.insertPartition(parts[k], segs)
			r.workerHook("insert-mid")
		}
	})
	return total, groupOK, err
}

// forgetEvents drops the events of a group that is being thrown away.
func (r *parRun) forgetEvents() {
	for _, w := range r.workers[:r.nActive] {
		clear(w.evs)
		w.nEv = 0
	}
}

// insertPartition is the insertion phase for one partition: the records of the
// group that belong to it, in frontier order and, within a segment, in
// expansion order. Only the owner of the partition runs it, and it never looks
// at the clock: the table, the arena and the parent array of a partition are
// append-only, so a half-inserted group cannot be taken back.
func (w *parWorker) insertPartition(p int, segs []*parSeg) {
	R := w.r.recBytes
	for _, sg := range segs {
		recs := sg.sorted[int(sg.start[p])*R : int(sg.start[p+1])*R]
		for off := 0; off < len(recs); off += R {
			rec := recs[off : off+R]
			w.store(leU64(rec), rec[parRecordHeader:], leU32(rec[8:]), leU32(rec[12:]))
		}
	}
}

// countPrefix is the longest prefix of the states of ranges whose records, in
// the order of the group, fit in rcap, and, when not even the first does, the
// bytes of the first state's records. It expands the states one after the
// other and counts, so it costs a sequential expansion of the group; it runs
// only after a group overflowed.
func (r *parRun) countPrefix(ranges []parRange, rcap int64) (int, int64) {
	w := r.workers[0]
	var used int64
	k := 0
	for _, rg := range ranges {
		for i := rg.lo; i < rg.hi; i++ {
			var c parCounts
			w.mode, w.cnt = pmCount, &c
			w.walk(uint32(rg.part)<<parIndexBits | uint32(i))
			b := c.recs * r.recAcct
			if used+b > rcap {
				if k == 0 {
					return 0, b
				}
				return k, 0
			}
			used += b
			k++
		}
	}
	return k, 0
}

// makeSegs cuts the ranges of a group into segments, reusing the segment
// buffers of earlier groups.
func (r *parRun) makeSegs(ranges []parRange) []*parSeg {
	S := r.kn.segmentStates()
	n := 0
	for _, rg := range ranges {
		for lo := rg.lo; lo < rg.hi; lo += S {
			if n == len(r.segs) {
				r.segs = append(r.segs, &parSeg{})
			}
			sg := r.segs[n]
			sg.part, sg.lo, sg.hi = rg.part, lo, min(lo+S, rg.hi)
			n++
		}
	}
	return r.segs[:n]
}

// workerBytes estimates what the workers hold that the memory estimate of the
// run leaves out because it depends on their number: for each, a compiled copy
// of the model, the scratch states and the retained events, and the buffers of
// the segments.
func (r *parRun) workerBytes() int64 {
	const edgeBytes, procBytes, propBytes, baseBytes = 512, 256, 128, 4096
	c := r.s.c
	model := int64(baseBytes + len(c.procs)*procBytes + len(c.props)*propBytes)
	for _, p := range c.procs {
		model += int64(len(p.edge)) * edgeBytes
	}
	per := model + int64(2*r.stateLen) + int64(len(r.workers[0].evs))*8
	total := int64(len(r.workers)) * per
	for _, w := range r.workers {
		total += int64(cap(w.stage)) + int64(cap(w.pend.buf)) + int64(len(w.hist))*4
	}
	for _, sg := range r.segs {
		total += int64(cap(sg.sorted)) + int64(len(sg.start))*4
	}
	return total
}

// finalize copies the counters of the run into the result.
func (r *parRun) finalize() {
	res := r.s.res
	res.States = r.set.Len()
	res.Transitions = int(r.trans)
	res.AtomicSteps = int(r.atomic)
	res.MaxDepth = r.maxDepth
	res.MemBytes = r.estimate()
	// The depths the search finished: all of them when it went to the end (or to
	// the depth budget, which stores the layer beyond it whole), else those below
	// the first layer whose expansion did not run to the end.
	last := len(r.layerSizes) - 1
	if r.s.stop != "complete" {
		last = r.doneLayers - 1
	}
	total := 0
	for d := 0; d <= last && d < len(r.layerSizes); d++ {
		total += r.layerSizes[d]
		res.Levels = append(res.Levels, Level{Depth: d, States: total})
	}
	for i := range r.s.watch {
		for _, w := range r.workers {
			if w.cov[i][0] {
				r.s.watch[i].EverTrue = true
			}
			if w.cov[i][1] {
				r.s.watch[i].EverFalse = true
			}
		}
	}
}
