package spike

import "fmt"

// MutexFlaw is CH2/mutex_flaw.pml encoded by hand from the source and checked
// against pan -d (spin -a -o1 -o2 -o3): the local `byte me = _pid + 1` is
// initialised at process creation and is not a transition; each `goto L1` is
// folded into the guard transition that precedes it; `cnt++`, `assert`,
// `cnt--` are three separate global transitions.
//
// State layout (6 bytes): cnt, x, y, z, pc[0], pc[1]. `me` is pid+1 and is
// not stored (it is constant per process). pan's state vector is 28 bytes
// because it also carries process headers and the two `me` locals.
//
// Control locations:
//
//	L1: x = me                       -> L2
//	L2: (y!=0 && y!=me) -> L1 | (y==0 || y==me) -> L3
//	L3: z = me                       -> L4
//	L4: (x!=me) -> L1 | (x==me)      -> L5
//	L5: y = me                       -> L6
//	L6: (z!=me) -> L1 | (z==me)      -> L7
//	L7: cnt++                        -> L8
//	L8: assert(cnt == 1)             -> L9
//	L9: cnt--                        -> L1
type MutexFlaw struct{}

func NewMutexFlaw() *MutexFlaw { return &MutexFlaw{} }

func (*MutexFlaw) Name() string { return "mutex_flaw" }

const (
	mCnt = iota
	mX
	mY
	mZ
	mPC0
	mPC1
	mLen
)

const mutexProcs = 2

// Initial: all variables 0, both processes at L1 (pc = 1).
func (*MutexFlaw) Initial() []byte { return []byte{0, 0, 0, 0, 1, 1} }

// Successors: processes in pid order (0, 1), alternatives in source order.
// Each `if` alternative is its own transition; exactly one is enabled at any
// state because the guards are complementary.
func (*MutexFlaw) Successors(s []byte, fn func(string, []byte, string)) {
	var next [mLen]byte
	for pid := 0; pid < mutexProcs; pid++ {
		me := byte(pid + 1)
		pcIdx := mPC0 + pid
		pc := s[pcIdx]
		step := func(label string, to byte, mutate func(n []byte), violated string) {
			copy(next[:], s)
			if mutate != nil {
				mutate(next[:])
			}
			next[pcIdx] = to
			fn(mutexLabel(pid, label), next[:], violated)
		}
		switch pc {
		case 1:
			step("L1: x = me", 2, func(n []byte) { n[mX] = me }, "")
		case 2:
			if s[mY] != 0 && s[mY] != me {
				step("L2: (y != 0 && y != me) -> goto L1", 1, nil, "")
			} else {
				step("L2: (y == 0 || y == me)", 3, nil, "")
			}
		case 3:
			step("L3: z = me", 4, func(n []byte) { n[mZ] = me }, "")
		case 4:
			if s[mX] != me {
				step("L4: (x != me) -> goto L1", 1, nil, "")
			} else {
				step("L4: (x == me)", 5, nil, "")
			}
		case 5:
			step("L5: y = me", 6, func(n []byte) { n[mY] = me }, "")
		case 6:
			if s[mZ] != me {
				step("L6: (z != me) -> goto L1", 1, nil, "")
			} else {
				step("L6: (z == me)", 7, nil, "")
			}
		case 7:
			step("L7: cnt++", 8, func(n []byte) { n[mCnt]++ }, "")
		case 8:
			violated := ""
			if s[mCnt] != 1 {
				violated = "assert(cnt == 1)"
			}
			step("assert(cnt == 1)", 9, nil, violated)
		case 9:
			step("cnt--; goto L1", 1, func(n []byte) { n[mCnt]-- }, "")
		default:
			panic(fmt.Sprintf("mutex_flaw: bad pc %d", pc))
		}
	}
}

// mutexLabel prefixes a statement with its process; the table avoids fmt on
// the hot path.
var mutexLabels [mutexProcs]map[string]string

func mutexLabel(pid int, stmt string) string {
	if mutexLabels[pid] == nil {
		mutexLabels[pid] = map[string]string{}
	}
	if l, ok := mutexLabels[pid][stmt]; ok {
		return l
	}
	l := fmt.Sprintf("user[%d]:%s", pid, stmt)
	mutexLabels[pid][stmt] = l
	return l
}

func (*MutexFlaw) Describe(s []byte) string {
	return fmt.Sprintf("cnt=%d x=%d y=%d z=%d pc=[L%d L%d]", s[mCnt], s[mX], s[mY], s[mZ], s[mPC0], s[mPC1])
}

func (*MutexFlaw) Vars(s []byte) []Var {
	return []Var{
		{"cnt", int(s[mCnt])}, {"x", int(s[mX])}, {"y", int(s[mY])}, {"z", int(s[mZ])},
		{"pc0", int(s[mPC0])}, {"pc1", int(s[mPC1])},
	}
}
