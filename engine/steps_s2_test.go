package modelcheck_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"modelcheck/internal/tmpspace"
)

// s2World is the per-scenario state for the harness scenarios of
// features/s2-resources.feature. They start this very test binary a second
// time with -test.run=^$ (no test selected), so that only TestMain runs, and
// look at its exit code and its standard error: the probe of the scratch
// directory is observed the way a developer sees it, not through Go values.
type s2World struct {
	tmp    string // the TMPDIR handed to the child, a fresh directory
	exit   int
	stdout string
	stderr string
}

// aboveAnyFreeSpace is a request no file system can satisfy (one pebibyte),
// so the probe fails on the free-space check and writes nothing.
const aboveAnyFreeSpace = 1 << 30

func init() {
	stepRegistrars = append(stepRegistrars, registerS2Steps)
}

func registerS2Steps(sc *godog.ScenarioContext) {
	w := &s2World{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			os.RemoveAll(w.tmp)
		}
		*w = s2World{}
		return ctx, nil
	})

	start := func(minMB string) error {
		dir, err := os.MkdirTemp("", "s2-tmpdir-")
		if err != nil {
			return fmt.Errorf("create a scratch directory for the child: %w", err)
		}
		w.tmp = dir
		// The request above any free space is refused on the statfs check,
		// before a byte is written — but only where statfs is known. Where it
		// is not (Windows, Solaris, NetBSD, OpenBSD: see free_other.go) the
		// child would write the whole request, so the scenario refuses to run
		// there, loudly.
		if r, err := tmpspace.Probe(dir, 1); err != nil {
			return fmt.Errorf("the scratch directory of the scenario itself fails the probe: %w", err)
		} else if r.FreeMB < 0 {
			return fmt.Errorf("free space is unknown on %s: the scenario would write the whole request and is not covered here", runtime.GOOS)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "MCD_TMP_MIN_MB="+minMB, "TMPDIR="+dir)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err = cmd.Run()
		w.stdout, w.stderr = out.String(), errb.String()
		var ee *exec.ExitError
		switch {
		case err == nil:
			w.exit = 0
		case errors.As(err, &ee):
			w.exit = ee.ExitCode()
		default:
			return fmt.Errorf("start %s again: %w", os.Args[0], err)
		}
		return nil
	}
	sc.Step(`^the engine test binary starts with MCD_TMP_MIN_MB above any free space$`, func() error {
		return start(strconv.Itoa(aboveAnyFreeSpace))
	})
	sc.Step(`^the engine test binary starts with MCD_TMP_MIN_MB "([^"]*)"$`, start)

	sc.Step(`^the test binary exits with 2 before any test runs$`, func() error {
		if w.exit != 2 {
			return fmt.Errorf("the test binary exited with %d, want 2; stderr: %s", w.exit, strings.TrimSpace(w.stderr))
		}
		for _, mark := range []string{"=== RUN", "PASS", "FAIL"} {
			if strings.Contains(w.stdout, mark) {
				return fmt.Errorf("the test binary ran tests before refusing: stdout has %q", mark)
			}
		}
		return nil
	})
	sc.Step(`^the test binary exits with 0$`, func() error {
		if w.exit != 0 {
			return fmt.Errorf("the test binary exited with %d, want 0; stderr: %s", w.exit, strings.TrimSpace(w.stderr))
		}
		return nil
	})
	sc.Step(`^its message names the scratch directory, the amount asked and "([^"]+)"$`, func(word string) error {
		for _, want := range []string{w.tmp, strconv.Itoa(aboveAnyFreeSpace) + " MB", word} {
			if !strings.Contains(w.stderr, want) {
				return fmt.Errorf("the message does not name %q: %s", want, strings.TrimSpace(w.stderr))
			}
		}
		return nil
	})
}
