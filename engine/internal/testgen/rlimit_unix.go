//go:build linux || darwin || freebsd

package testgen

import "syscall"

// limitAddressSpace caps the virtual memory of the process, so that a case
// the limits miss dies of its own allocation instead of taking the machine
// into swap (the resources rule). A failure to set it is ignored: the cap
// is a safety net, not the thing under test.
func limitAddressSpace(mb int) {
	if mb <= 0 {
		return
	}
	lim := &syscall.Rlimit{Cur: uint64(mb) << 20, Max: uint64(mb) << 20}
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, lim)
}
