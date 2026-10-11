//go:build !(linux || darwin || freebsd || dragonfly)

package tmpspace

// freeMB is unknown on this platform (Windows, Solaris, illumos, AIX,
// NetBSD, OpenBSD): the probe relies on the write alone.
func freeMB(string) int64 { return -1 }
