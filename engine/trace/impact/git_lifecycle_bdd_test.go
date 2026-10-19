package impact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type gitLifecycleWorld struct {
	t              *testing.T
	mode           string
	err            error
	started        time.Time
	pidFile        string
	child          int
	result         chan error
	helperDir      string
	previousPath   string
	previousHelper string
	previousPID    string
	hadPath        bool
	hadHelper      bool
	hadPID         bool
}

func TestG9ImpactGitLifecycleFeatures(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			world := &gitLifecycleWorld{t: t}
			world.captureEnvironment()
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				world.cleanup()
				return ctx, nil
			})
			sc.Step(`^a lifecycle Git helper that closes stdout and floods stderr$`, world.stderrOverflowHelper)
			sc.Step(`^a lifecycle Git helper whose child inherits output pipes$`, world.inheritedPipeHelper)
			sc.Step(`^a lifecycle Git helper whose leader exits after closing stdout$`, world.leaderExitHelper)
			sc.Step(`^the bounded lifecycle Git command is run$`, world.run)
			sc.Step(`^the bounded lifecycle Git command is cancelled$`, world.cancelledRun)
			sc.Step(`^it returns a stderr output-limit error promptly$`, world.stderrOverflowError)
			sc.Step(`^the self-expiring child closes the inherited pipes before the context expires$`, world.inheritedPipeResult)
			sc.Step(`^the surviving descendant terminates promptly$`, world.descendantResult)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/g9-impact-git-lifecycle.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run Git lifecycle features")
	}
}

func (w *gitLifecycleWorld) stderrOverflowHelper() error {
	w.mode = "stderr"
	return w.installHelper(w.mode + "-after-stdout")
}

func (w *gitLifecycleWorld) inheritedPipeHelper() error {
	w.mode = "inherited"
	return w.installHelper("inherited-pipe-parent")
}

func (w *gitLifecycleWorld) leaderExitHelper() error {
	return w.installHelper("stdout-closed-exiting-leader")
}

func (w *gitLifecycleWorld) run() error {
	w.started = time.Now()
	if w.mode == "stderr" {
		_, _, w.err = runGit(context.Background(), w.t.TempDir(), 32, "status")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, w.err = runGit(ctx, w.t.TempDir(), 4096, "status")
	return nil
}

func (w *gitLifecycleWorld) cancelledRun() error {
	tempDir := w.t.TempDir()
	w.pidFile = filepath.Join(tempDir, "child.pid")
	if err := os.Setenv("MCD_GIT_LIFECYCLE_PID", w.pidFile); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.result = make(chan error, 1)
	go func() {
		_, _, err := runGit(ctx, tempDir, 4096, "status")
		w.result <- err
	}()
	child, err := waitForGitHelperPID(w.pidFile)
	if err != nil {
		cancel()
		return err
	}
	w.child = child
	cancel()
	err = <-w.result
	if !errors.Is(err, context.Canceled) {
		return fmt.Errorf("cancelled lifecycle Git helper error = %v, want context canceled", err)
	}
	return nil
}

func (w *gitLifecycleWorld) stderrOverflowError() error {
	if w.err == nil || !strings.Contains(w.err.Error(), "stderr exceeded") {
		return errors.New("lifecycle Git helper did not report stderr overflow")
	}
	if elapsed := time.Since(w.started); elapsed >= time.Second {
		return errors.New("lifecycle Git helper did not stop promptly")
	}
	return nil
}

func (w *gitLifecycleWorld) inheritedPipeResult() error {
	if w.err != nil {
		return w.err
	}
	return nil
}

func (w *gitLifecycleWorld) descendantResult() error {
	if w.child == 0 {
		return errors.New("cancelled lifecycle helper did not record a child PID")
	}
	if !waitForGitHelperExit(w.child) {
		return fmt.Errorf("descendant %d survived cancellation", w.child)
	}
	return nil
}

func (w *gitLifecycleWorld) captureEnvironment() {
	w.previousPath, w.hadPath = os.LookupEnv("PATH")
	w.previousHelper, w.hadHelper = os.LookupEnv("MCD_GIT_LIFECYCLE_HELPER")
	w.previousPID, w.hadPID = os.LookupEnv("MCD_GIT_LIFECYCLE_PID")
}

func (w *gitLifecycleWorld) installHelper(mode string) error {
	dir, err := os.MkdirTemp("", "git-lifecycle-bdd-")
	if err != nil {
		return err
	}
	w.helperDir = dir
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o700); err != nil {
		return err
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		return err
	}
	return os.Setenv("MCD_GIT_LIFECYCLE_HELPER", mode)
}

func (w *gitLifecycleWorld) cleanup() {
	if w.hadPath {
		_ = os.Setenv("PATH", w.previousPath)
	} else {
		_ = os.Unsetenv("PATH")
	}
	if w.hadHelper {
		_ = os.Setenv("MCD_GIT_LIFECYCLE_HELPER", w.previousHelper)
	} else {
		_ = os.Unsetenv("MCD_GIT_LIFECYCLE_HELPER")
	}
	if w.hadPID {
		_ = os.Setenv("MCD_GIT_LIFECYCLE_PID", w.previousPID)
	} else {
		_ = os.Unsetenv("MCD_GIT_LIFECYCLE_PID")
	}
	if w.helperDir != "" {
		_ = os.RemoveAll(w.helperDir)
	}
}
