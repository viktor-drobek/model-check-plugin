package pandiff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"modelcheck/explore"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// Differential triple (G4): for one model, one property and one fairness
// setting, three verdicts that must agree —
//
//	Engine     the engine with its own LTL → Büchi automaton for the
//	           formula (or, without a formula, the model's never claim /
//	           accept labels for mode "a", the np_ automaton for mode "l");
//	Claim      the engine running SPIN's never claim for the formula
//	           (`spin -f '!(formula)'` appended to the model; the model's own
//	           never claim is removed first) — isolates the translation from
//	           the product; "n/a" when there is no formula or mode is "l";
//	Pan        pan itself: `spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE
//	           [-DNP]`, `./pan -a [-f] -c0` or `./pan -l [-f] -c0`.
//
// A verdict is "violated" (an acceptance / non-progress cycle, a claim
// end or a claim assert; pan: errors > 0), "verified" (pan: errors = 0
// with a completed search), or, for the engine, "inconclusive: …".
type Triple struct {
	Model, Formula, Fairness, Mode string
	Engine, Claim, Pan             string
	PanClass                       string
	EngineStates, ClaimStates      int
	PanStates                      int
	Agree                          bool
	Note                           string
}

// RunTriple computes the triple. model is a path; defines are -D symbols.
func RunTriple(ctx context.Context, tools Tools, model string, defines []string, formula, fairness, mode string) (*Triple, error) {
	t := &Triple{Model: model, Formula: formula, Fairness: fairness, Mode: mode, Claim: "n/a"}
	src, err := os.ReadFile(model)
	if err != nil {
		return nil, err
	}
	if fairness == "" {
		fairness = "none"
	}
	// (i) engine with its own automaton.
	base, perr := promela.Parse(src, model, defines)
	if perr != nil {
		return nil, perr
	}
	props := engineProps(base.Model, formula, mode)
	hasClaim := false
	for _, p := range base.Model.Processes {
		if p.Claim {
			hasClaim = true
		}
	}
	t.Engine, t.EngineStates, err = engineVerdict(ctx, base.Model, base.Defines, props, fairness, formula == "" && mode == "a" && !hasClaim)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "triple-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	// The defines go into the pan copy as #define lines: spin passes -D to
	// cpp through a shell, which breaks on values with spaces or parentheses.
	panSrc := src
	if len(defines) > 0 {
		var b strings.Builder
		for _, d := range defines {
			name, val := d, "1"
			if i := strings.IndexByte(d, '='); i >= 0 {
				name, val = d[:i], d[i+1:]
			}
			fmt.Fprintf(&b, "#define %s %s\n", name, val)
		}
		panSrc = append([]byte(b.String()), src...)
	}
	if formula != "" && mode == "a" {
		claim, err := run(ctx, dir, tools.spin(), "-f", "!("+formula+")")
		if err != nil || !strings.Contains(claim, "never") {
			return nil, fmt.Errorf("spin -f failed: %s%s", firstLines(claim, 3), errNote(err))
		}
		panSrc = append(stripNever(panSrc), []byte("\n"+claim)...)
		// (ii) engine with SPIN's claim.
		withClaim, perr := promela.Parse(panSrc, model, defines)
		if perr != nil {
			return nil, fmt.Errorf("model with SPIN's claim: %v", perr)
		}
		t.Claim, t.ClaimStates, err = engineVerdict(ctx, withClaim.Model, withClaim.Defines, []ir.Property{{ID: "never", Kind: ir.KindLTL}}, fairness, false)
		if err != nil {
			return nil, err
		}
	}
	// (iii) pan.
	local := filepath.Join(dir, "m.pml")
	if err := os.WriteFile(local, panSrc, 0o644); err != nil {
		return nil, err
	}
	out, err := run(ctx, dir, tools.spin(), "-a", "-o1", "-o2", "-o3", "m.pml")
	if err != nil || strings.Contains(out, "Error") {
		return nil, fmt.Errorf("spin -a failed: %s%s", firstLines(out, 3), errNote(err))
	}
	gccArgs := []string{"-O2", "-DNOREDUCE"}
	if mode == "l" {
		gccArgs = append(gccArgs, "-DNP")
	}
	gccArgs = append(gccArgs, "-o", "pan", "pan.c")
	if out, err := run(ctx, dir, tools.gcc(), gccArgs...); err != nil {
		return nil, fmt.Errorf("gcc failed: %s%s", firstLines(out, 3), errNote(err))
	}
	panArgs := []string{"-" + mode, "-c0", "-m100000"}
	if fairness == "weak" {
		panArgs = append(panArgs, "-f")
	}
	c0, _ := run(ctx, dir, filepath.Join(dir, "pan"), panArgs...)
	stored, _, errors, perr2 := ParseC0(c0)
	if perr2 != nil {
		return nil, fmt.Errorf("pan: %v\n%s", perr2, firstLines(c0, 8))
	}
	t.PanStates = stored
	first, _ := run(ctx, dir, filepath.Join(dir, "pan"), panArgs[:len(panArgs)-2]...)
	_, t.PanClass = ParseFirst(first, errors)
	switch {
	case errors > 0:
		t.Pan = "violated"
	case strings.Contains(c0, "Search not completed") || strings.Contains(c0, "max search depth too small"):
		t.Pan = "inconclusive: pan search not completed"
	default:
		t.Pan = "verified"
	}
	word := func(v string) string { return strings.SplitN(v, " ", 2)[0] }
	t.Agree = word(t.Engine) == t.Pan && (t.Claim == "n/a" || word(t.Claim) == t.Pan)
	return t, nil
}

func engineProps(m *ir.Model, formula, mode string) []ir.Property {
	if mode == "l" {
		return []ir.Property{{ID: "progress", Kind: ir.KindProgress}}
	}
	if formula != "" {
		return []ir.Property{{ID: "f", Kind: ir.KindLTL, Formula: formula}}
	}
	for _, p := range m.Properties {
		if p.Kind == ir.KindLTL {
			return []ir.Property{p}
		}
	}
	return []ir.Property{{ID: "accept", Kind: ir.KindLTL}}
}

// engineVerdict runs the temporal property and, when foldAssert is set,
// the model's assert property too: pan -a reports assertion violations as
// errors "if within scope of claim", i.e. over the whole state space only
// when there is no claim. The engine's safety search checks asserts over
// the whole state space regardless of any claim (stricter than pan), so
// the fold applies only to a model without a claim checked as written.
func engineVerdict(ctx context.Context, m *ir.Model, defines map[string]string, props []ir.Property, fairness string, foldAssert bool) (string, int, error) {
	mm := *m
	mm.Properties = props
	if foldAssert {
		for _, p := range m.Properties {
			if p.Kind == ir.KindAssert {
				mm.Properties = append(mm.Properties, p)
			}
		}
	}
	res, err := explore.Run(ctx, &mm, explore.Options{Budget: explore.Budget{MaxStates: 5_000_000}, Fairness: fairness, Defines: defines, Sweep: true})
	if err != nil {
		return "", 0, err
	}
	verdict, states := "", 0
	assertViolated := false
	for _, o := range res.Outcomes {
		if foldAssert && o.Property.Kind == ir.KindAssert && o.Status == explore.Violated {
			assertViolated = true // the explorer adds `assert` implicitly; it counts only when folded
		}
		if o.Property.ID != props[0].ID {
			continue
		}
		states = res.States
		if o.Stats != nil {
			states = o.Stats.States
		}
		switch o.Status {
		case explore.Violated, explore.Verified:
			verdict = string(o.Status)
		default:
			verdict = string(o.Status) + ": " + o.Reason
		}
	}
	if verdict == "" {
		return "", 0, fmt.Errorf("no outcome for %s", props[0].ID)
	}
	if verdict == "verified" && assertViolated {
		verdict = "violated (assert)"
	}
	return verdict, states, nil
}

var neverStart = regexp.MustCompile(`(?m)^\s*never\s*\{`)

// stripNever removes every `never { … }` block (brace-balanced) so that
// SPIN's generated claim can be appended; the #ifdef lines around a
// removed block stay and select nothing.
func stripNever(src []byte) []byte {
	s := string(src)
	for {
		loc := neverStart.FindStringIndex(s)
		if loc == nil {
			return []byte(s)
		}
		depth := 0
		end := -1
		for i := loc[1] - 1; i < len(s); i++ {
			switch s[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i + 1
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return []byte(s)
		}
		s = s[:loc[0]] + s[end:]
	}
}

// Row renders the triple as one table line.
func (t *Triple) Row() string {
	mark := "agree"
	if !t.Agree {
		mark = "DISAGREE"
	}
	f := t.Formula
	if f == "" {
		f = "(model as written)"
	}
	claim := t.Claim
	if t.Claim != "n/a" {
		claim = fmt.Sprintf("%s (%d)", t.Claim, t.ClaimStates)
	}
	return fmt.Sprintf("%-32s %-48s %-5s %-2s | engine %s (%d) | engine+SPIN claim %s | pan %s (%d, %s) | %s%s",
		filepath.Base(t.Model), f, t.Fairness, t.Mode, t.Engine, t.EngineStates, claim, t.Pan, t.PanStates, t.PanClass, mark, noteSuffix(t.Note))
}

func noteSuffix(n string) string {
	if n == "" {
		return ""
	}
	return " (" + n + ")"
}
