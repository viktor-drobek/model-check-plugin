package spike

import "fmt"

// Petrinet1 is Holzmann's App_C/petrinet1: six places, six transitions.
// State = one byte per place (p1..p6), exactly the Promela `byte` domain.
// Transitions in the order of the Promela `do` alternatives t1..t6; the
// atomic guard/decrement/increment of the Promela is one step here, which
// matches pan (atomic intermediate states are not stored, see pan -d).
type Petrinet1 struct{}

func NewPetrinet1() *Petrinet1 { return &Petrinet1{} }

func (*Petrinet1) Name() string { return "petrinet1" }

func (*Petrinet1) Initial() []byte { return []byte{1, 0, 0, 1, 0, 0} } // p1 = p4 = 1

type ptTransition struct {
	label   string
	in, out []int // place indices (0-based)
}

var petrinet1Transitions = []ptTransition{
	{"t1", []int{0}, []int{1}},    // inp1(p1) -> out1(p2)
	{"t2", []int{1, 3}, []int{2}}, // inp2(p2,p4) -> out1(p3)
	{"t3", []int{2}, []int{0, 3}}, // inp1(p3) -> out2(p1,p4)
	{"t4", []int{3}, []int{4}},    // inp1(p4) -> out1(p5)
	{"t5", []int{0, 4}, []int{5}}, // inp2(p1,p5) -> out1(p6)
	{"t6", []int{5}, []int{3, 0}}, // inp1(p6) -> out2(p4,p1)
}

func (*Petrinet1) Successors(s []byte, fn func(string, []byte, string)) {
	var next [6]byte
	for _, t := range petrinet1Transitions {
		enabled := true
		for _, p := range t.in {
			if s[p] == 0 {
				enabled = false
				break
			}
		}
		if !enabled {
			continue
		}
		copy(next[:], s)
		for _, p := range t.in {
			next[p]--
		}
		for _, p := range t.out {
			next[p]++ // byte overflow (>255) is not checked in the spike
		}
		fn(t.label, next[:], "")
	}
}

func (*Petrinet1) Describe(s []byte) string {
	out := ""
	for i, v := range s {
		if v != 0 {
			if out != "" {
				out += " "
			}
			out += fmt.Sprintf("p%d=%d", i+1, v)
		}
	}
	if out == "" {
		return "empty"
	}
	return out
}

func (*Petrinet1) Vars(s []byte) []Var {
	vs := make([]Var, 6)
	for i, v := range s {
		vs[i] = Var{Name: fmt.Sprintf("p%d", i+1), Value: int(v)}
	}
	return vs
}
