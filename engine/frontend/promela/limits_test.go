package promela

import (
	"strings"
	"testing"
	"time"

	"modelcheck/internal/testgen"
)

// parseMsg runs the whole frontend on src and returns the rejection's
// message, or "ok".
func parseMsg(src string) string {
	_, err := Parse([]byte(src), "limit.pml", nil)
	if err == nil {
		return "ok"
	}
	return err.Kind + ": " + err.Message
}

// TestLimitsCrashScale runs the inputs of issue #9 at the size that used to
// end the process (stack overflow, out of memory) in a child process with a
// memory cap: every one must be answered by a limit, fast.
func TestLimitsCrashScale(t *testing.T) {
	// The address-space cap is a safety net against a case the limits miss;
	// Go reserves arenas generously, so it sits well above what a caught
	// case allocates (a 4 MB line lexes into hundreds of megabytes of tokens).
	testgen.RunInChild(t, "TestLimitsCrashScale", 6000, 90*time.Second, []testgen.Case{
		{Name: "nested-parens-600000", Want: "nested or chained deeper than 1000", Run: func() string { return parseMsg(testgen.NestedParens(600000)) }},
		{Name: "plus-chain-1000000", Want: "nested or chained deeper than 1000", Run: func() string { return parseMsg(testgen.PlusChain(1000000)) }},
		{Name: "paren-chain-499", Want: "nested or chained deeper than 1000", Run: func() string { return parseMsg(testgen.ParenChain(499, 499)) }},
		{Name: "nested-if-100000", Want: "statements nested deeper than 1000", Run: func() string { return parseMsg(testgen.NestedIf(100000)) }},
		{Name: "if-parens-1000000", Want: "#if expression nested deeper than 1000", Run: func() string { return parseMsg(testgen.IfParens(1000000)) }},
		{Name: "define-doubling-23", Want: "macro expansion of more than 4000000 tokens", Run: func() string { return parseMsg(testgen.DefineDoubling(23)) }},
		{Name: "define-chain-50000", Want: "macro expansion nested deeper than 1000", Run: func() string { return parseMsg(testgen.DefineChain(50000)) }},
		{Name: "define-fanout-20000", Want: "macro expansion of more than 4000000 tokens", Run: func() string { return parseMsg(testgen.DefineFanout(20000, 20000)) }},
		{Name: "inline-doubling-22", Want: "inline expansion of more than 4000000 tokens", Run: func() string { return parseMsg(testgen.InlineDoubling(22)) }},
		{Name: "inline-fanout-20000", Want: "inline expansion of more than 4000000 tokens", Run: func() string { return parseMsg(testgen.InlineFanout(20000, 20000)) }},
		{Name: "typedef-fanout-16x100", Want: "flatten to more than 100000 fields", Run: func() string { return parseMsg(testgen.TypedefFanout(16, 100)) }},
		{Name: "index-nest-600000", Want: "nested or chained deeper than 1000", Run: func() string { return parseMsg(testgen.IndexNest(600000)) }},
		{Name: "pcvalue-nest-600000", Want: "nested or chained deeper than 1000", Run: func() string { return parseMsg(testgen.PCValueNest(600000)) }},
		{Name: "huge-array-init", Want: "the limit is 65535", Run: func() string { return parseMsg(testgen.HugeArrayInit()) }},
		{Name: "many-arrays-2000", Want: "state vector", Run: func() string { return parseMsg(testgen.ManyArrays(2000)) }},
		{Name: "many-arrays-init-7000", Want: "state vector", Run: func() string { return parseMsg(testgen.ManyArraysInit(7000)) }},
		{Name: "run-locals-init-300", Want: "state vector", Run: func() string { return parseMsg(testgen.RunLocalsInit(300)) }},
		{Name: "chan-arrays-1000", Want: "the limit is 255", Run: func() string { return parseMsg(testgen.ChanArrays(1000)) }},
		{Name: "wide-chans-250x1000", Want: "state vector", Run: func() string { return parseMsg(testgen.WideChans(250, 1000)) }},
		{Name: "wide-zero-chans-255x1000000", Want: "state vector", Run: func() string { return parseMsg(testgen.WideZeroChans(255, 1000000)) }},
		{Name: "active-254-long-body", Want: "tokens together", Run: func() string { return parseMsg(testgen.ActiveLongBody(254, 200000)) }},
		{Name: "run-init-locals-253", Want: "state vector", Run: func() string { return parseMsg(testgen.RunInitLocals(253)) }},
		{Name: "typedef-chain-100000", Want: "field names", Run: func() string { return parseMsg(testgen.TypedefChain(100000)) }},
		// Within the expansion budget; the rescan must stay linear (it used
		// to copy the result once per wrapper and run for minutes), and the
		// 2^18 leaves are then refused by the IR weight.
		{Name: "define-wrappers-18x970", Want: "limit", Run: func() string { return parseMsg(testgen.DefineWrappers(18, 970)) }},
		{Name: "chains-100x999", Want: "the IR weighs", Run: func() string { return parseMsg(testgen.Chains(100, 999)) }},
	})
}

// TestLimitsBoundary pins where each limit sits: the last accepted size
// parses, the next one is refused as outside-subset with the limit named.
func TestLimitsBoundary(t *testing.T) {
	cases := []struct {
		name   string
		ok     string
		refuse string
		want   string
	}{
		{"parens", testgen.NestedParens(1000), testgen.NestedParens(1001), "nested or chained deeper than 1000"},
		{"plus-chain", testgen.PlusChain(1000), testgen.PlusChain(1001), "nested or chained deeper than 1000"},
		// `-1` folds into the number, so 1000 prefix operators build 999
		// nodes over the leaf: height 1000, the last accepted.
		{"prefix-chain", testgen.PrefixChain(1000), testgen.PrefixChain(1001), "nested or chained deeper than 1000"},
		{"nested-if", testgen.NestedIf(200), testgen.NestedIf(1001), "statements nested deeper than 1000"},
		{"if-parens", testgen.IfParens(1000), testgen.IfParens(1001), "#if expression nested deeper than 1000"},
		{"wide-mixed", testgen.WideMixed(400), "", ""},
		{"define-doubling", testgen.DefineDoubling(10), testgen.DefineDoubling(23), "macro expansion of more than"},
		// 14 doubling defines expand to about 130 000 tokens: well under the
		// budget, and charged once per token, not once per level (16 levels
		// pass the budget too, but their IR weight is over the line).
		{"define-doubling-14", testgen.DefineDoubling(14), "", ""},
		{"index-nest", testgen.IndexNest(999), testgen.IndexNest(1001), "nested or chained deeper than 1000"},
		// One run-created proctype with three arrays (196 605 slots) is
		// counted for one instance, not for a pool of eight.
		{"run-locals", "proctype P() { int a[65535]; int b[65535]; int c[65535]; a[0] = b[0] + c[0] }\ninit { run P() }\n", "", ""},
		// A typedef definition declares no storage: an instance just under
		// the limit passes; three instances of it do not.
		{"typedef-storage", "typedef T { int a0[65535]; int a1[65535]; int a2[65535]; int a3[65535]; int a4[65535]; int a5[65535]; int a6[65535]; int a7[65535]; int a8[65535] }\nT t;\nactive proctype P() { t.a0[0] = 1 }\n",
			"typedef T { int a0[65535]; int a1[65535]; int a2[65535]; int a3[65535]; int a4[65535]; int a5[65535]; int a6[65535]; int a7[65535]; int a8[65535] }\nT t; T u; T v;\nactive proctype P() { t.a0[0] = 1 }\n", "state vector"},
		{"many-arrays", testgen.ManyArrays(15), testgen.ManyArrays(17), "state vector"},
		{"chan-array", "chan q[255] = [0] of { byte };\nactive proctype P() { q[0]!1 }\n", "chan q[256] = [0] of { byte };\nactive proctype P() { q[0]!1 }\n", "the limit is 255"},
		{"typedef-fanout", testgen.TypedefFanout(4, 4), testgen.TypedefFanout(12, 40), "flatten to more than 100000 fields"},
		{"chains", testgen.Chains(3, 999), testgen.Chains(100, 999), "the IR weighs"},
	}
	for _, c := range cases {
		if c.ok == "" {
			continue
		}
		if got := parseMsg(c.ok); got != "ok" {
			t.Errorf("%s: the accepted size is refused: %s", c.name, got)
		}
		if c.refuse == "" {
			continue
		}
		got := parseMsg(c.refuse)
		if !strings.HasPrefix(got, KindOutside+": ") || !strings.Contains(got, c.want) || !strings.Contains(got, "limit") {
			t.Errorf("%s: refused as %q, want outside-subset mentioning %q and the limit", c.name, got, c.want)
		}
	}
}

// TestMaxProcsIsBounded: a pool larger than the engine can name is refused
// before it is built, whatever the caller asks for.
func TestMaxProcsIsBounded(t *testing.T) {
	src := "proctype P() { skip }\nactive proctype Q() { do :: run P() od }\n"
	_, err := ParseWith([]byte(src), "pool.pml", nil, Options{MaxProcs: 1000000000})
	if err == nil || !strings.Contains(err.Message, "254") {
		t.Fatalf("max-procs of a billion: %v, want a refusal naming 254", err)
	}
	if _, err := ParseWith([]byte(src), "pool.pml", nil, Options{MaxProcs: 254}); err == nil {
		// 254 pool instances plus the active Q exceed the 254 the engine
		// names: refused by the instance count, not by a crash.
		t.Fatal("255 instances accepted")
	}
	if _, err := ParseWith([]byte(src), "pool.pml", nil, Options{MaxProcs: 200}); err != nil {
		t.Fatalf("a pool of 200: %v", err)
	}
}

// TestHeightsAreRecordedForEveryNode walks the corpus-like shapes and the
// operators of the subset through the parser: a construction site the
// height bookkeeping missed would surface as the internal error of built.
func TestHeightsAreRecordedForEveryNode(t *testing.T) {
	srcs := []string{
		testgen.Model("x = -(1 + 2) * 3 % 4 - !x"),
		"chan c = [2] of { byte }; byte a[3];\nactive proctype P() { byte y; c!1; (len(c) > 0 && !full(c) && empty(c) == 0) -> a[y+1] = pc_value(0) + _nr_pr; (a[0] != a[1] || a[2] >= -y) -> skip }\n",
		testgen.WideMixed(50),
		testgen.ParenChain(5, 5),
	}
	for _, s := range srcs {
		if got := parseMsg(s); got != "ok" {
			t.Errorf("%s\n→ %s", s, got)
		}
	}
}
