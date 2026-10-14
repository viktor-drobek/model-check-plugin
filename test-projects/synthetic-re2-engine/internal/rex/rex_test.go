package rex

import "testing"

func TestFindRE2LikeCases(t *testing.T) {
	for _, tc := range []struct {
		p, s string
		want [2]int
	}{
		{"a|bc", "xxbc", [2]int{2, 4}}, {"(a+)(b)", "aaab", [2]int{0, 4}},
		{"ab*c", "ac", [2]int{0, 2}}, {"[a-c]+", "xxabc", [2]int{2, 5}},
		{"^a.$", "ab", [2]int{0, 2}}, {"\\d+", "x42", [2]int{1, 3}},
	} {
		p, err := Compile(tc.p)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tc.p, err)
		}
		got := p.Find([]byte(tc.s))
		if !got.Matched || got.Span != tc.want {
			t.Errorf("%q on %q = %+v, want %v", tc.p, tc.s, got, tc.want)
		}
	}
}
func TestCapturesAndFindAll(t *testing.T) {
	p, err := Compile("(a)(b)?")
	if err != nil {
		t.Fatal(err)
	}
	m := p.Find([]byte("ab"))
	if m.Groups[1] != [2]int{0, 1} || m.Groups[2] != [2]int{1, 2} {
		t.Fatalf("groups=%v", m.Groups)
	}
	p, _ = Compile("a*")
	if got := p.FindAll([]byte("bb"), 10); len(got) != 3 {
		t.Fatalf("find all=%v", got)
	}
}
func TestRejectUnsupported(t *testing.T) {
	for _, p := range []string{"a{2}", "(?=a)", "("} {
		if _, err := Compile(p); err == nil {
			t.Errorf("%q accepted", p)
		}
	}
}
