//go:build linux || darwin || freebsd || dragonfly

package tmpspace

import "syscall"

// freeMB is the space statfs reports as available to an unprivileged process
// (Bavail, not Bfree), in megabytes, or -1 when the call fails. The field
// types differ between the platforms, hence the conversions; on FreeBSD and
// DragonFly Bavail is signed and goes negative when root has eaten the
// reserved blocks, which counts as nothing free. NetBSD has no
// syscall.Statfs and OpenBSD names the fields differently: they take the
// unknown of free_other.go.
func freeMB(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	mb := int64(st.Bavail) * int64(st.Bsize) >> 20 //nolint:unconvert // Bavail and Bsize are not int64 on every platform
	if mb < 0 {
		return 0
	}
	return mb
}
