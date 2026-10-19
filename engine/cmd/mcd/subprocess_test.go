package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"modelcheck/ir"
	"modelcheck/report"
)

var (
	mcdBuildOnce sync.Once
	mcdBinary    string
	mcdBuildErr  error
	mcdBuildDir  string
)

const (
	stdioServerMaxMS           = 5000
	stdioResponseTimeout       = 10 * time.Second
	stdioShutdownTimeout       = 10 * time.Second
	stdioForcedShutdownTimeout = time.Second
)

func TestMain(m *testing.M) {
	code := m.Run()
	if mcdBuildDir != "" {
		_ = os.RemoveAll(mcdBuildDir)
	}
	os.Exit(code)
}

func mcdPackageDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func mcdPath(t *testing.T) string {
	t.Helper()
	mcdBuildOnce.Do(func() {
		mcdBuildDir, mcdBuildErr = os.MkdirTemp("", "mcd-subprocess-")
		if mcdBuildErr != nil {
			return
		}
		mcdBinary = filepath.Join(mcdBuildDir, "mcd")
		cmd := exec.Command("go", "build", "-o", mcdBinary, ".")
		cmd.Dir = mcdPackageDir()
		output, err := cmd.CombinedOutput()
		if err != nil {
			mcdBuildErr = fmt.Errorf("go build %s: %w\n%s", cmd.Dir, err, output)
		}
	})
	if mcdBuildErr != nil {
		t.Fatal(mcdBuildErr)
	}
	return mcdBinary
}

func runMCD(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(mcdPath(t), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return string(stdout), stderr.String(), code
}

func TestSubprocessCLI(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		stdout, stderr, code := runMCD(t, "version")
		if code != 0 {
			t.Fatalf("mcd version exit code = %d, stderr = %q", code, stderr)
		}
		if stderr != "" {
			t.Fatalf("mcd version wrote to stderr: %q", stderr)
		}
		want := fmt.Sprintf("%s %s (ir %s, report %s)", report.EngineName, report.EngineVersion, ir.Schema, report.ReportSchema)
		if strings.TrimSpace(stdout) != want {
			t.Fatalf("mcd version = %q, want %q", strings.TrimSpace(stdout), want)
		}
	})

	t.Run("parse", func(t *testing.T) {
		fixture := filepath.Join(mcdPackageDir(), "..", "..", "testdata", "promela", "atomic-t1.pml")
		stdout, stderr, code := runMCD(t, "parse", "--promela", fixture)
		if code != 0 {
			t.Fatalf("mcd parse exit code = %d, stderr = %q", code, stderr)
		}
		if stderr != "" {
			t.Fatalf("mcd parse wrote to stderr: %q", stderr)
		}
		var model ir.Model
		if err := json.Unmarshal([]byte(stdout), &model); err != nil {
			t.Fatalf("mcd parse returned invalid IR JSON: %v\n%s", err, stdout)
		}
		if model.Schema != ir.Schema {
			t.Fatalf("parsed schema = %q, want %q", model.Schema, ir.Schema)
		}
		if model.Name == "" || len(model.Processes) == 0 {
			t.Fatalf("parsed model is incomplete: name=%q processes=%d", model.Name, len(model.Processes))
		}
	})
}

type stdioClient struct {
	cmd             *exec.Cmd
	in              io.WriteCloser
	stdout          io.ReadCloser
	responses       chan stdioResponse
	stderr          synchronizedBuffer
	responseTimeout time.Duration
	stopReading     chan struct{}
	stopReadingOnce sync.Once
	readerDone      chan struct{}
	id              int
}

type stdioResponse struct {
	line string
	err  error
}

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func readStdioResponses(out io.Reader, responses chan<- stdioResponse, stopReading <-chan struct{}, readerDone chan<- struct{}) {
	defer close(readerDone)
	reader := bufio.NewReader(out)
	for {
		line, err := reader.ReadString('\n')
		select {
		case responses <- stdioResponse{line: line, err: err}:
		case <-stopReading:
			return
		}
		if err != nil {
			close(responses)
			return
		}
	}
}

func startStdioClient(t *testing.T, binary, sessionDir string) *stdioClient {
	t.Helper()
	cmd := exec.Command(binary, "serve", "--session-dir", sessionDir, "--max-ms", fmt.Sprint(stdioServerMaxMS))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	responses := make(chan stdioResponse, 1)
	client := &stdioClient{
		cmd:         cmd,
		in:          in,
		stdout:      out,
		responses:   responses,
		stopReading: make(chan struct{}),
		readerDone:  make(chan struct{}),
	}
	cmd.Stderr = &client.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go readStdioResponses(out, responses, client.stopReading, client.readerDone)
	t.Cleanup(func() { client.close(t) })
	if _, err := client.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcd-subprocess-test", "version": "0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.notify("notifications/initialized", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	return client
}

func (c *stdioClient) write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(data, '\n'))
	return err
}

func (c *stdioClient) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *stdioClient) call(method string, params any) (map[string]any, error) {
	c.id++
	id := c.id
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	timeout := c.responseTimeout
	if timeout <= 0 {
		timeout = stdioResponseTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return nil, fmt.Errorf("%s: timed out after %s waiting for MCP response (server stderr: %s)", method, timeout, c.stderr.String())
		case response, ok := <-c.responses:
			if !ok {
				return nil, fmt.Errorf("%s: server output closed (server stderr: %s)", method, c.stderr.String())
			}
			if response.err != nil {
				return nil, fmt.Errorf("%s: %w (server stderr: %s)", method, response.err, c.stderr.String())
			}
			line := response.line
			var message map[string]any
			if err := json.Unmarshal([]byte(line), &message); err != nil {
				continue
			}
			got, ok := message["id"].(float64)
			if !ok || int(got) != id {
				continue
			}
			if rpcErr, ok := message["error"]; ok {
				return nil, fmt.Errorf("%s: %v", method, rpcErr)
			}
			result, _ := message["result"].(map[string]any)
			return result, nil
		}
	}
}

func (c *stdioClient) close(t *testing.T) {
	t.Helper()
	if c.in == nil {
		return
	}
	c.stopReadingOnce.Do(func() { close(c.stopReading) })
	_ = c.in.Close()
	readerStopped := waitForSignal(c.readerDone, stdioShutdownTimeout)
	if !readerStopped {
		_ = c.cmd.Process.Kill()
		_ = c.stdout.Close()
		readerStopped = waitForSignal(c.readerDone, stdioForcedShutdownTimeout)
		if !readerStopped {
			t.Errorf("mcd serve response reader did not exit after forced shutdown (stderr: %s)", c.stderr.String())
		}
	}

	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	waitErr, processStopped := waitForProcess(done, stdioShutdownTimeout)
	if !processStopped {
		_ = c.cmd.Process.Kill()
		_ = c.stdout.Close()
		waitErr, processStopped = waitForProcess(done, stdioForcedShutdownTimeout)
	}
	if !readerStopped || !processStopped || waitErr != nil {
		t.Errorf("mcd serve cleanup failed (reader stopped: %t; process stopped: %t; wait: %v; stderr: %s)", readerStopped, processStopped, waitErr, c.stderr.String())
	}
	c.in = nil
}

func waitForSignal(signal <-chan struct{}, timeout time.Duration) bool {
	if signal == nil {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-signal:
		return true
	case <-timer.C:
		return false
	}
}

func waitForProcess(done <-chan error, timeout time.Duration) (error, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err, true
	case <-timer.C:
		return nil, false
	}
}

type writeOnlyCloser struct{}

func (writeOnlyCloser) Write(p []byte) (int, error) { return len(p), nil }
func (writeOnlyCloser) Close() error                { return nil }

func TestStdioClientCallHasResponseTimeout(t *testing.T) {
	client := &stdioClient{
		in:              writeOnlyCloser{},
		responses:       make(chan stdioResponse),
		responseTimeout: 10 * time.Millisecond,
	}
	started := time.Now()
	_, err := client.call("initialize", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("call error = %v, want a response timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s, want it bounded near %s", elapsed, client.responseTimeout)
	}
}

func TestStdioResponseReaderStopsWhenResponseChannelIsFull(t *testing.T) {
	reader, writer := io.Pipe()
	responses := make(chan stdioResponse, 1)
	responses <- stdioResponse{line: "already buffered\n"}
	stopReading := make(chan struct{})
	readerDone := make(chan struct{})
	go readStdioResponses(reader, responses, stopReading, readerDone)

	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	close(stopReading)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("response reader did not stop after cancellation")
	}
}

func TestStdioResponseReaderClosesChannelOnEOF(t *testing.T) {
	reader, writer := io.Pipe()
	responses := make(chan stdioResponse, 1)
	stopReading := make(chan struct{})
	readerDone := make(chan struct{})
	go readStdioResponses(reader, responses, stopReading, readerDone)

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("response reader did not finish after EOF")
	}
	response, ok := <-responses
	if !ok || response.err != io.EOF {
		t.Fatalf("EOF response = %#v, open = %v; want an EOF response", response, ok)
	}
	if _, ok := <-responses; ok {
		t.Fatal("response channel remained open after EOF")
	}
}

func TestSynchronizedBufferSupportsConcurrentAccess(t *testing.T) {
	var buffer synchronizedBuffer
	const writers = 8
	const writesPerWriter = 100
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				_, _ = buffer.Write([]byte("x"))
				_ = buffer.String()
			}
		}()
	}
	wg.Wait()
	if got, want := len(buffer.String()), writers*writesPerWriter; got != want {
		t.Fatalf("buffer length = %d, want %d", got, want)
	}
}

func TestSubprocessServeRoundTrip(t *testing.T) {
	client := startStdioClient(t, mcdPath(t), t.TempDir())

	toolsResult, err := client.call("tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	toolsValue, ok := toolsResult["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list returned no tools array: %#v", toolsResult)
	}
	var tools []string
	for _, raw := range toolsValue {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tools/list returned non-object tool %T", raw)
		}
		name, ok := tool["name"].(string)
		if !ok {
			t.Fatalf("tools/list returned tool without name: %#v", tool)
		}
		tools = append(tools, name)
	}
	sort.Strings(tools)
	wantTools := []string{"mc_check", "mc_estimate", "mc_explain", "mc_lint_property", "mc_manifest", "mc_parse", "mc_simulate"}
	if strings.Join(tools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf("tools/list = %v, want %v", tools, wantTools)
	}

	parseResult, err := client.call("tools/call", map[string]any{
		"name": "mc_parse",
		"arguments": map[string]any{
			"promela": "byte x; active proctype A(){ x = 1 }",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if isError, _ := parseResult["isError"].(bool); isError {
		t.Fatalf("mc_parse returned a tool error: %#v", parseResult)
	}
	structured, ok := parseResult["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("mc_parse returned no structured content: %#v", parseResult)
	}
	if outcome, _ := structured["outcome"].(string); outcome != "ir" {
		t.Fatalf("mc_parse outcome = %q, want ir: %#v", outcome, structured)
	}
}
