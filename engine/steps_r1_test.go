package modelcheck_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// Step definitions for features/r1-review-fixes.feature.
//
// The pass this feature accepts is a documentation repair: the engine moved on
// in G5/G6 and the skill's prose did not, so the instructions promised
// capabilities the engine lacks and forbade ones it has (steps/review-astra-skill.md,
// steps/review-sol-reconciliation.md). What is checkable about such a repair is
// exactly which sentences are present and which are gone, so the two steps here
// are verbatim substring checks over a file of the plugin. A phrase that was
// wrong must not come back by a copy-paste later; a phrase that carries the
// correction must stay until someone changes it on purpose.
func init() {
	stepRegistrars = append(stepRegistrars, registerR1Steps)
}

func registerR1Steps(sc *godog.ScenarioContext) {
	read := func(rel string) (string, error) {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(filepath.Join(plugin, rel))
		if err != nil {
			return "", fmt.Errorf("%s: %w", rel, err)
		}
		// The prose wraps, so a phrase may be split by a newline and its
		// indentation; compare on a single-spaced copy.
		return strings.Join(strings.Fields(string(b)), " "), nil
	}

	sc.Step(`^"([^"]+)" contains:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var missing []string
		for _, row := range t.Rows {
			phrase := strings.Join(strings.Fields(row.Cells[0].Value), " ")
			if !strings.Contains(s, phrase) {
				missing = append(missing, phrase)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s does not contain: %s", rel, strings.Join(missing, " | "))
		}
		return nil
	})

	sc.Step(`^"([^"]+)" no longer contains:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var present []string
		for _, row := range t.Rows {
			phrase := strings.Join(strings.Fields(row.Cells[0].Value), " ")
			if strings.Contains(s, phrase) {
				present = append(present, phrase)
			}
		}
		if len(present) > 0 {
			return fmt.Errorf("%s still contains: %s", rel, strings.Join(present, " | "))
		}
		return nil
	})
}
