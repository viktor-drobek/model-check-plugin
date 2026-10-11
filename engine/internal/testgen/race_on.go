//go:build race

package testgen

// raceEnabled: the race detector reserves address space of its own, so a
// child process must not be capped (the resources rule: no ulimit -v under
// -race); the timeout still bounds it.
const raceEnabled = true
