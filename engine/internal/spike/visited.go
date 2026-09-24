package spike

import (
	"encoding/binary"
	"hash/maphash"
)

// Visited is the set of stored states. Two implementations are measured:
// the Go map baseline and a compact open-addressing table.
type Visited interface {
	// Add stores s if new and reports whether it was new.
	Add(s []byte) bool
	// Has reports whether s is stored.
	Has(s []byte) bool
	Len() int
}

// mapVisited is the baseline: one string allocation per state plus map
// bucket overhead. Simple and obviously correct; expensive in bytes/state.
type mapVisited struct{ m map[string]struct{} }

func newMapVisited(hint int) *mapVisited {
	return &mapVisited{m: make(map[string]struct{}, hint)}
}

func (v *mapVisited) Add(s []byte) bool {
	if _, ok := v.m[string(s)]; ok {
		return false
	}
	v.m[string(s)] = struct{}{}
	return true
}

func (v *mapVisited) Has(s []byte) bool {
	_, ok := v.m[string(s)]
	return ok
}

func (v *mapVisited) Len() int { return len(v.m) }

// compactVisited is an open-addressing hash table of 64-bit hashes whose
// slots point into a single append-only arena holding the full state
// vectors (exact mode of plan 14 §4.1: hash for lookup, full vector for
// equality). All states of one run have the same length, so the arena needs
// no per-state length field.
//
// Memory per state ≈ stateLen + 8 (hash) + 4 (offset), times the load-factor
// slack of the table (≤ 2× at load 0.5–1.0 across doublings).
type compactVisited struct {
	seed     maphash.Seed
	stateLen int
	hashes   []uint64 // 0 marks an empty slot; real hashes are forced non-zero
	offsets  []uint32 // index into arena / stateLen
	arena    []byte
	n        int
}

func newCompactVisited(stateLen, hint int) *compactVisited {
	size := 1024
	for size < hint*2 {
		size <<= 1
	}
	return &compactVisited{
		seed:     maphash.MakeSeed(),
		stateLen: stateLen,
		hashes:   make([]uint64, size),
		offsets:  make([]uint32, size),
	}
}

func (v *compactVisited) hash(s []byte) uint64 {
	h := maphash.Bytes(v.seed, s)
	if h == 0 {
		h = 1
	}
	return h
}

// find returns the slot holding s, or the empty slot where it would go.
func (v *compactVisited) find(s []byte, h uint64) (slot uint64, present bool) {
	mask := uint64(len(v.hashes) - 1)
	i := h & mask
	for {
		if v.hashes[i] == 0 {
			return i, false
		}
		if v.hashes[i] == h {
			off := int(v.offsets[i]) * v.stateLen
			if string(v.arena[off:off+v.stateLen]) == string(s) {
				return i, true
			}
		}
		i = (i + 1) & mask
	}
}

func (v *compactVisited) Has(s []byte) bool {
	_, present := v.find(s, v.hash(s))
	return present
}

func (v *compactVisited) Add(s []byte) bool {
	if len(s) != v.stateLen {
		panic("compactVisited: state length changed")
	}
	if 2*(v.n+1) > len(v.hashes) {
		v.grow()
	}
	h := v.hash(s)
	i, present := v.find(s, h)
	if present {
		return false
	}
	v.hashes[i] = h
	v.offsets[i] = uint32(v.n)
	v.arena = append(v.arena, s...)
	v.n++
	return true
}

func (v *compactVisited) grow() {
	oldH, oldO := v.hashes, v.offsets
	v.hashes = make([]uint64, 2*len(oldH))
	v.offsets = make([]uint32, 2*len(oldO))
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
		v.offsets[i] = oldO[j]
	}
}

func (v *compactVisited) Len() int { return v.n }

// putU16 is a helper shared by the hard-coded models.
func putU16(b []byte, x uint16) { binary.LittleEndian.PutUint16(b, x) }
