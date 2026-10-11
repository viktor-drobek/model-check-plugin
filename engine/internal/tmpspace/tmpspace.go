// Package tmpspace checks that a scratch directory can hold the temporary
// files of a run before the run starts.
//
// SPIN and gcc write pan.c, its assembler output and the pan binary into the
// temporary directory; go test writes its own work directory and test log
// there too. When that directory cannot take the files, the failure arrives
// half-way through a long run, from gcc, and reads like a defect of the model
// or of the engine. The check here fails first and says what to do.
//
// Free space alone is not enough. A tmpfs mounted with a user quota reports
// gigabytes free to statfs and df while every write fails with "disk quota
// exceeded"; the same holds for project quotas on ext4 and xfs. Probe
// therefore writes a file of the requested size and keeps it only for as long
// as the write takes. The probe is not a reservation: another process can
// take the space afterwards.
package tmpspace

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"strconv"
	"syscall"
)

// Report describes one probe: what was asked, what the file system said and
// what was actually written.
type Report struct {
	Dir       string // the directory probed
	FreeMB    int64  // free space statfs reports, -1 where unknown
	NeedMB    int    // megabytes asked for
	WrittenMB int    // megabytes the probe wrote before it stopped
}

// Error is the failure of a probe. It wraps the cause (a syscall error, a
// permission error) and prints a single line that names the directory, the
// numbers, the cause and the remedy.
type Error struct {
	Report Report
	Cause  error
	reason string
}

func (e *Error) Error() string {
	free := "free space unknown"
	if e.Report.FreeMB >= 0 {
		free = strconv.FormatInt(e.Report.FreeMB, 10) + " MB free"
	}
	return fmt.Sprintf("tmpspace: %s: %s, wrote %d MB of %d MB: %s — set TMPDIR to a directory on disk with at least %d MB",
		e.Report.Dir, free, e.Report.WrittenMB, e.Report.NeedMB, e.reason, e.Report.NeedMB)
}

func (e *Error) Unwrap() error { return e.Cause }

// chunkMB is the size of one write. The buffer holds random bytes: zeros
// would be compressed away on a file system with transparent compression and
// the probe would pass where the real files do not fit.
const chunkMB = 1

// Probe checks that dir is a directory that can hold needMB megabytes: free
// space by statfs where the platform reports it, then a file of needMB
// written in full and closed, then removed. needMB <= 0 only checks that dir
// is a directory. The error, when there is one, is an *Error.
func Probe(dir string, needMB int) (Report, error) {
	r := Report{Dir: dir, FreeMB: -1, NeedMB: needMB}
	st, err := os.Stat(dir)
	if err != nil {
		return r, &Error{Report: r, Cause: err, reason: describe(err)}
	}
	if !st.IsDir() {
		err := fmt.Errorf("%s is not a directory", dir)
		return r, &Error{Report: r, Cause: err, reason: "not a directory"}
	}
	if needMB <= 0 {
		return r, nil
	}
	r.FreeMB = freeMB(dir)
	if r.FreeMB >= 0 && r.FreeMB < int64(needMB) {
		err := fmt.Errorf("%d MB free, %d MB needed", r.FreeMB, needMB)
		return r, &Error{Report: r, Cause: err, reason: "not enough free space"}
	}
	if err := write(dir, needMB, &r); err != nil {
		return r, &Error{Report: r, Cause: err, reason: describe(err)}
	}
	return r, nil
}

// write creates the probe file, fills it chunk by chunk, closes it (a quota
// error can arrive on close on network file systems) and removes it. The file
// is removed on every path; the first error wins.
func write(dir string, needMB int, r *Report) (err error) {
	f, err := os.CreateTemp(dir, ".tmpspace-probe-*")
	if err != nil {
		return err
	}
	defer func() {
		if rmErr := os.Remove(f.Name()); rmErr != nil && err == nil {
			err = rmErr
		}
	}()
	defer func() {
		// Close after a failed write: report the write, not the close.
		if cErr := f.Close(); cErr != nil && err == nil && !errors.Is(cErr, os.ErrClosed) {
			err = cErr
		}
	}()
	buf := make([]byte, chunkMB<<20)
	rng := rand.New(rand.NewPCG(uint64(os.Getpid()), uint64(needMB)))
	for i := range buf {
		buf[i] = byte(rng.Uint32())
	}
	for r.WrittenMB < needMB {
		n, wErr := f.Write(buf)
		if wErr != nil {
			return wErr
		}
		if n != len(buf) {
			return fmt.Errorf("short write of %d bytes", n)
		}
		r.WrittenMB += chunkMB
	}
	// No Sync: the directories this guards are tmpfs or local disk, where the
	// quota and the space are charged at write time, and a sync of hundreds
	// of megabytes would cost more than the check is worth.
	return f.Close()
}

// describe names the cause of a failed write in the words a developer looks
// for: the quota, the full disk, the permission, or the error as it came.
func describe(err error) string {
	switch {
	case errors.Is(err, syscall.EDQUOT):
		return "disk quota exceeded"
	case errors.Is(err, syscall.ENOSPC):
		return "no space left on the device"
	case errors.Is(err, fs.ErrPermission):
		return "not writable"
	case errors.Is(err, fs.ErrNotExist):
		return "does not exist"
	case errors.Is(err, syscall.ENOTDIR):
		return "not a directory"
	case errors.Is(err, syscall.EROFS):
		return "read-only file system"
	}
	return err.Error()
}

// EnvVar names the environment variable that sets the megabytes ProbeEnv
// asks for; "0" switches the probe off.
const EnvVar = "MCD_TMP_MIN_MB"

// DefaultMB is the scratch space a run of the engine's test suite or of a
// SPIN campaign asks for when EnvVar is unset: one pan.c compiles to tens of
// megabytes of assembler, object and binary, and several compile at a time
// under go test -p or a campaign's -jobs. The number is a heuristic with
// room to spare, not a measurement of every corpus.
const DefaultMB = 256

// ProbeEnv probes os.TempDir(), the scratch directory of SPIN, gcc and go
// test, for the amount EnvVar sets, or def when it is unset. An amount of 0
// switches the probe off: nothing is checked, not even that the directory
// exists, so that the variable is a complete bypass.
func ProbeEnv(def int) (Report, error) {
	r := Report{Dir: os.TempDir(), FreeMB: -1}
	need, err := EnvMB(EnvVar, def)
	if err != nil || need == 0 {
		return r, err
	}
	return Probe(r.Dir, need)
}

// EnvMB reads a megabyte count from the environment variable name: unset or
// empty gives def; "0" switches the probe off; anything that is not a
// non-negative integer is an error, not a silent default.
func EnvMB(name string, def int) (int, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s=%q: want a non-negative number of megabytes", name, v)
	}
	return n, nil
}
