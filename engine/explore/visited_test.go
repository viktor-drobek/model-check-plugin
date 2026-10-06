package explore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

// vec returns the i-th state of length n: the index in the first bytes, a
// pattern after it, so that a shifted or overlapping copy is visible. Vectors
// are distinct while n is large enough to hold the index (n >= 3 for the
// ranges used here); shorter ones repeat, which the reference set handles
// the same way.
func vec(i, n int) []byte {
	b := make([]byte, n)
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(i)+1)
	copy(b, tmp[:])
	for k := 8; k < n; k++ {
		b[k] = byte(i*31 + k)
	}
	return b
}

// TestCompactAgreesWithMapOnRandomSequences cross-checks the default set
// against the reference set on a random sequence of adds with repeats, for
// vector lengths below, at and above the arena chunk.
func TestCompactAgreesWithMapOnRandomSequences(t *testing.T) {
	for _, n := range []int{0, 1, 3, 16, 100, 5000, arenaChunkBytes + 1} {
		rnd := rand.New(rand.NewSource(int64(n)))
		a, b := NewCompact(n, 0), NewMap()
		// Enough distinct vectors to cross several chunk boundaries at
		// every length (a chunk holds 16 KiB / n vectors, rounded down to
		// a power of two), within a memory cap for the long vectors.
		distinct := 9000
		if n > 0 && n*distinct > 8<<20 {
			distinct = 8 << 20 / n
		}
		for step := 0; step < 12000; step++ {
			s := vec(rnd.Intn(distinct), n)
			ia, na := a.Add(s)
			ib, nb := b.Add(s)
			if ia != ib || na != nb {
				t.Fatalf("len %d step %d: Add = (%d,%v), reference (%d,%v)", n, step, ia, na, ib, nb)
			}
			probe := vec(rnd.Intn(2*distinct), n)
			ha, oka := a.Has(probe)
			hb, okb := b.Has(probe)
			if ha != hb || oka != okb {
				t.Fatalf("len %d step %d: Has = (%d,%v), reference (%d,%v)", n, step, ha, oka, hb, okb)
			}
		}
		if a.Len() != b.Len() {
			t.Fatalf("len %d: Len %d, reference %d", n, a.Len(), b.Len())
		}
		for i := 0; i < a.Len(); i++ {
			if !bytes.Equal(a.Get(i), b.Get(i)) {
				t.Fatalf("len %d: Get(%d) differs from the reference", n, i)
			}
		}
	}
}

// TestGetViewsStayPut: a view returned by Get aliases the stored copy for
// the life of the set. The contract used to say "valid until the next Add"
// because growing the arena moved every stored vector.
func TestGetViewsStayPut(t *testing.T) {
	const n, total = 16, 50000
	v := NewCompact(n, 0)
	for i := 0; i < 100; i++ {
		v.Add(vec(i, n))
	}
	early := make([]*byte, 100)
	for i := range early {
		early[i] = unsafe.SliceData(v.Get(i))
	}
	for i := 100; i < total; i++ {
		v.Add(vec(i, n))
	}
	for i := range early {
		if got := unsafe.SliceData(v.Get(i)); got != early[i] {
			t.Fatalf("state %d moved while the set grew", i)
		}
		if !bytes.Equal(v.Get(i), vec(i, n)) {
			t.Fatalf("state %d changed", i)
		}
	}
}

// TestGetViewCannotReachItsNeighbour: Get is a read-only view, but a caller
// that appends to it must not write into the next stored vector.
func TestGetViewCannotReachItsNeighbour(t *testing.T) {
	const n = 16
	v := NewCompact(n, 0)
	for i := 0; i < 3; i++ {
		v.Add(vec(i, n))
	}
	_ = append(v.Get(0), 0xff)
	if !bytes.Equal(v.Get(1), vec(1, n)) {
		t.Fatal("append to the view of state 0 overwrote state 1")
	}
}

// TestCompactBytes: the memory figure is a pure function of the sequence of
// adds (reports and budgets depend on it being reproducible), never smaller
// than the vectors themselves, and grows with the set.
func TestCompactBytes(t *testing.T) {
	const n = 16
	fill := func(count int) Visited {
		v := NewCompact(n, 0)
		for i := 0; i < count; i++ {
			v.Add(vec(i, n))
		}
		return v
	}
	prev := int64(0)
	for _, count := range []int{0, 1, 1000, 20000, 100000} {
		a, b := fill(count), fill(count)
		if a.Bytes() != b.Bytes() {
			t.Fatalf("%d states: Bytes %d then %d for the same sequence", count, a.Bytes(), b.Bytes())
		}
		if a.Bytes() < int64(count*n) {
			t.Fatalf("%d states: Bytes %d is less than the %d bytes of vectors", count, a.Bytes(), count*n)
		}
		if a.Bytes() < prev {
			t.Fatalf("%d states: Bytes %d fell from %d", count, a.Bytes(), prev)
		}
		prev = a.Bytes()
	}
	// Duplicates add nothing.
	v := fill(1000)
	before := v.Bytes()
	for i := 0; i < 1000; i++ {
		v.Add(vec(i, n))
	}
	if v.Bytes() != before {
		t.Fatalf("re-adding stored states changed Bytes: %d -> %d", before, v.Bytes())
	}
}

// TestFrameBytesIsTheFrame: the memory estimate charges frameBytes per stack
// frame; it used to be 64 for a 72-byte frame, so the stack was undercounted.
func TestFrameBytesIsTheFrame(t *testing.T) {
	if got := int64(unsafe.Sizeof(frame{})); got != frameBytes {
		t.Fatalf("frameBytes = %d, but a frame is %d bytes", int64(frameBytes), got)
	}
}

// TestPushFrameGrowsByDoubling: append grows a large slice by about a
// quarter, so a deep search stack was copied around five times over; the
// stack doubles instead. The frames survive every growth.
func TestPushFrameGrowsByDoubling(t *testing.T) {
	var st []frame
	copies := 0
	for i := 0; i < 100000; i++ {
		before := unsafe.SliceData(st)
		st = pushFrame(st, frame{idx: int32(i)})
		if before != nil && unsafe.SliceData(st) != before {
			copies++
		}
	}
	for i := range st {
		if st[i].idx != int32(i) {
			t.Fatalf("frame %d lost in growth", i)
		}
	}
	// 100000 frames from a first capacity of 64 take 11 doublings.
	if copies > 12 {
		t.Fatalf("the stack was reallocated %d times for 100000 pushes", copies)
	}
}

// BenchmarkVisitedAdd measures the store alone: a million distinct 16-byte
// vectors, then a lookup of each (a revisit, as the search does on most
// transitions). hint is the size the set is created for.
func BenchmarkVisitedAdd(b *testing.B) {
	const n, total = 16, 1 << 20
	states := make([][]byte, total)
	for i := range states {
		states[i] = vec(i, n)
	}
	for _, hint := range []int{1024, total} {
		b.Run(fmt.Sprintf("hint=%d", hint), func(b *testing.B) {
			b.ReportAllocs()
			for it := 0; it < b.N; it++ {
				v := NewCompact(n, hint)
				for _, s := range states {
					v.Add(s)
				}
				for _, s := range states {
					if _, ok := v.Has(s); !ok {
						b.Fatal("lost a state")
					}
				}
			}
		})
	}
}

// TestCompactTableCostsEightBytesASlot: a table slot is one 64-bit word (a
// 32-bit fingerprint of the hash and the 32-bit index of the state, plus one,
// so that zero is empty), not a 64-bit hash and a 32-bit index in two arrays.
// One word is one cache line touched per probe where there were two, and a
// third less memory for the table. A fresh set holds the table and nothing else.
func TestCompactTableCostsEightBytesASlot(t *testing.T) {
	v := NewCompact(16, 0)
	if got := v.Bytes(); got != 1024*8 {
		t.Fatalf("a fresh set of 1024 slots holds %d bytes, want %d", got, 1024*8)
	}
	big := NewCompact(16, 100000) // 2^18 slots: the table for a hint of 100000 states
	if got := big.Bytes(); got != 262144*8 {
		t.Fatalf("a set sized for 100000 states holds %d bytes, want %d", got, 262144*8)
	}
}

// TestCompactEqualFingerprintsAreToldApartByTheVector: the table compares a
// 32-bit fingerprint of the hash and then the whole vector. Vectors stored
// under the same hash (same fingerprint, same position), under hashes with
// the same fingerprint at another position, and under hashes at the same
// position with another fingerprint must all be kept apart, found at their
// own index, and never mistaken for a vector that is not stored.
func TestCompactEqualFingerprintsAreToldApartByTheVector(t *testing.T) {
	v := NewCompact(16, 4096).(*compact) // large enough that nothing grows and the crafted hashes stay valid
	const h = 0xABCDEF0123456789
	hSameFingerprintOtherPlace := uint64(h&^0xFFFF) | 0x1234
	hOtherFingerprintSamePlace := uint64(h) ^ 1<<40
	stored := []struct {
		s []byte
		h uint64
	}{
		{vec(1, 16), h},
		{vec(2, 16), h}, // the same hash as the first
		{vec(3, 16), hSameFingerprintOtherPlace},
		{vec(4, 16), hOtherFingerprintSamePlace},
	}
	for i, x := range stored {
		if idx, isNew := v.addHashed(x.s, x.h); idx != i || !isNew {
			t.Fatalf("vector %d: Add = (%d, %v), want (%d, true)", i, idx, isNew, i)
		}
	}
	for i, x := range stored {
		if idx, ok := v.hasHashed(x.s, x.h); !ok || idx != i {
			t.Fatalf("vector %d: Has = (%d, %v), want (%d, true)", i, idx, ok, i)
		}
		if idx, isNew := v.addHashed(x.s, x.h); isNew || idx != i {
			t.Fatalf("vector %d stored again: Add = (%d, %v)", i, idx, isNew)
		}
	}
	absent := vec(5, 16)
	for _, hh := range []uint64{h, hSameFingerprintOtherPlace, hOtherFingerprintSamePlace} {
		if idx, ok := v.hasHashed(absent, hh); ok {
			t.Fatalf("a vector that was never stored is found at %d under hash %#x", idx, hh)
		}
	}
	if v.Len() != 4 {
		t.Fatalf("Len = %d, want 4", v.Len())
	}
}

// TestCompactRefusesMoreStatesThanAWordCanIndex: the index of a state is 32
// bits of the slot word, plus one. Past that the word would be corrupt and the
// search would answer wrongly without noticing, so the set stops with a
// message. (Reached only with about 70 GB of 16-byte vectors.)
func TestCompactRefusesMoreStatesThanAWordCanIndex(t *testing.T) {
	v := NewCompact(1, 0).(*compact)
	v.n = maxStates // as if that many were stored
	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.Contains(msg, "states") {
			t.Fatalf("Add past the limit: recovered %v, want a panic that names the states", r)
		}
	}()
	v.Add([]byte{1})
	t.Fatal("Add past the limit returned")
}
