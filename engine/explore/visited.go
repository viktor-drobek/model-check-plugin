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
	// view stays valid until the next Add.
	Get(idx int) []byte
	Len() int
	// Bytes is the memory held by the set: arena plus index tables. It is a
	// pure function of the sequence of Adds, so it is reproducible.
	Bytes() int64
}

// compact is the default set (plan 14 §4.1, exact mode): an open-addressing
// table of 64-bit hashes whose slots hold the state's index, and an
// append-only arena of full vectors. Lookup compares the hash, then the full
// vector, so there are no false positives. Memory per state ≈ stateLen + 12
// bytes times the load-factor slack (table at most half full).
type compact struct {
	seed     maphash.Seed
	stateLen int
	hashes   []uint64 // 0 = empty; real hashes are forced non-zero
	idx      []uint32
	arena    []byte
	n        int
}

// NewCompact returns the default visited set for states of stateLen bytes.
func NewCompact(stateLen, hint int) Visited {
	size := 1024
	for size < hint*2 {
		size <<= 1
	}
	return &compact{
		seed:     maphash.MakeSeed(),
		stateLen: stateLen,
		hashes:   make([]uint64, size),
		idx:      make([]uint32, size),
	}
}

func (v *compact) hash(s []byte) uint64 {
	h := maphash.Bytes(v.seed, s)
	if h == 0 {
		h = 1
	}
	return h
}

func (v *compact) find(s []byte, h uint64) (slot uint64, present bool) {
	mask := uint64(len(v.hashes) - 1)
	i := h & mask
	for {
		if v.hashes[i] == 0 {
			return i, false
		}
		if v.hashes[i] == h {
			off := int(v.idx[i]) * v.stateLen
			if string(v.arena[off:off+v.stateLen]) == string(s) {
				return i, true
			}
		}
		i = (i + 1) & mask
	}
}

func (v *compact) Add(s []byte) (int, bool) {
	if len(s) != v.stateLen {
		panic("visited: state length changed")
	}
	if 2*(v.n+1) > len(v.hashes) {
		v.grow()
	}
	h := v.hash(s)
	i, present := v.find(s, h)
	if present {
		return int(v.idx[i]), false
	}
	v.hashes[i] = h
	v.idx[i] = uint32(v.n)
	v.arena = append(v.arena, s...)
	v.n++
	return v.n - 1, true
}

func (v *compact) grow() {
	oldH, oldI := v.hashes, v.idx
	v.hashes = make([]uint64, 2*len(oldH))
	v.idx = make([]uint32, 2*len(oldI))
	mask := uint64(len(v.hashes) - 1)
	for j, h := range oldH {
		if h == 0 {
			continue
		}
		i := h & mask
		for v.hashes[i] != 0 {
			i = (i + 1) & mask
		}
		v.hashes[i] = h
		v.idx[i] = oldI[j]
	}
}

func (v *compact) Has(s []byte) (int, bool) {
	i, present := v.find(s, v.hash(s))
	if !present {
		return 0, false
	}
	return int(v.idx[i]), true
}

func (v *compact) Get(idx int) []byte {
	off := idx * v.stateLen
	return v.arena[off : off+v.stateLen]
}

func (v *compact) Len() int { return v.n }

func (v *compact) Bytes() int64 {
	return int64(cap(v.arena)) + int64(len(v.hashes))*12
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

func (v *mapSet) Get(idx int) []byte { return v.states[idx] }
func (v *mapSet) Len() int           { return len(v.states) }
func (v *mapSet) Bytes() int64       { return v.bytes }
