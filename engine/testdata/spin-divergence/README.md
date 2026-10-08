# Documented divergences from SPIN

Models on which this engine and SPIN 6.5.2 give different answers, kept so that
the difference is stated and pinned rather than rediscovered. A scenario of
`features/g5-ctl-v1.feature` (tagged `@spin`) runs each of them through
`tools/pandiff` and asserts the divergence itself: when it stops holding, the
scenario fails and the documentation (the `_nr_pr` row of
`skills/model-check/references/promela-subset.md`, and
`steps/fix-nrpr-confirmation.md`) has to be changed with it.

`nrpr-*.pml`: pan counts the `never` claim, and an LTL formula checked with
`-a` (which is a claim too), as a process in `_nr_pr`; this engine counts only
the model's own processes. A verdict that reads `_nr_pr` and is checked with a
claim can therefore differ. Without a claim they agree.
