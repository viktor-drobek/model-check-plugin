package explore

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"modelcheck/cex"
)

// The worker side of the parallel search: expanding a stored state exactly as
// the sequential breadth-first search does, and what is done with the stored
// successors it finds.

// parMode is what the walk over a state's successors does with a stored
// successor.
type parMode uint8

const (
	pmRecord parMode = iota // append a record to the segment (a batched group)
	pmPend                  // append it to the pending buffer of one state (an inline group)
	pmFind                  // look for the successor equal to a target (counterexamples)
	pmCount                 // count it (the size of a state's records)
)

// quiet: the walk produces no event and no counter, because it only looks.
func (m parMode) quiet() bool { return m == pmFind || m == pmCount }

// insert stores a state, whose hash is h and whose expansion is that of parent,
// in its partition. It runs on the owner of the partition, or on the calling
// goroutine for an inline group, and records the first touch of a partition in
// a level.
func (w *parWorker) insert(h uint64, vec []byte, parent uint32) (uint32, parResult) {
	set := w.r.set
	pi, pt := set.part(h)
	if pt.epoch != set.epoch {
		pt.epoch = set.epoch
		pt.levelLo = int32(pt.n)
		w.touched = append(w.touched, uint8(pi))
	}
	if pt.dirty != set.groupEpoch {
		pt.dirty = set.groupEpoch
		w.dirty = append(w.dirty, uint8(pi))
	}
	return set.addIn(pi, pt, h, vec, parent)
}

// store inserts a successor produced at step q of the expansion of parent and,
// when it is new, runs the checks of the properties on it.
func (w *parWorker) store(h uint64, vec []byte, parent, q uint32) {
	id, res := w.insert(h, vec, parent)
	switch res {
	case parAdded:
		w.added++
		w.checkNew(id, vec, parent, q)
	case parFull:
		key := parKey{u: parent, q: q, slot: 1}
		if w.wants(int(evCapacity), key) {
			w.keep(&parEvent{key: key, kind: evCapacity, text: fmt.Sprintf("state budget exhausted: a partition of the visited set holds %d states, the most it can number", w.r.set.limit)})
		}
	}
}

// storeCapped is store under the state budget, which only the single-threaded
// paths (an inline group, and the group in which the budget can be crossed)
// use: a new state is stored only while fewer than the budget are, and the
// first new state that cannot be is an event of the stream (so every event
// after it is dropped) and ends the insertion of the group. A state that is
// stored already is no new state and never counts. It reports whether the
// insertion goes on.
func (w *parWorker) storeCapped(h uint64, vec []byte, parent, q uint32) bool {
	r := w.r
	if r.maxStates > 0 && r.stored+w.added >= r.maxStates {
		if _, present := r.set.has(h, vec); present {
			return true
		}
		key := parKey{u: parent, q: q, slot: 1}
		if w.wants(int(evStateBudget), key) {
			w.keep(&parEvent{key: key, kind: evStateBudget, text: fmt.Sprintf("state budget exhausted: %d states stored", r.stored+w.added)})
		}
		return false
	}
	w.store(h, vec, parent, q)
	return true
}

// dropEvents forgets the events the walk of state u kept: the state is not
// part of this group after all.
func (w *parWorker) dropEvents(u uint32) {
	for i, e := range w.evs {
		if e != nil && e.key.u == u {
			w.evs[i] = nil
			w.nEv--
		}
	}
}

// stopped says whether the expansion should be abandoned: the context expired
// (the worker looks every so many states, and any worker that sees it says so
// to the others) or the records of the group no longer fit. Only the expansion
// phase looks: the walk of an inline group and the re-derivation of a
// counterexample do not.
func (w *parWorker) stopped() bool {
	r := w.r
	if w.ticks++; w.ticks >= r.kn.cancelStates() {
		w.ticks = 0
		if r.s.ctx.Err() != nil {
			r.cancelled.Store(true)
		}
	}
	return r.cancelled.Load() || r.overflow.Load()
}

// expandSegment expands the states of a segment into the worker's staging
// buffer, in expansion order, and copies the records partition by partition
// into the segment (a counting sort: the records of one partition are then in
// one piece for the worker that owns it in the insertion phase).
func (w *parWorker) expandSegment(sg *parSeg) {
	sg.cnt = parCounts{}
	w.stage, w.stageN, w.hist = w.stage[:0], 0, [parPartitions + 1]uint32{}
	w.mode, w.cnt = pmRecord, &sg.cnt
	w.unsent = 0
	for i := sg.lo; i < sg.hi; i++ {
		if w.stopped() {
			return
		}
		w.walk(uint32(sg.part)<<parIndexBits | uint32(i))
	}
	w.flushRecords()
	for k := 0; k < parPartitions; k++ {
		w.hist[k+1] += w.hist[k]
	}
	sg.start = w.hist
	R := w.r.recBytes
	if need := w.stageN * R; cap(sg.sorted) < need {
		sg.sorted = make([]byte, need)
	} else {
		sg.sorted = sg.sorted[:need]
	}
	var pos [parPartitions]uint32
	copy(pos[:], w.hist[:parPartitions])
	for off := 0; off < len(w.stage); off += R {
		rec := w.stage[off : off+R]
		p := rec[7] // the top byte of the little-endian hash is the partition
		copy(sg.sorted[int(pos[p])*R:], rec)
		pos[p]++
	}
}

// walk expands stored state u as the sequential breadth-first search does:
// every enabled move is fired, a successor inside an atomic sequence is
// expanded in turn (depth-first) and every other successor is a stored state,
// which the mode of the walk turns into a record, a pending record, a
// comparison or a count. The events of the expansion are kept in the order of
// the sequential code's own steps (see parKey).
func (w *parWorker) walk(u uint32) {
	r := w.r
	s := w.s
	quiet := w.mode.quiet()
	if len(w.nodes) == 0 {
		w.nodes = append(w.nodes, parNode{state: make([]byte, r.stateLen)})
	}
	nodes := w.nodes
	defer func() { w.nodes = nodes }()
	depth := 1
	copy(nodes[0].state, r.set.get(u))
	nodes[0].f = frame{proc: -1, rv: -1}
	w.refs = w.refs[:0]
	var q uint32
	for depth > 0 {
		n := &nodes[depth-1]
		copy(s.cur, n.state)
		q++
		m, ok, err := s.nextEnabled(&n.f, s.cur)
		if err != nil {
			if !quiet {
				w.errorEvent(u, q, err, m, false, nil)
			}
			return
		}
		if !ok {
			if depth == 1 && n.f.enabled == 0 && !quiet && r.openDeadlock && !s.allTerminated(s.cur) {
				if key := (parKey{u: u, q: parQEnd}); w.wants(int(evDeadlock), key) {
					w.keep(&parEvent{key: key, kind: evDeadlock, from: u})
				}
			}
			depth--
			if depth > 0 {
				w.refs = w.refs[:depth-1]
			}
			continue
		}
		n.f.enabled++
		w.cnt.trans++
		q++
		failed, err := s.fire(m)
		if err != nil {
			if !quiet {
				w.errorEvent(u, q, err, m, true, s.next)
			}
			return
		}
		if failed != nil && !quiet && r.openAssert {
			if key := (parKey{u: u, q: q}); w.wants(int(evAssert), key) {
				w.keep(&parEvent{key: key, kind: evAssert, from: u, chain: w.copyChain(m, true), final: append([]byte(nil), s.next...), text: failed.assert.String()})
			}
		}
		q++
		inter, err := s.intermediate(s.next)
		if err != nil {
			if !quiet {
				w.errorEvent(u, q, err, m, true, s.next)
			}
			return
		}
		if inter {
			w.cnt.atomic++
			if depth > dstepLimit {
				if key := (parKey{u: u, q: q}); !quiet && w.wants(int(evAtomic), key) {
					w.keep(&parEvent{key: key, kind: evAtomic})
				}
				return
			}
			if w.mode == pmRecord && depth&255 == 255 && w.stopped() {
				return // the clock ran out, or the records no longer fit, inside a long atomic sequence
			}
			if depth == len(nodes) {
				nodes = append(nodes, parNode{state: make([]byte, r.stateLen)})
			}
			copy(nodes[depth].state, s.next)
			nodes[depth].f = frame{proc: -1, rv: -1}
			depth++
			w.refs = append(w.refs, s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
			continue
		}
		switch w.mode {
		case pmFind:
			if bytes.Equal(s.next, w.target) {
				w.found = w.copyChain(m, true)
				return
			}
		case pmCount:
			w.cnt.recs++
		default:
			w.cnt.recs++
			if !w.appendRecord(parHash(s.next), u, q) {
				return // the records of the state do not fit
			}
		}
	}
}

// appendRecord appends a record for the successor in s.next: to the staging
// buffer of the segment being expanded (a batched group) or to the pending
// buffer of the state being expanded (an inline group). It reports false when
// the record does not fit under the cap on the records of a group: for an
// inline group the cap is the room left in the group, for a batched one the
// group total, shared by the workers, which each add what they write before they
// write it.
func (w *parWorker) appendRecord(h uint64, u, q uint32) bool {
	r := w.r
	R := r.recBytes
	var buf *[]byte
	if w.mode == pmPend {
		if int64(w.pend.n+1)*r.recAcct > w.room {
			w.over = true
			return false
		}
		buf = &w.pend.buf
		w.pend.n++
	} else {
		if w.unsent += r.recAcct; w.unsent >= parFlushBytes {
			w.flushRecords()
		}
		buf = &w.stage
		w.hist[(h>>56)+1]++
		w.stageN++
	}
	n0 := len(*buf)
	if n0+R > cap(*buf) {
		grown := make([]byte, n0, max(2*cap(*buf), n0+64*R))
		copy(grown, *buf)
		*buf = grown
	}
	*buf = (*buf)[:n0+R]
	rec := (*buf)[n0:]
	binary.LittleEndian.PutUint64(rec, h)
	binary.LittleEndian.PutUint32(rec[8:], u)
	binary.LittleEndian.PutUint32(rec[12:], q)
	copy(rec[parRecordHeader:], w.s.next)
	return true
}

// flushRecords adds the record bytes this worker has written since the last
// flush to the group's shared total, and flags the group as overflowed when the
// total passes the cap.
func (w *parWorker) flushRecords() {
	r := w.r
	if w.unsent > 0 && r.usedRecs.Add(w.unsent) > r.rcap {
		r.overflow.Store(true)
	}
	w.unsent = 0
}

// copyChain is the chain of moves from the expanded state to the end of the
// walk, plus the move m being fired.
func (w *parWorker) copyChain(m move, with bool) []cex.Ref {
	n := len(w.refs)
	out := make([]cex.Ref, n, n+1)
	copy(out, w.refs)
	if with {
		out = append(out, w.s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
	}
	return out
}

func leU64(b []byte) uint64 { return binary.LittleEndian.Uint64(b) }
func leU32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
