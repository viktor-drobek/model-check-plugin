package mutate

// SubsetCampaign is the list of corpus models the K3 mutation run uses, with
// the checks that are natural to each of them.
//
// Membership is not a judgement of this step: it is the differential table of
// steps/g1-confirmation.md §3.1 — every model of chapters 2–3 that the G1
// frontend accepted — extended, as the step brief allows, to the chapter 4,
// chapter 8, App_A and App_C models the G4 triples of
// steps/g4-confirmation.md §3.1 ran. Models that G1 recorded as
// outside-subset or as rejected by SPIN are absent; models that G1 recorded
// as `disagree-explained` (byte overflow: the engine answers `invalid-model`
// where pan silently wraps, plan §4.1) ARE present, because leaving them out
// would quietly improve the rate — their mutants fall into class (iv) on
// their own, with the reason printed.
//
// Which checks a model gets:
//
//   - safety (`mcd check` vs plain `pan`): every model WITHOUT a never
//     claim. With a claim, `pan` disables invalid-end-state checking and
//     limits assertions to the claim's scope, so the two sides would be
//     answering different questions; those models are checked with `pan -a`
//     instead.
//   - acceptance (`pan -a`): a model with a never claim or accept labels.
//   - non-progress (`pan -l`): a model with progress labels.
//   - an LTL formula: where the G4 triple table gives one for the model
//     (steps/g4-confirmation.md §3.1). One formula per model — the mutation
//     run multiplies models by mutants by checks, and a second formula buys
//     less than a second model.
//
// Fairness is left at `none`: the G4 table shows engine and pan agreeing on
// the verdict under weak fairness but not on the state count, and the class
// of a mutant is decided on verdicts only, so the extra runs would not change
// a class.
func SubsetCampaign() []ModelSpec {
	safety := []Check{{Name: "safety"}}
	spec := func(path string, checks ...Check) ModelSpec {
		return ModelSpec{Path: path, Checks: checks}
	}
	return []ModelSpec{
		// Chapter 2 (G1 §3.1: all agree).
		spec("CH2/mutex_flaw.pml", safety...),
		spec("CH2/peterson.pml", safety...),
		spec("CH2/peterson2.pml", safety...),
		spec("CH2/prodcons.pml", safety...),
		spec("CH2/mutex.pml", safety...),
		spec("CH2/protocol", safety...),
		spec("CH2/protocol2", safety...),
		spec("CH2/false.pml", safety...),
		spec("CH2/hello.pml", safety...),
		spec("CH2/hello2.pml", safety...),
		// Chapter 3 (G1 §3.1: agree, plus the three byte-overflow models).
		spec("CH3/alternatingbit.pml", safety...),
		spec("CH3/alternatingbit2.pml", safety...),
		spec("CH3/counter3.pml", safety...),
		spec("CH3/counter4.pml", safety...),
		spec("CH3/euclid.pml", safety...),
		spec("CH3/macro.pml", safety...),
		spec("CH3/mtype.pml", safety...),
		spec("CH3/rendezvous.pml", safety...),
		spec("CH3/send_recv.pml", safety...),
		spec("CH3/you_run.pml", safety...),
		spec("CH3/you_run2.pml", safety...),
		{Path: "CH3/counter.pml", Checks: safety,
			Note: "G1 §3.1 disagree-explained: `count--` at 0 is invalid-model for the engine, wrap-around for pan"},
		{Path: "CH3/counter2.pml", Checks: safety,
			Note: "G1 §3.1 disagree-explained: `count++` at 255"},
		{Path: "CH3/xr.pml", Checks: safety,
			Note: "G1 §3.1 disagree-explained: `s++` at 255"},
		// Chapter 4 (G4 §3.1).
		spec("CH4/dijkstra.pml", safety...),
		spec("CH4/dijkstra_progress.pml",
			Check{Name: "safety"},
			Check{Mode: "l"},
			Check{Mode: "a", Formula: "[]<>(len(sema) == 0)"}),
		spec("CH4/fair.pml",
			Check{Name: "safety"},
			Check{Mode: "l"},
			Check{Mode: "a", Formula: "[]<>(x == 1)"}),
		spec("CH4/fair_accept.pml",
			Check{Name: "safety"},
			Check{Mode: "a"}),
		spec("CH4/true.pml", safety...),
		spec("CH4/false.pml", safety...),
		{Path: "CH4/prop.pml", Checks: []Check{{Mode: "a", Formula: "[]p"}},
			Note: "always carries a never claim (one of two #ifdef branches), so there is no safety run"},
		// Chapter 8 (G4 §3.1).
		spec("CH8/example.pml", safety...),
		spec("CH8/fairness.pml",
			Check{Name: "safety"},
			Check{Mode: "a"}),
		{Path: "CH8/trivial.pml", Checks: []Check{{Mode: "a"}, {Mode: "a", Formula: "[]<>x"}},
			Note: "carries a never claim, so there is no safety run"},
		// Appendix A (G4 §3.1) and appendix C (G1 §3.1).
		{Path: "App_A/example", Checks: []Check{{Mode: "a"}, {Mode: "a", Formula: "<>[]p"}},
			Note: "carries a never claim, so there is no safety run"},
		spec("App_C/petrinet1", safety...),
		spec("App_C/petrinet2", safety...),
	}
}
