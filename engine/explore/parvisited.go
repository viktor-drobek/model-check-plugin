package explore

import (
	"encoding/binary"
	"math/bits"
)

// The visited structure of the parallel search (performance plan 5, §2.3).
//
// A partSet splits the set of stored states into parPartitions partitions by a
// fixed-seed hash of the state vector. Each partition has its own open-
// addressing table, its own chunked arena of vectors and its own array of
// parents, and the parallel search lets exactly one goroutine write a
// partition at a time (the owner of the partition in the insertion phase of a
// group) while no goroutine writes any partition in the expansion phase. So a
// partSet has no lock and no atomic access: the discipline of the phases, with
// a barrier between them, is the whole of its synchronisation, and the race
// detector checks that the discipline is kept.
//
// A state's id is partition<<24 | its arrival index in the partition. The ids,
// and therefore the frontier order, the choice of a counterexample and the
// memory estimate, are functions of the model and of the order in which the
// search offers states to the set, never of a worker count, a segment size or
// a clock. The hash is therefore a function of our own, fixed for ever:
// hash/maphash seeds every process differently, which would give a different
// partition to the same state in two runs.
//
// partSet is not a Visited: the default search keeps its own set (compact) and
// its exact code and speed.

const (
	parPartBits   = 8
	parPartitions = 1 << parPartBits
	parIndexBits  = 32 - parPartBits
	// parMaxPerPartition is the number of states a partition can hold. An id is
	// at most 255<<24 | (2^24-2), so 0xFFFFFFFF is never the id of a state: it
	// is the parent of the initial state.
	parMaxPerPartition = 1<<parIndexBits - 1
	// parNoParent is the parent of the initial state.
	parNoParent = ^uint32(0)
	// parFirstSlots is the table size of a partition at its first insert: a
	// small model touches a few partitions and must cost kilobytes, not the
	// tens of megabytes that 256 eagerly allocated tables would.
	parFirstSlots = 256
)

// parHash is the fixed-seed hash of a state vector: a few lines of word-at-a-
// time multiply-and-fold arithmetic (the wyhash family), little-endian reads,
// the same on every platform. Bits 56-63 select the partition, bits 24-55 are
// the fingerprint kept in the table slot and the low bits are the position in
// the table. The three are disjoint in a table of up to 2^24 slots; a partition
// near its capacity needs a larger table, whose position reaches into the
// fingerprint bits. The fingerprint is only a filter (a slot is taken as the
// state only after the whole vector has been compared), so that costs a few more
// comparisons and never a wrong answer.
func parHash(b []byte) uint64 {
	const (
		m1   = 0xa0761d6478bd642f
		m2   = 0xe7037ed1a0b428db
		m3   = 0x8ebc6af09c88c6e3
		seed = 0x2d358dccaa6c78a5
	)
	h := uint64(len(b))*m3 ^ seed
	n := len(b)
	i := 0
	for ; i+8 <= n; i += 8 {
		h = parMix(h^binary.LittleEndian.Uint64(b[i:]), m1)
	}
	if i < n {
		var v uint64
		if n >= 8 {
			v = binary.LittleEndian.Uint64(b[n-8:]) // overlaps the last full word
		} else {
			for k := n - 1; k >= 0; k-- {
				v = v<<8 | uint64(b[k])
			}
		}
		h = parMix(h^v, m1)
	}
	return parMix(h, m2)
}

// parMix folds the 128-bit product of a and b into 64 bits.
func parMix(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	return hi ^ lo
}

// parPartOf is the partition of a hash.
func parPartOf(h uint64) int { return int(h >> (64 - parPartBits)) }

// parResult is what add did.
type parResult uint8

const (
	// parAdded: the vector was new and is stored.
	parAdded parResult = iota
	// parPresent: the vector was already stored; nothing changed.
	parPresent
	// parFull: the vector is new but its partition is at capacity; it is not
	// stored. A search stops on this as a declared bound.
	parFull
)

func (r parResult) String() string {
	switch r {
	case parAdded:
		return "added"
	case parPresent:
		return "present"
	case parFull:
		return "full"
	}
	return "?"
}

// parPart is one partition.
type parPart struct {
	slots  []uint64 // 0 = empty; else fingerprint<<32 | index+1
	chunks [][]byte // full vectors, 1<<shift per chunk, in arrival order
	parent []uint32 // the state whose expansion first produced each state
	n      int

	// Bookkeeping of the search, written only by the goroutine that owns the
	// partition in the insertion phase: the level in which the partition last
	// received a state and its size before that level's first insert (the
	// states from there on are the next frontier's range of this partition).
	epoch   int32
	levelLo int32
	// dirty is the group in which the partition last received a state, and acct
	// the bytes of it the set has accounted for (see accountGroup).
	dirty int32
	acct  int64
}

// partSet is the partitioned visited set.
type partSet struct {
	stateLen int
	shift    uint // log2 of the vectors per chunk
	mask     int
	parts    [parPartitions]*parPart
	// limit is the capacity of a partition; tests lower it to reach the bound
	// without storing 16 million states.
	limit int
	// epoch is the level whose states are being stored. The search changes it
	// between levels, while no goroutine is inside the set, and the owner of a
	// partition compares it with the partition's own to tell the first state
	// it stores in a level.
	epoch int32
	// reading is true while the workers expand the states of a group: no one
	// writes the set then. The search sets it between two barriers and the
	// workers read it, so it needs no atomic access; an insert while it is set
	// is a defect of the search, caught here instead of showing up as a race.
	reading bool
	// groupEpoch is the group being inserted, bytesAcct the sum of the bytes of
	// the partitions as of the last accountGroup: the coordinator keeps it up to
	// date from the partitions a group touched, so that the byte count of a set
	// of 256 partitions is not recomputed for every group (a model with two
	// million layers has two million groups).
	groupEpoch int32
	bytesAcct  int64
}

// newPartSet returns an empty set for vectors of stateLen bytes.
func newPartSet(stateLen int) *partSet {
	shift := uint(0)
	for shift < maxChunkShift && (2<<shift)*stateLen <= arenaChunkBytes {
		shift++
	}
	return &partSet{stateLen: stateLen, shift: shift, mask: 1<<shift - 1, limit: parMaxPerPartition, groupEpoch: 1}
}

func (ps *partSet) vec(p *parPart, idx int) []byte {
	off := (idx & ps.mask) * ps.stateLen
	return p.chunks[idx>>ps.shift][off : off+ps.stateLen : off+ps.stateLen]
}

// find probes partition p for s, whose hash is h: the slot where it is stored,
// or the empty slot where it would go.
func (ps *partSet) find(p *parPart, s []byte, h uint64) (slot uint64, present bool) {
	mask := uint64(len(p.slots) - 1)
	fp := uint64(uint32(h >> 24))
	i := h & mask
	for {
		w := p.slots[i]
		if w == 0 {
			return i, false
		}
		if w>>32 == fp && string(ps.vec(p, int(uint32(w))-1)) == string(s) {
			return i, true
		}
		i = (i + 1) & mask
	}
}

// grow doubles the table of p, rehashing the arena in arrival order.
func (ps *partSet) grow(p *parPart) {
	p.slots = make([]uint64, 2*len(p.slots))
	mask := uint64(len(p.slots) - 1)
	for idx := 0; idx < p.n; idx++ {
		h := parHash(ps.vec(p, idx))
		i := h & mask
		for p.slots[i] != 0 {
			i = (i + 1) & mask
		}
		p.slots[i] = uint64(uint32(h>>24))<<32 | uint64(idx+1)
	}
}

// part returns the partition for hash h, creating it at its first use.
func (ps *partSet) part(h uint64) (int, *parPart) {
	pi := parPartOf(h)
	p := ps.parts[pi]
	if p == nil {
		p = &parPart{slots: make([]uint64, parFirstSlots)}
		ps.parts[pi] = p
	}
	return pi, p
}

// add stores s, whose hash is h, with the given parent, unless it is stored
// already or its partition is full. It returns the id of the state (stored or
// found; undefined for parFull) and what it did. s is copied.
//
// The hash is a parameter, not computed here, so that a test can store
// vectors under hashes it chose (equal fingerprints of different vectors are
// far too rare to meet at random) and so that the search hashes a successor
// once, when it writes its record.
func (ps *partSet) add(h uint64, s []byte, parent uint32) (uint32, parResult) {
	pi, p := ps.part(h)
	return ps.addIn(pi, p, h, s, parent)
}

// addIn is add for a caller that has found the partition already (pi, p =
// ps.part(h)) and wants to look at it before the insert.
func (ps *partSet) addIn(pi int, p *parPart, h uint64, s []byte, parent uint32) (uint32, parResult) {
	if ps.reading {
		panic("parvisited: an insert during the expansion phase, which no goroutine may write in")
	}
	if p.n >= ps.limit {
		// A full partition accepts only what it stores already, and nothing
		// grows: the table of a partition is a function of its size alone.
		if i, present := ps.find(p, s, h); present {
			return uint32(pi)<<parIndexBits | uint32(uint32(p.slots[i])-1), parPresent
		}
		return 0, parFull
	}
	if 2*(p.n+1) > len(p.slots) {
		ps.grow(p)
	}
	i, present := ps.find(p, s, h)
	if present {
		return uint32(pi)<<parIndexBits | uint32(uint32(p.slots[i])-1), parPresent
	}
	idx := p.n
	p.slots[i] = uint64(uint32(h>>24))<<32 | uint64(idx+1)
	if c := idx >> ps.shift; c == len(p.chunks) {
		p.chunks = append(p.chunks, make([]byte, ps.stateLen<<ps.shift))
	}
	copy(ps.vec(p, idx), s)
	p.parent = append(p.parent, parent)
	p.n++
	return uint32(pi)<<parIndexBits | uint32(idx), parAdded
}

// has reports whether s, whose hash is h, is stored, and its id.
func (ps *partSet) has(h uint64, s []byte) (uint32, bool) {
	pi := parPartOf(h)
	p := ps.parts[pi]
	if p == nil {
		return 0, false
	}
	i, present := ps.find(p, s, h)
	if !present {
		return 0, false
	}
	return uint32(pi)<<parIndexBits | uint32(uint32(p.slots[i])-1), true
}

// get returns a read-only view of the vector with the given id. The view
// aliases the arena and is never moved by later inserts.
func (ps *partSet) get(id uint32) []byte {
	return ps.vec(ps.parts[id>>parIndexBits], int(id&(1<<parIndexBits-1)))
}

// parentOf is the id of the state whose expansion first produced id
// (parNoParent for the initial state).
func (ps *partSet) parentOf(id uint32) uint32 {
	return ps.parts[id>>parIndexBits].parent[id&(1<<parIndexBits-1)]
}

// Len is the number of stored states: the sum of the partition sizes, which no
// two owners write alike (a shared counter would be written by every worker of
// the insertion phase).
func (ps *partSet) Len() int {
	n := 0
	for _, p := range ps.parts {
		if p != nil {
			n += p.n
		}
	}
	return n
}

// partLen is the number of states stored in partition pi.
func (ps *partSet) partLen(pi int) int {
	if p := ps.parts[pi]; p != nil {
		return p.n
	}
	return 0
}

// each calls f for every stored state in ascending id order.
func (ps *partSet) each(f func(id uint32, v []byte)) {
	for pi, p := range ps.parts {
		if p == nil {
			continue
		}
		for idx := 0; idx < p.n; idx++ {
			f(uint32(pi)<<parIndexBits|uint32(idx), ps.vec(p, idx))
		}
	}
}

// Bytes is the memory the set holds: per used partition the chunks of its
// arena (a chunk counts whole) with their directory entries, its table at its
// length and 4 bytes of parent per state, counted by formula and not by the
// capacity the runtime gave the slice; plus the 256 pointers of the set. It is
// a function of the stored set alone: the partition of a state is fixed by the
// hash, a partition's table length and chunk count follow from its size, so
// neither the order of insertion nor the worker count enters. It does depend
// on the hash, like every figure that depends on a data layout.
func (ps *partSet) Bytes() int64 {
	total := int64(parPartitions) * 8
	for _, p := range ps.parts {
		if p != nil {
			total += ps.partBytes(p)
		}
	}
	return total
}

// partBytes is the memory of one partition, a function of its size alone.
func (ps *partSet) partBytes(p *parPart) int64 {
	return int64(len(p.chunks))*(int64(ps.stateLen<<ps.shift)+sliceHeaderBytes) + int64(len(p.slots))*slotBytes + int64(p.n)*4
}

// accountGroup brings the running byte count up to date with the partitions
// that received a state since the last call (the coordinator calls it between
// two barriers, with the partitions the workers reported, and then increments
// groupEpoch to start the next group's bookkeeping).
func (ps *partSet) accountGroup(dirty []uint8) {
	for _, pi := range dirty {
		p := ps.parts[pi]
		nb := ps.partBytes(p)
		ps.bytesAcct += nb - p.acct
		p.acct = nb
	}
}

// accounted is Bytes as the running count has it, in constant time; it agrees
// with Bytes whenever every group has been accounted.
func (ps *partSet) accounted() int64 { return int64(parPartitions)*8 + ps.bytesAcct }
