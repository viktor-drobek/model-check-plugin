package tmpspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestProbeWritesAndRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	r, err := Probe(dir, 1)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if r.WrittenMB != 1 || r.NeedMB != 1 || r.Dir != dir {
		t.Errorf("report %+v, want 1 MB written in %s", r, dir)
	}
	if r.FreeMB != -1 && r.FreeMB < 1 {
		t.Errorf("FreeMB = %d, want the free space of %s (at least 1) or -1 for unknown", r.FreeMB, dir)
	}
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the probe left %d file(s) behind: %v", len(left), left)
	}
}

func TestProbeZeroOnlyChecksTheDirectory(t *testing.T) {
	dir := t.TempDir()
	r, err := Probe(dir, 0)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if r.WrittenMB != 0 || r.FreeMB != -1 {
		t.Errorf("report %+v, want nothing written and no statfs", r)
	}
	if _, err := Probe(filepath.Join(dir, "missing"), 0); err == nil {
		t.Error("a missing directory passed")
	}
}

func TestProbeRefusesARegularFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Probe(file, 1)
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if !strings.Contains(err.Error(), "not a directory") || !strings.Contains(err.Error(), "set TMPDIR") {
		t.Errorf("message %q does not name the cause and the remedy", err)
	}
}

func TestProbeRefusesMoreThanFreeWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	r, err := Probe(dir, 1)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if r.FreeMB < 0 {
		t.Skip("free space unknown on this platform")
	}
	need := int(r.FreeMB) + 1024
	r, err = Probe(dir, need)
	if err == nil {
		t.Fatalf("asking for %d MB in a directory with %d MB free passed", need, r.FreeMB)
	}
	if r.WrittenMB != 0 {
		t.Errorf("wrote %d MB although statfs already said no", r.WrittenMB)
	}
	msg := err.Error()
	for _, want := range []string{dir, "not enough free space", "set TMPDIR", "MB"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("files left behind: %v", left)
	}
}

func TestProbeNamesAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	_, err := Probe(dir, 1)
	if err == nil {
		t.Fatal("an unwritable directory passed")
	}
	if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("message %q does not say not writable", err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("err = %v, want to unwrap to a permission error", err)
	}
}

// The quota and the full disk cannot be produced in a unit test, so the
// words the probe prints for them are checked on synthetic errors.
func TestDescribeNamesTheCause(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&fs.PathError{Op: "write", Path: "/x", Err: syscall.EDQUOT}, "disk quota exceeded"},
		{&fs.PathError{Op: "write", Path: "/x", Err: syscall.ENOSPC}, "no space left on the device"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}, "not writable"},
		{&fs.PathError{Op: "stat", Path: "/x", Err: syscall.ENOENT}, "does not exist"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOTDIR}, "not a directory"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.EROFS}, "read-only file system"},
		{errors.New("something else"), "something else"},
	}
	for _, c := range cases {
		if got := describe(c.err); got != c.want {
			t.Errorf("describe(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestErrorPrintsTheNumbersAndTheRemedy(t *testing.T) {
	e := &Error{Report: Report{Dir: "/scratch", FreeMB: 10, NeedMB: 256, WrittenMB: 3}, Cause: syscall.EDQUOT, reason: "disk quota exceeded"}
	want := "tmpspace: /scratch: 10 MB free, wrote 3 MB of 256 MB: disk quota exceeded — set TMPDIR to a directory on disk with at least 256 MB"
	if got := e.Error(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if !errors.Is(e, syscall.EDQUOT) {
		t.Error("the cause does not unwrap")
	}
	e.Report.FreeMB = -1
	if got := e.Error(); !strings.Contains(got, "free space unknown") {
		t.Errorf("unknown free space is printed as %q", got)
	}
}

func TestProbeEnvZeroChecksNothing(t *testing.T) {
	t.Setenv(EnvVar, "0")
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := ProbeEnv(256); err != nil {
		t.Errorf("MCD_TMP_MIN_MB=0 still checked the directory: %v", err)
	}
	t.Setenv(EnvVar, "x")
	if _, err := ProbeEnv(256); err == nil {
		t.Error("a bad value passed")
	}
}

func TestEnvMB(t *testing.T) {
	cases := []struct {
		value   string
		set     bool
		want    int
		wantErr bool
	}{
		{set: false, want: 256},
		{set: true, value: "", want: 256},
		{set: true, value: "0", want: 0},
		{set: true, value: "512", want: 512},
		{set: true, value: "abc", wantErr: true},
		{set: true, value: "-5", wantErr: true},
		{set: true, value: "1.5", wantErr: true},
	}
	for _, c := range cases {
		const name = "TMPSPACE_TEST_MB"
		if c.set {
			t.Setenv(name, c.value)
		} else {
			os.Unsetenv(name)
		}
		got, err := EnvMB(name, 256)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", c.value, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("%q: got %d, want %d", c.value, got, c.want)
		}
	}
}
