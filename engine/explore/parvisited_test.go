package explore

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"testing"
)

// Step 1 of performance plan 5: the partitioned visited set of the parallel
// search. The tests are the contract of parvisited.go: it behaves as a set
// (against a map), its ids and byte count are functions of the stored set and
// never of the order of insertion, its hash is the same on every platform and
// in every process, and it spreads structured vectors evenly.

func randVec(rnd *rand.Rand, n, alphabet int) []byte {
	v := make([]byte, n)
	for i := range v {
		v[i] = byte(rnd.Intn(alphabet))
	}
	return v
}

func TestPartSetAgreesWithTheMapReference(t *testing.T) {
	for _, tc := range []struct{ stateLen, adds, alphabet int }{
		{1, 200, 256}, {3, 1000, 7}, {16, 5000, 4}, {16, 30000, 256}, {100, 3000, 3}, {5000, 120, 2}, {0, 5, 1},
	} {
		t.Run(fmt.Sprintf("len%d_adds%d", tc.stateLen, tc.adds), func(t *testing.T) {
			rnd := rand.New(rand.NewSource(int64(tc.stateLen*7919 + tc.adds)))
			ps := newPartSet(tc.stateLen)
			ref := map[string]uint32{}
			var order []string
			for i := 0; i < tc.adds; i++ {
				v := randVec(rnd, tc.stateLen, tc.alphabet)
				parent := uint32(rnd.Intn(1000))
				id, res := ps.add(parHash(v), v, parent)
				want, seen := ref[string(v)]
				switch {
				case seen && res != parPresent:
					t.Fatalf("add %d: stored vector reported %v, want present", i, res)
				case seen && id != want:
					t.Fatalf("add %d: id %#x for a stored vector, first id %#x", i, id, want)
				case !seen && res != parAdded:
					t.Fatalf("add %d: new vector reported %v", i, res)
				case !seen:
					ref[string(v)] = id
					order = append(order, string(v))
					if got := ps.parentOf(id); got != parent {
						t.Fatalf("add %d: parent %d, want %d", i, got, parent)
					}
				}
			}
			if ps.Len() != len(ref) {
				t.Fatalf("Len %d, want %d", ps.Len(), len(ref))
			}
			seenID := map[uint32]bool{}
			for _, v := range order {
				id := ref[v]
				if seenID[id] {
					t.Fatalf("id %#x given to two vectors", id)
				}
				seenID[id] = true
				if got := string(ps.get(id)); got != v {
					t.Fatalf("view(%#x) = %x, want %x", id, got, v)
				}
				if got, ok := ps.has(parHash([]byte(v)), []byte(v)); !ok || got != id {
					t.Fatalf("has(%x) = %#x, %v; want %#x", v, got, ok, id)
				}
			}
			// Every state is reachable through the iteration, once.
			n := 0
			ps.each(func(id uint32, v []byte) {
				n++
				if ref[string(v)] != id {
					t.Fatalf("each: %x has id %#x, want %#x", v, id, ref[string(v)])
				}
			})
			if n != len(ref) {
				t.Fatalf("each visited %d states, want %d", n, len(ref))
			}
			// An absent vector is not found.
			absent := make([]byte, tc.stateLen)
			for i := range absent {
				absent[i] = 255
			}
			if _, in := ref[string(absent)]; !in {
				if _, ok := ps.has(parHash(absent), absent); ok {
					t.Fatal("has: an absent vector was found")
				}
			}
		})
	}
}

func TestPartSetIDsNameThePartitionAndTheArrivalIndex(t *testing.T) {
	rnd := rand.New(rand.NewSource(11))
	ps := newPartSet(8)
	next := map[int]int{} // partition -> arrivals so far
	for i := 0; i < 20000; i++ {
		v := randVec(rnd, 8, 256)
		h := parHash(v)
		id, res := ps.add(h, v, parNoParent)
		if res != parAdded {
			continue
		}
		p := parPartOf(h)
		if int(id>>parIndexBits) != p || int(id&(1<<parIndexBits-1)) != next[p] {
			t.Fatalf("id %#x for partition %d arrival %d", id, p, next[p])
		}
		next[p]++
		if id == parNoParent {
			t.Fatalf("a state got the id of the missing parent")
		}
	}
}

// Bytes is a function of the set of stored states alone: not of the order the
// states arrived in, not of the order the partitions were visited in, and not
// of any capacity the Go runtime chose.
func TestPartSetBytesDoesNotDependOnTheOrder(t *testing.T) {
	rnd := rand.New(rand.NewSource(5))
	var vecs [][]byte
	seen := map[string]bool{}
	for len(vecs) < 40000 {
		v := randVec(rnd, 12, 256)
		if !seen[string(v)] {
			seen[string(v)] = true
			vecs = append(vecs, v)
		}
	}
	fill := func(order []int) int64 {
		ps := newPartSet(12)
		for _, i := range order {
			ps.add(parHash(vecs[i]), vecs[i], 0)
		}
		return ps.Bytes()
	}
	ident := make([]int, len(vecs))
	for i := range ident {
		ident[i] = i
	}
	want := fill(ident)
	if want <= 0 {
		t.Fatalf("Bytes = %d", want)
	}
	for k := 0; k < 5; k++ {
		p := append([]int(nil), ident...)
		rnd.Shuffle(len(p), func(i, j int) { p[i], p[j] = p[j], p[i] })
		if got := fill(p); got != want {
			t.Fatalf("shuffled order %d: Bytes %d, want %d", k, got, want)
		}
	}
	// Partition by partition, in a permuted order of the partitions.
	byPart := make([][]int, parPartitions)
	for i, v := range vecs {
		p := parPartOf(parHash(v))
		byPart[p] = append(byPart[p], i)
	}
	perm := rnd.Perm(parPartitions)
	var grouped []int
	for _, p := range perm {
		grouped = append(grouped, byPart[p]...)
	}
	if got := fill(grouped); got != want {
		t.Fatalf("partition-major order: Bytes %d, want %d", got, want)
	}
	// And Bytes grows with the set.
	if small := fill(ident[:100]); small >= want {
		t.Fatalf("100 states cost %d bytes, 40000 cost %d", small, want)
	}
}

func TestPartSetCostsNothingForPartitionsNobodyUses(t *testing.T) {
	ps := newPartSet(16)
	empty := ps.Bytes()
	v := make([]byte, 16)
	ps.add(parHash(v), v, parNoParent)
	if one := ps.Bytes() - empty; one <= 0 || one > 64<<10 {
		t.Fatalf("one state costs %d bytes on top of %d for the empty set: partitions are not created lazily", one, empty)
	}
}

// parHash is pinned: the id of a state, the order of the frontier, the choice
// of a counterexample and the memory estimate all follow from it, so a change
// would change reports. The values were produced by refHash below (big-integer products, written
// separately from the code under test) when the function was written.
func TestParHashIsPinned(t *testing.T) {
	for _, tc := range []struct {
		in   []byte
		want uint64
	}{
		{nil, 0x54896dee09ba54be},
		{[]byte{0}, 0x34460726314a120b},
		{[]byte("abcdefgh"), 0x51efe738d2075c5f},
		{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, 0x7afe609841e01a5b},
		{make([]byte, 16), 0x51c30e07d5a09bb2},
		{[]byte{1, 2, 3}, 0x40059db330dc7bd9},
		{[]byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 11}, 0x82940fdb146c631a},
	} {
		if got := parHash(tc.in); got != tc.want {
			t.Errorf("parHash(%x) = %#x, pinned %#x", tc.in, got, tc.want)
		}
	}
}

// refHash is an independent statement of the function, with big integers for
// the 128-bit products, so that the pinned values are not just a copy of the
// code's own output.
func refHash(b []byte) uint64 {
	const (
		m1   = 0xa0761d6478bd642f
		m2   = 0xe7037ed1a0b428db
		m3   = 0x8ebc6af09c88c6e3
		seed = 0x2d358dccaa6c78a5
	)
	mix := func(a, c uint64) uint64 {
		p := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(c))
		lo := new(big.Int).And(p, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
		hi := new(big.Int).Rsh(p, 64).Uint64()
		return hi ^ lo
	}
	h := uint64(len(b))*m3 ^ seed
	i := 0
	for ; i+8 <= len(b); i += 8 {
		h = mix(h^binary.LittleEndian.Uint64(b[i:]), m1)
	}
	if i < len(b) {
		var v uint64
		if len(b) >= 8 {
			v = binary.LittleEndian.Uint64(b[len(b)-8:])
		} else {
			for k := len(b) - 1; k >= 0; k-- {
				v = v<<8 | uint64(b[k])
			}
		}
		h = mix(h^v, m1)
	}
	return mix(h, m2)
}

func TestParHashMatchesTheReferenceStatement(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	for n := 0; n <= 70; n++ {
		for k := 0; k < 20; k++ {
			v := randVec(rnd, n, 256)
			if got, want := parHash(v), refHash(v); got != want {
				t.Fatalf("parHash(%x) = %#x, the reference gives %#x", v, got, want)
			}
		}
	}
}

func TestParHashSeparatesNearbyVectors(t *testing.T) {
	// A different length, a different last byte and a single flipped bit all
	// give a different hash (no collision among 100 000 such vectors).
	seen := map[uint64][]byte{}
	for n := 0; n < 20; n++ {
		for b := 0; b < n*8; b++ {
			v := make([]byte, n)
			v[b/8] = 1 << (b % 8)
			h := parHash(v)
			if o, dup := seen[h]; dup && string(o) != string(v) {
				t.Fatalf("%x and %x collide", o, v)
			}
			seen[h] = v
		}
	}
	for i := 0; i < 70000; i++ {
		v := make([]byte, 16)
		binary.LittleEndian.PutUint32(v, uint32(i))
		h := parHash(v)
		if o, dup := seen[h]; dup && string(o) != string(v) {
			t.Fatalf("%x and %x collide", o, v)
		}
		seen[h] = v
	}
}

// Structured vectors are what a model produces: counters, one-hot program
// counters, channel buffers with a length byte. The partition (the top byte of
// the hash) must spread them as a good hash would: chi-square around 255 for
// 256 partitions; the bound is five standard deviations above it, and no
// partition may be further than five standard deviations from the mean.
func TestParHashBalancesStructuredVectors(t *testing.T) {
	type gen struct {
		name string
		n    int
		vec  func(i int, v []byte)
	}
	gens := []gen{
		{"five counters below 10", 100000, func(i int, v []byte) {
			for k := 0; k < 5; k++ {
				v[2+k] = byte(i % 10)
				i /= 10
			}
		}},
		{"program counters: three processes with 40 locations and a shared byte", 64000, func(i int, v []byte) {
			v[0], v[1], v[2], v[3] = byte(i%40), byte(i/40%40), byte(i/1600%40), byte(i/64000%256)
		}},
		{"a channel: length byte, three messages of two fields", 4 * 20 * 20 * 20, func(i int, v []byte) {
			v[0] = byte(i % 4)
			i /= 4
			for k := 0; k < 3; k++ {
				v[1+2*k] = byte(i % 5)
				v[2+2*k] = byte(i / 5 % 4)
				i /= 20
			}
		}},
		{"sequence number in a 4-byte field", 100000, func(i int, v []byte) { binary.LittleEndian.PutUint32(v[4:], uint32(i)) }},
		{"two 16-bit counters", 100000, func(i int, v []byte) {
			binary.LittleEndian.PutUint16(v[8:], uint16(i%316))
			binary.LittleEndian.PutUint16(v[10:], uint16(i/316))
		}},
	}
	for _, g := range gens {
		var cnt [parPartitions]int
		distinct := map[string]bool{}
		for i := 0; i < g.n; i++ {
			v := make([]byte, 16)
			g.vec(i, v)
			if distinct[string(v)] {
				continue // a state is counted once
			}
			distinct[string(v)] = true
			cnt[parPartOf(parHash(v))]++
		}
		if len(distinct) < g.n/2 {
			t.Fatalf("%s: only %d distinct vectors of %d", g.name, len(distinct), g.n)
		}
		mean := float64(len(distinct)) / parPartitions
		chi, worst := 0.0, 0.0
		for _, c := range cnt {
			d := float64(c) - mean
			chi += d * d / mean
			worst = math.Max(worst, math.Abs(d)/mean)
		}
		if chi > 255+5*math.Sqrt(2*255) {
			t.Errorf("%s: chi-square %.0f over 256 partitions (expected about 255)", g.name, chi)
		}
		if bound := 5 / math.Sqrt(mean); worst > bound { // five standard deviations of a binomial count
			t.Errorf("%s: a partition is %.0f%% from the mean of %.0f states (bound %.0f%%)", g.name, worst*100, mean, bound*100)
		}
	}
}

func TestParHashUsesAllBitsItRoutesWith(t *testing.T) {
	// The partition (bits 56-63), the fingerprint (24-55) and the position
	// (low bits) must each vary over a range of vectors: a hash that left one
	// of them constant would turn the table into a list.
	var part, fp, low [256]int
	for i := 0; i < 50000; i++ {
		v := make([]byte, 16)
		binary.LittleEndian.PutUint32(v, uint32(i))
		h := parHash(v)
		part[h>>56]++
		fp[byte(h>>24)]++
		fp[byte(h>>40)]++
		low[byte(h)]++
	}
	for name, a := range map[string]*[256]int{"partition": &part, "fingerprint": &fp, "position": &low} {
		used := 0
		for _, c := range a {
			if c > 0 {
				used++
			}
		}
		if used < 250 {
			t.Errorf("%s bits take only %d of 256 byte values", name, used)
		}
	}
}

// Two different vectors whose hashes are equal in the fingerprint and in the
// position are told apart by comparing the vectors: add takes the hash as an
// argument for exactly this test (equal fingerprints of different vectors are
// far too rare to meet at random).
func TestPartSetTellsEqualHashesApart(t *testing.T) {
	ps := newPartSet(4)
	h := uint64(0xAB)<<56 | 0x1234567<<24 | 7 // one partition, one fingerprint, one position
	var ids []uint32
	var vecs [][]byte
	for i := 0; i < 50; i++ {
		v := []byte{byte(i), 1, 2, 3}
		id, res := ps.add(h, v, parNoParent)
		if res != parAdded {
			t.Fatalf("vector %d under a shared hash: %v", i, res)
		}
		ids = append(ids, id)
		vecs = append(vecs, v)
	}
	for i, v := range vecs {
		if id, res := ps.add(h, v, parNoParent); res != parPresent || id != ids[i] {
			t.Fatalf("vector %d again: %#x %v, want %#x present", i, id, res, ids[i])
		}
		if got, ok := ps.has(h, v); !ok || got != ids[i] {
			t.Fatalf("has %d: %#x %v", i, got, ok)
		}
	}
	if _, ok := ps.has(h, []byte{99, 9, 9, 9}); ok {
		t.Fatal("a stranger under the same hash was found")
	}
	if ps.Len() != 50 {
		t.Fatalf("Len %d", ps.Len())
	}
}

func TestPartSetStopsAtThePartitionCapacity(t *testing.T) {
	ps := newPartSet(4)
	ps.limit = 5
	var inP []([]byte)
	for i := 0; len(inP) < 8; i++ {
		v := []byte{byte(i), byte(i >> 8), 0, 0}
		if parPartOf(parHash(v)) == 3 {
			inP = append(inP, v)
		}
	}
	for i, v := range inP {
		id, res := ps.add(parHash(v), v, parNoParent)
		if i < 5 && res != parAdded {
			t.Fatalf("state %d: %v, want added", i, res)
		}
		if i >= 5 && res != parFull {
			t.Fatalf("state %d: %v, want full (capacity 5)", i, res)
		}
		_ = id
	}
	if ps.Len() != 5 || ps.partLen(3) != 5 {
		t.Fatalf("Len %d partLen %d, want 5 5", ps.Len(), ps.partLen(3))
	}
	// A state already stored is still found in a full partition.
	if _, res := ps.add(parHash(inP[2]), inP[2], parNoParent); res != parPresent {
		t.Fatalf("a stored state in a full partition: %v", res)
	}
	// A state that was refused is not stored and is refused again.
	if _, ok := ps.has(parHash(inP[7]), inP[7]); ok {
		t.Fatal("a refused state is stored")
	}
	// The other partitions are unaffected.
	for i := 0; ; i++ {
		v := []byte{byte(i), 1, 1, 1}
		if parPartOf(parHash(v)) != 3 {
			if _, res := ps.add(parHash(v), v, parNoParent); res != parAdded {
				t.Fatalf("another partition: %v", res)
			}
			break
		}
	}
	if parMaxPerPartition != 1<<24-1 {
		t.Fatalf("parMaxPerPartition = %d, the id format needs 2^24-1", parMaxPerPartition)
	}
	if id := uint32(parPartitions-1)<<parIndexBits | uint32(parMaxPerPartition-1); id == parNoParent {
		t.Fatalf("the largest id %#x is the parent sentinel", id)
	}
}

func TestPartSetGrowsAcrossTheTableAndChunkBoundaries(t *testing.T) {
	// All vectors in one partition so that one table grows many times and one
	// arena crosses many chunks; every id keeps naming its vector.
	ps := newPartSet(24)
	var vecs [][]byte
	var ids []uint32
	for i := 0; len(vecs) < 12000; i++ {
		v := make([]byte, 24)
		binary.LittleEndian.PutUint64(v, uint64(i)*0x9E3779B97F4A7C15)
		if parPartOf(parHash(v)) != 17 {
			continue
		}
		id, res := ps.add(parHash(v), v, uint32(len(vecs)))
		if res != parAdded {
			t.Fatalf("vector %d: %v", len(vecs), res)
		}
		vecs = append(vecs, v)
		ids = append(ids, id)
	}
	for i, v := range vecs {
		if string(ps.get(ids[i])) != string(v) || ps.parentOf(ids[i]) != uint32(i) {
			t.Fatalf("vector %d lost after growth", i)
		}
		if id, ok := ps.has(parHash(v), v); !ok || id != ids[i] {
			t.Fatalf("vector %d not found after growth", i)
		}
	}
	// Ids ascend in arrival order within the partition.
	if !sort.SliceIsSorted(ids, func(a, b int) bool { return ids[a] < ids[b] }) {
		t.Fatal("ids of one partition do not ascend in arrival order")
	}
}
