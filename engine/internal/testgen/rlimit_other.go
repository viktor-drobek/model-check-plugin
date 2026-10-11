//go:build !(linux || darwin || freebsd)

package testgen

// limitAddressSpace has no portable form here; the timeout still bounds
// the child.
func limitAddressSpace(int) {}
