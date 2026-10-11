package testgen

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// childEnv names the case a child test process runs.
const childEnv = "MCD_LIMIT_CASE"

// Case is one crash-scale input: Run returns the message the engine answers
// (an error text, or "ok" when the input is accepted); Want is a substring
// the message must contain.
type Case struct {
	Name string
	Want string
	Run  func() string
}

// RunInChild runs every case in a child process: the test binary itself,
// started with -test.run=^<test>$ and the case's name in the environment,
// its address space capped at memLimitMB (where the platform allows it) and
// its time at timeout. A case that still overflows the stack or the memory
// kills the child, not the suite, and the failure names the case. The
// child side is the same function: when the environment names a case it
// runs it, prints the message and exits.
func RunInChild(t *testing.T, test string, memLimitMB int, timeout time.Duration, cases []Case) {
	t.Helper()
	if name := os.Getenv(childEnv); name != "" {
		for _, c := range cases {
			if c.Name == name {
				if !raceEnabled {
					limitAddressSpace(memLimitMB)
				}
				os.Stdout.WriteString(c.Run())
				return
			}
		}
		t.Fatalf("no case %q", name)
	}
	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+test+"$", "-test.count=1")
			cmd.Env = append(os.Environ(), childEnv+"="+c.Name)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			err := cmd.Run()
			got := strings.TrimSpace(out.String())
			if err != nil {
				t.Fatalf("the child did not answer (%v): the input was not caught by a limit\nstdout: %s\nstderr: %s", err, got, tail(errb.String(), 600))
			}
			if !strings.Contains(got, c.Want) {
				t.Fatalf("answer %q does not contain %q", got, c.Want)
			}
		})
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
