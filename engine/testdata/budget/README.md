# Models of the time-budget scenarios (issue #8)

These models are built to run past any time budget (2^40 paths through one
atomic block, nothing stored) and are kept out of testdata/promela on purpose:
the corpus comparisons there expect runs that finish or stop at the same point
for every search mode, which a stop on the clock does not.
