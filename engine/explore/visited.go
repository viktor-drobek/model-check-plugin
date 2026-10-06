package explore

import "hash/maphash"

// Visited stores the set of reached states and gives each a dense index in
// insertion order, so that DFS frames and BFS parent links can refer to a
// state by index instead of holding a copy of it.
type Visited interface {
	// Add stores s if new. It returns the state's index and whether it was
	// new. s is copied; the caller may reuse its buffer.
	Add(s []byte) (idx int, isNew bool)
	// Has reports whether s is stored and its index.
	Has(s []byte) (idx int, ok bool)
	// Get returns a read-only view of the state with the given index. The
	// view aliases the stored copy and stays valid, unchanged, for the life
	// of the set: adding states never moves one that is stored. Its capacity
	// ends with the state, so a caller that appends to it copies.
	Get(idx int) []byte
	Len() int
	// Bytes is the memory held by the set: arena plus index tables. It is a
	// pure function of the sequence of Adds, so it is reproducible.
	Bytes() int64
}

const (
	// arenaChunkBytes is the target size of one arena chunk: the arena is a
	// list of chunks holding a power-of-two number of vectors each (as many
	// as fit), so that a vector is found by shifting its index and a stored
	// vector is never moved. 16 KiB is a small-object size class: no chunk
	// is zeroed or copied again, and a tiny model holds one chunk.
	arenaChunkBytes = 16 << 10
	// maxStates is how many states a set can number: the index goes into 32
	// bits of the slot word, plus one so that zero is an empty slot.
	maxStates = 1<<32 - 2
	// maxChunkShift bounds the vectors per chunk (1<<30) for a zero-length
	// vector, which would otherwise fit forever.
	maxChunkShift = 30
	// slotBytes is what the table holds per slot: one 64-bit word, a 32-bit
	// fingerprint of the hash and the 32-bit index of the state plus one.
	slotBytes = 8
	// sliceHeaderBytes is the cost of one entry of the chunk directory (a
	// slice header on a 64-bit target, the only kind the engine ships for).
	sliceHeaderBytes = 24
)

// compact is the default set (plan 14 §4.1, exact mode): an open-addressing
// table of 64-bit words, and an append-only arena of full vectors. A word is
// the high 32 bits of the vector's hash (its fingerprint) over the 32-bit
// index of the vector plus one, so that a zero word is an empty slot; the
// position of the entry is the low bits of the hash. A lookup reads one word
// per probe, and compares the whole vector only when the fingerprint
// matches, so there are no false positives however often two vectors share a
// fingerprint (one probe in 2^32 meets a stranger). Memory per state ≈
// stateLen + 8 bytes times the load-factor slack (table at most half full).
//
// The word does not hold the low bits of the hash, so when the table grows
// the positions are found by hashing the vectors again, in index order, which
// reads the arena sequentially. That costs one hash per state per doubling,
// a few nanoseconds against the cache miss a random write into the new table
// costs anyway.
//
// The arena is a directory of fixed-size chunks rather than one slice. A
// slice that grows by append is copied whole at every growth (Go enlarges a
// large one by about a quarter, so a million vectors are copied several
// times over, and the old and the new array coexist while it happens); a
// chunk, once filled, is never touched again, and the directory that
// points at the chunks is 24 bytes per chunk.
type compact struct {
	seed     maphash.Seed
	stateLen int
	slots    []uint64 // 0 = empty; else fingerprint<<32 | index+1
	chunks   [][]byte // full vectors, 1<<shift per chunk, in index order
	shift    uint     // log2 of the vectors per chunk
	mask     int      // 1<<shift - 1
	n        int
}

// initialStates is how many states a default set is sized for before it has
// seen one. It is not taken from the state budget: a budget is a ceiling, not
// a forecast, and a table sized for a million states costs 24 MB (and shows
// as such in the memory estimate) before a model of twenty states has run.
// Measured on a million 16-byte states, a table sized for all of them up
// front saves under a tenth of the time and about a sixth of the bytes
// allocated, so growing from here is the better default; a caller that
// knows the size (an estimate) can call NewCompact itself.
const initialStates = 1024

// defaultVisited is the visited set of a search that was not given one.
func defaultVisited(stateLen int) Visited { return NewCompact(stateLen, initialStates) }

// NewCompact returns the default visited set for states of stateLen bytes,
// sized for hint states.
func NewCompact(stateLen, hint int) Visited {
	size := 1024
	for size < hint*2 {
		size <<= 1
	}
	// As many vectors per chunk as fit, a power of two. The bound matters
	// only for an empty vector, which fits any number of times.
	shift := uint(0)
	for shift < maxChunkShift && (2<<shift)*stateLen <= arenaChunkBytes {
		shift++
	}
	return &compact{
		seed:     maphash.MakeSeed(),
		stateLen: stateLen,
		slots:    make([]uint64, size),
		shift:    shift,
		mask:     1<<shift - 1,
	}
}

func (v *compact) hash(s []byte) uint64 { return maphash.Bytes(v.seed, s) }

// word is the table entry of vector idx whose hash is h.
func word(h uint64, idx int) uint64 { return h&^0xFFFFFFFF | uint64(idx+1) }

// view is the stored vector with the given index, capped at its own end.
func (v *compact) view(idx int) []byte {
	off := (idx & v.mask) * v.stateLen
	return v.chunks[idx>>v.shift][off : off+v.stateLen : off+v.stateLen]
}

// find probes for s, whose hash is h: the slot where it is stored, or the
// empty slot where it would go.
func (v *compact) find(s []byte, h uint64) (slot uint64, present bool) {
	mask := uint64(len(v.slots) - 1)
	fp := h >> 32
	i := h & mask
	for {
		w := v.slots[i]
		if w == 0 {
			return i, false
		}
		if w>>32 == fp && string(v.view(int(uint32(w))-1)) == string(s) {
			return i, true
		}
		i = (i + 1) & mask
	}
}

func (v *compact) Add(s []byte) (int, bool) { return v.addHashed(s, v.hash(s)) }

// addHashed is Add for a vector whose hash is h. The seam exists so that a
// test can store vectors under hashes it chose, which it cannot do through
// the hash function: equal fingerprints of different vectors are far too
// rare to meet at random.
func (v *compact) addHashed(s []byte, h uint64) (int, bool) {
	if len(s) != v.stateLen {
		panic("visited: state length changed")
	}
	if v.n >= maxStates {
		// 70 GB of 16-byte vectors. A word with a corrupt index would give a
		// wrong verdict without a sign of it, which is worse than stopping.
		panic("visited: more than 2^32-2 states cannot be numbered")
	}
	if 2*(v.n+1) > len(v.slots) {
		v.grow()
	}
	i, present := v.find(s, h)
	if present {
		return int(uint32(v.slots[i])) - 1, false
	}
	v.slots[i] = word(h, v.n)
	if c := v.n >> v.shift; c == len(v.chunks) {
		v.chunks = append(v.chunks, make([]byte, v.stateLen<<v.shift))
	}
	copy(v.view(v.n), s)
	v.n++
	return v.n - 1, true
}

func (v *compact) grow() {
	v.slots = make([]uint64, 2*len(v.slots))
	mask := uint64(len(v.slots) - 1)
	for idx := 0; idx < v.n; idx++ {
		h := v.hash(v.view(idx))
		i := h & mask
		for v.slots[i] != 0 {
			i = (i + 1) & mask
		}
		v.slots[i] = word(h, idx)
	}
}

func (v *compact) Has(s []byte) (int, bool) { return v.hasHashed(s, v.hash(s)) }

func (v *compact) hasHashed(s []byte, h uint64) (int, bool) {
	i, present := v.find(s, h)
	if !present {
		return 0, false
	}
	return int(uint32(v.slots[i])) - 1, true
}

func (v *compact) Get(idx int) []byte { return v.view(idx) }

func (v *compact) Len() int { return v.n }

// Bytes is the memory the set holds: every chunk allocated (a chunk counts
// whole, filled or not), one slice header per chunk for the directory, and
// the table at its length. Every term is fixed by the sequence of
// Adds alone; none comes from how the Go runtime rounds the capacity of a
// slice, so a report does not change with the toolchain. It does not include
// what the runtime keeps besides (collector headroom, the old table while a
// new one is built).
func (v *compact) Bytes() int64 {
	return int64(len(v.chunks))*(int64(v.stateLen<<v.shift)+sliceHeaderBytes) +
		int64(len(v.slots))*slotBytes
}

// mapSet is the obviously-correct reference used by tests to cross-check
// compact: a Go map plus a slice of copies.
type mapSet struct {
	m      map[string]int
	states [][]byte
	bytes  int64
}

// NewMap returns the reference set.
func NewMap() Visited { return &mapSet{m: map[string]int{}} }

func (v *mapSet) Add(s []byte) (int, bool) {
	if i, ok := v.m[string(s)]; ok {
		return i, false
	}
	c := append([]byte(nil), s...)
	v.m[string(c)] = len(v.states)
	v.states = append(v.states, c)
	v.bytes += int64(len(c)) + 48
	return len(v.states) - 1, true
}

func (v *mapSet) Has(s []byte) (int, bool) {
	i, ok := v.m[string(s)]
	return i, ok
}

func (v *mapSet) Get(idx int) []byte {
	c := v.states[idx]
	return c[:len(c):len(c)]
}
func (v *mapSet) Len() int     { return len(v.states) }
func (v *mapSet) Bytes() int64 { return v.bytes }
