// Command conformance checks frozen JSONL cases against a rex-compatible binary.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type record struct {
	Meta     map[string]any  `json:"meta,omitempty"`
	Request  json.RawMessage `json:"request,omitempty"`
	Expected json.RawMessage `json:"expected,omitempty"`
}

func main() {
	logs := flag.String("logs", "testdata/re2logs", "JSONL directory")
	bin := flag.String("bin", "go run ./cmd/rex", "engine command")
	flag.Parse()
	files, err := filepath.Glob(filepath.Join(*logs, "*.jsonl"))
	if err != nil {
		panic(err)
	}
	fail := 0
	for _, f := range files {
		h, err := os.Open(f)
		if err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(h)
		n := 0
		for sc.Scan() {
			n++
			var r record
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				fail++
				fmt.Printf("%s:%d: bad log: %v\n", f, n, err)
				continue
			}
			if r.Meta != nil || r.Request == nil {
				continue
			}
			cmd := exec.Command("sh", "-c", *bin)
			cmd.Stdin = bytes.NewReader(append(r.Request, '\n'))
			got, err := cmd.Output()
			if err != nil {
				fail++
				fmt.Printf("%s:%d: %v\n", f, n, err)
				continue
			}
			var gm, wm any
			if json.Unmarshal(got, &gm) != nil || json.Unmarshal(r.Expected, &wm) != nil || !equal(gm, wm) {
				fail++
				fmt.Printf("%s:%d mismatch\n got: %s want: %s\n", f, n, got, r.Expected)
			}
		}
		_ = h.Close()
	}
	if fail > 0 {
		fmt.Fprintf(os.Stderr, "%d conformance failures\n", fail)
		os.Exit(1)
	}
	fmt.Printf("conformance: %d log files passed\n", len(files))
}
func equal(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}
