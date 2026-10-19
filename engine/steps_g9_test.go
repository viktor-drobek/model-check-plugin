package modelcheck_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/cucumber/godog"
)

func init() {
	stepRegistrars = append(stepRegistrars, registerG9Steps)
}

type g9World struct {
	fixtures string
	document map[string]any
}

func registerG9Steps(sc *godog.ScenarioContext) {
	w := &g9World{}

	sc.Step(`^the algorithm trace fixtures under "([^"]+)"$`, func(rel string) error {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return err
		}
		w.fixtures = filepath.Join(plugin, rel)
		info, err := os.Stat(w.fixtures)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("algorithm trace fixture path %s is not a directory", w.fixtures)
		}
		return nil
	})

	sc.Step(`^I run the algtrace CLI$`, func() error {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return err
		}
		cmd := exec.Command("python3", filepath.Join(plugin, "tools", "algtrace", "algtrace.py"), w.fixtures)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("algtrace failed: %w: %s", err, out)
		}
		if err := json.Unmarshal(out, &w.document); err != nil {
			return fmt.Errorf("algtrace output is not JSON: %w", err)
		}
		return nil
	})

	sc.Step(`^the algorithm graph schema is "([^"]+)"$`, func(want string) error {
		if got, _ := w.document["schema"].(string); got != want {
			return fmt.Errorf("algorithm graph schema is %q, want %q", got, want)
		}
		return nil
	})

	sc.Step(`^the algorithm graph reports these languages:$`, func(table *godog.Table) error {
		var want []string
		for _, row := range table.Rows {
			want = append(want, row.Cells[0].Value)
		}
		sort.Strings(want)
		var got []string
		for _, value := range w.document["languages"].([]any) {
			got = append(got, value.(string))
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("algorithm graph languages are %v, want %v", got, want)
		}
		return nil
	})

	sc.Step(`^every algorithm graph relation has evidence "([^"]+)"$`, func(want string) error {
		edges, ok := w.document["edges"].([]any)
		if !ok || len(edges) == 0 {
			return fmt.Errorf("algorithm graph has no edges")
		}
		for _, value := range edges {
			edge, ok := value.(map[string]any)
			if !ok || edge["evidence"] != want {
				return fmt.Errorf("algorithm graph edge has evidence %v, want %q", edge["evidence"], want)
			}
		}
		return nil
	})

	sc.Step(`^the algorithm graph contains each relation kind:$`, func(table *godog.Table) error {
		seen := map[string]bool{}
		for _, value := range w.document["edges"].([]any) {
			edge, _ := value.(map[string]any)
			if kind, _ := edge["kind"].(string); kind != "" {
				seen[kind] = true
			}
		}
		for _, row := range table.Rows {
			if want := row.Cells[0].Value; !seen[want] {
				return fmt.Errorf("algorithm graph contains no %q relation; got %v", want, seen)
			}
		}
		return nil
	})
}
