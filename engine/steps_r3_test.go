package modelcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// Step definitions for features/r3-agent-tasks.feature.
//
// The set under test (steps/agent-tasks/tasks.json) exists because the live runs
// so far produced two outcomes of six. Two things are checked here and they are
// different in kind: that the expected outcome of a task is what this binary
// really answers (re-run, not trusted), and that a recorded agent run carried
// that outcome into its answer without claiming more than it allows.
func init() {
	stepRegistrars = append(stepRegistrars, registerR3Steps)
}

type r3Task struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Question string   `json:"question"`
	Model    string   `json:"model"`
	Why      string   `json:"why"`
	Covers   []string `json:"covers"`
	Expected struct {
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
	} `json:"expected"`
	GroundTruth []struct {
		Args          []string `json:"args"`
		Property      string   `json:"property"`
		RejectionKind string   `json:"rejection_kind"`
		Status        string   `json:"status"`
		Evidence      string   `json:"evidence"`
	} `json:"ground_truth"`
}

type r3Set struct {
	Note              string              `json:"note"`
	Tasks             []r3Task            `json:"tasks"`
	ForbiddenByStatus map[string][]string `json:"forbidden_by_status"`
	// Deviations are asserted, not excused: a record listed here has to show
	// exactly the deviation named, and every record not listed has to pass
	// every check. A run that improves turns this red too, on purpose.
	Deviations []struct {
		Executor string `json:"executor"`
		Task     string `json:"task"`
		Kind     string `json:"kind"`
		Note     string `json:"note"`
	} `json:"deviations"`
}

type r3World struct {
	plugin    string
	set       r3Set
	run       string // directory of the recorded run, when a scenario names one
	task      *r3Task
	deviation string // the kind recorded for this record, empty when none
}

// deviationOf: the kind recorded for (executor, task), or "".
func deviationOf(set *r3Set, executor, task string) string {
	for _, d := range set.Deviations {
		if d.Executor == executor && d.Task == task {
			return d.Kind
		}
	}
	return ""
}

func registerR3Steps(sc *godog.ScenarioContext) {
	w := &r3World{}

	task := func(id string) (*r3Task, error) {
		for i := range w.set.Tasks {
			if w.set.Tasks[i].ID == id {
				return &w.set.Tasks[i], nil
			}
		}
		return nil, fmt.Errorf("the set has no task %q", id)
	}

	// The engine as the plugin ships it, not a rebuild: the expected outcomes
	// were fixed with this binary and the acceptance re-runs the same one.
	runEngine := func(t *r3Task, args []string) (map[string]any, error) {
		dir, err := os.MkdirTemp("", "r3-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		model := filepath.Join(dir, "model.pml")
		if err := os.WriteFile(model, []byte(t.Model), 0o644); err != nil {
			return nil, err
		}
		full := append([]string{"check", "--promela", model}, args...)
		cmd := exec.Command(filepath.Join(w.plugin, "engine", "bin", "mcd"), full...)
		out, err := cmd.Output()
		var doc map[string]any
		if jsonErr := json.Unmarshal(out, &doc); jsonErr != nil {
			return nil, fmt.Errorf("mcd %v: output is not JSON: %v", full, jsonErr)
		}
		// A non-zero exit is only allowed when the engine itself explains it: exit
		// 2 with a rejection object. Otherwise the binary crashed after printing
		// something parseable, and a check that ignores that is green on a broken
		// engine (implementation review, finding 4).
		if err != nil {
			if e, _ := doc["error"].(map[string]any); e == nil {
				return nil, fmt.Errorf("mcd %v exited non-zero without a rejection: %v", full, err)
			}
		}
		return doc, nil
	}

	answerText := func() (string, error) {
		b, err := os.ReadFile(filepath.Join(w.run, "answer.md"))
		return string(b), err
	}

	// Every JSON under a directory that is a report this engine wrote.
	reportsUnder := func(dir string) []map[string]any {
		var out []map[string]any
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".json") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			var doc map[string]any
			if json.Unmarshal(b, &doc) != nil {
				return nil
			}
			if isEngineReport(doc) {
				out = append(out, doc)
			}
			return nil
		})
		return out
	}

	sc.Step(`^the agent task set "([^"]+)"$`, func(rel string) error {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return err
		}
		w.plugin = plugin
		b, err := os.ReadFile(filepath.Join(plugin, rel))
		if err != nil {
			return err
		}
		return json.Unmarshal(b, &w.set)
	})

	sc.Step(`^the set covers each of:$`, func(t *godog.Table) error {
		seen := map[string]bool{}
		for _, task := range w.set.Tasks {
			for _, c := range task.Covers {
				seen[c] = true
			}
			seen[task.Expected.Status] = true
			seen[task.Expected.Evidence] = true
		}
		var missing []string
		for _, row := range t.Rows {
			want := strings.TrimSpace(row.Cells[0].Value)
			if want == "unknown-evidence" {
				// spelt apart from the status of the same name on purpose
				if seen["unknown-evidence"] || seen["unknown"] {
					continue
				}
			}
			if !seen[want] {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("the set covers no task for: %s", strings.Join(missing, ", "))
		}
		return nil
	})

	sc.Step(`^no task expects the status "([^"]+)", which the engine does not emit$`, func(status string) error {
		for _, t := range w.set.Tasks {
			if t.Expected.Status == status {
				return fmt.Errorf("task %s expects %q", t.ID, status)
			}
		}
		return nil
	})

	sc.Step(`^every task names the model it gives and the reason its outcome is the honest one$`, func() error {
		for _, t := range w.set.Tasks {
			switch {
			case strings.TrimSpace(t.Model) == "":
				return fmt.Errorf("task %s gives no model", t.ID)
			case strings.TrimSpace(t.Question) == "":
				return fmt.Errorf("task %s asks nothing", t.ID)
			case strings.TrimSpace(t.Why) == "":
				return fmt.Errorf("task %s does not say why its outcome is the honest one", t.ID)
			case len(t.GroundTruth) == 0:
				return fmt.Errorf("task %s records no invocation for its expected outcome", t.ID)
			}
		}
		return nil
	})

	sc.Step(`^running the engine on task "([^"]+)" gives the status and evidence the task expects$`, func(id string) error {
		t, err := task(id)
		if err != nil {
			return err
		}
		for _, g := range t.GroundTruth {
			doc, err := runEngine(t, g.Args)
			if err != nil {
				return err
			}
			if g.RejectionKind != "" {
				e, _ := doc["error"].(map[string]any)
				if e == nil {
					return fmt.Errorf("%s %v: expected a rejection, got a report", id, g.Args)
				}
				if fmt.Sprint(e["kind"]) != g.RejectionKind || fmt.Sprint(e["status"]) != g.Status {
					return fmt.Errorf("%s %v: rejection is %v/%v, want %s/%s", id, g.Args, e["kind"], e["status"], g.RejectionKind, g.Status)
				}
				continue
			}
			props, _ := doc["properties"].([]any)
			var found map[string]any
			for _, p := range props {
				m, _ := p.(map[string]any)
				if m != nil && m["id"] == g.Property {
					found = m
				}
			}
			if found == nil {
				return fmt.Errorf("%s %v: the report carries no property %q", id, g.Args, g.Property)
			}
			if fmt.Sprint(found["status"]) != g.Status || fmt.Sprint(found["evidence"]) != g.Evidence {
				return fmt.Errorf("%s %v: %s is %v/%v, want %s/%s", id, g.Args, g.Property,
					found["status"], found["evidence"], g.Status, g.Evidence)
			}
			if g.Status != t.Expected.Status && t.Expected.Status != "" && len(t.GroundTruth) == 1 {
				return fmt.Errorf("%s: the recorded invocation gives %s, the task expects %s", id, g.Status, t.Expected.Status)
			}
		}
		return nil
	})

	sc.Step(`^the recorded agent run "([^"]+)"$`, func(rel string) error {
		w.run = filepath.Join(w.plugin, rel)
		st, err := os.Stat(w.run)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("no recorded run at %s", rel)
		}
		id := filepath.Base(w.run)
		t, err := task(id)
		if err != nil {
			return err
		}
		w.task = t
		w.deviation = deviationOf(&w.set, filepath.Base(filepath.Dir(w.run)), id)
		return nil
	})

	sc.Step(`^the run holds a report the engine wrote, or a rejection it returned$`, func() error {
		// Of this task's model: a report of something else says nothing about
		// this run (implementation review, finding 1).
		for _, doc := range reportsUnder(w.run) {
			if reportIsOf(doc, w.task.Model) || reportIsOfRunFile(doc, w.run) {
				return nil
			}
		}
		if len(reportsUnder(w.run)) > 0 && w.task.Expected.Status != "not-executed" {
			return fmt.Errorf("%s holds a report of the engine, but of neither the task's model "+
				"nor a file of this run", w.run)
		}
		// A rejection is not a report: it is the engine's error object, and for a
		// task whose honest outcome is `not-executed` it is the right artefact.
		var found bool
		_ = filepath.Walk(w.run, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".json") {
				return nil
			}
			b, _ := os.ReadFile(p)
			var doc map[string]any
			if json.Unmarshal(b, &doc) == nil {
				if e, _ := doc["error"].(map[string]any); e != nil && e["status"] != nil {
					found = true
				}
			}
			return nil
		})
		if !found && w.task.Expected.Status == "not-executed" {
			// The engine answers a rejected input with an error object, not a file:
			// over MCP it never reaches the disk. The answer naming the rejection
			// and the construct is then the only artefact there is.
			text, err := answerText()
			if err == nil && strings.Contains(strings.ToLower(text), "not-executed") {
				return nil
			}
		}
		if !found {
			if w.deviation == "no-answer" {
				if _, err := os.Stat(filepath.Join(w.run, "answer.md")); err == nil {
					return fmt.Errorf("%s is recorded as %q, but it has an answer now: "+
						"the record changed and the deviation has to be re-judged", w.run, w.deviation)
				}
				return nil
			}
			return fmt.Errorf("%s holds neither a report nor a rejection of the engine", w.run)
		}
		return nil
	})

	sc.Step(`^the run's answer states the status the task expects$`, func() error {
		text, err := answerText()
		if err != nil {
			if w.deviation == "no-answer" {
				return nil
			}
			return err
		}
		if !strings.Contains(text, w.task.Expected.Status) {
			if w.deviation == "status-missing" {
				return nil // asserted below to be exactly this and nothing worse
			}
			return fmt.Errorf("%s/answer.md does not state %q", w.task.ID, w.task.Expected.Status)
		}
		if w.deviation == "status-missing" {
			return fmt.Errorf("%s/answer.md states %q after all: the record changed and the "+
				"recorded deviation no longer holds", w.task.ID, w.task.Expected.Status)
		}
		return nil
	})

	claimsNoMore := func() error {
		text, err := answerText()
		if err != nil {
			return err
		}
		lower := strings.ToLower(text)
		for _, bad := range w.set.ForbiddenByStatus[w.task.Expected.Status] {
			word := strings.ToLower(bad)
			// The denial has to stand immediately before the word. Scanning the
			// whole line lets "Доказано: ошибок нет" pass on its trailing "нет"
			// and lets "не достигается" excuse itself (implementation review,
			// finding 2).
			for at := 0; ; {
				i := strings.Index(lower[at:], word)
				if i < 0 {
					break
				}
				i += at
				at = i + len(word)
				from := i - 24
				if from < 0 {
					from = 0
				}
				before := lower[from:i]
				denied := false
				for _, neg := range []string{"не ", "нельзя", "not ", "никогда не", "без "} {
					if strings.Contains(before, neg) {
						denied = true
					}
				}
				if !denied {
					line := lower[from:]
					if k := strings.IndexByte(line, '\n'); k >= 0 {
						line = line[:k]
					}
					return fmt.Errorf("%s/answer.md claims %q, which %s does not allow: …%s",
						w.task.ID, bad, w.task.Expected.Status, strings.TrimSpace(line))
				}
			}
		}
		return nil
	}

	// `not-executed` has its own rule: the skill allows rewriting the model into
	// the subset, and then a verdict about the rewritten model is legitimate —
	// provided the answer says the original did not run and that the model was
	// changed. A word list cannot express that (tasks.json, "rules").
	sc.Step(`^the run's answer does not claim more than that status allows$`, func() error {
		if w.task.Expected.Status == "not-executed" {
			text, err := answerText()
			if err != nil {
				if w.deviation == "no-answer" {
					return nil
				}
				return err
			}
			lower := strings.ToLower(text)
			verdict := ""
			for _, v := range []string{"violated", "verified"} {
				if strings.Contains(lower, v) {
					verdict = v
				}
			}
			if verdict == "" {
				return nil
			}
			for _, declared := range []string{"переписа", "изменени", "изменил", "rewrit", "заменил",
				"другой модел", "другая модел", "эквивалент", "вместо", "другой запис", "другую запис"} {
				if strings.Contains(lower, declared) {
					return nil
				}
			}
			return fmt.Errorf("%s/answer.md reports %q without declaring that the model was changed",
				w.task.ID, verdict)
		}
		return claimsNoMore()
	})

	sc.Step(`^the planted run "([^"]+)" fails the check that the report belongs to task "([^"]+)"$`, func(rel, id string) error {
		t, err := task(id)
		if err != nil {
			return err
		}
		dir := filepath.Join(w.plugin, rel)
		reports := reportsUnder(dir)
		if len(reports) == 0 {
			return fmt.Errorf("%s holds no report at all: the fixture must look convincing, "+
				"otherwise this scenario passes on an empty directory", rel)
		}
		answer, err := os.ReadFile(filepath.Join(dir, "answer.md"))
		if err != nil {
			return fmt.Errorf("%s has no answer.md: %v", rel, err)
		}
		if !strings.Contains(string(answer), t.Expected.Status) {
			return fmt.Errorf("%s does not even state %q: it would fail for the wrong reason",
				rel, t.Expected.Status)
		}
		sum := sha256.Sum256([]byte(t.Model))
		want := hex.EncodeToString(sum[:])
		for _, doc := range reports {
			inputs, _ := doc["inputs"].([]any)
			for _, i := range inputs {
				m, _ := i.(map[string]any)
				if m != nil && fmt.Sprint(m["sha256"]) == want {
					return fmt.Errorf("the planted run holds a report of task %s after all — the fixture is wrong, not the check", id)
				}
			}
		}
		return nil
	})

	sc.Step(`^every recorded answer of task "([^"]+)" states "([^"]+)" and "([^"]+)"$`, func(id, a, b string) error {
		return eachRecordedAnswer(w, id, func(exec, text string) error {
			lower := strings.ToLower(text)
			for _, tok := range []string{a, b} {
				if !strings.Contains(lower, strings.ToLower(tok)) {
					return fmt.Errorf("%s/%s: the answer does not state %q", exec, id, tok)
				}
			}
			return nil
		})
	})

	sc.Step(`^no recorded answer of task "([^"]+)" calls the rejection "([^"]+)"$`, func(id, word string) error {
		return eachRecordedAnswer(w, id, func(exec, text string) error {
			if strings.Contains(strings.ToLower(text), strings.ToLower(word)) {
				return fmt.Errorf("%s/%s: the rejection is called %q", exec, id, word)
			}
			return nil
		})
	})

	sc.Step(`^the run holds a report with fairness "([^"]+)" and a report with fairness "([^"]+)"$`, func(a, b string) error {
		want := map[string]bool{a: false, b: false}
		for _, doc := range reportsUnder(w.run) {
			props, _ := doc["properties"].([]any)
			for _, p := range props {
				m, _ := p.(map[string]any)
				if m == nil {
					continue
				}
				if tm, _ := m["temporal"].(map[string]any); tm != nil {
					if _, ok := want[fmt.Sprint(tm["fairness"])]; ok {
						want[fmt.Sprint(tm["fairness"])] = true
					}
				}
			}
		}
		var missing []string
		for k, seen := range want {
			if !seen {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			if w.deviation == "second-run-missing" {
				return nil
			}
			return fmt.Errorf("%s: no report ran under fairness %s", w.run, strings.Join(missing, ", "))
		}
		if w.deviation == "second-run-missing" {
			return fmt.Errorf("%s: both runs are there after all, the recorded deviation no longer holds", w.run)
		}
		return nil
	})
}

// eachRecordedAnswer applies f to every executor's answer for one task, and says
// nothing when no run of it was recorded: the per-run scenarios are what demand
// the records exist.
func eachRecordedAnswer(w *r3World, id string, f func(exec, text string) error) error {
	base := filepath.Join(w.plugin, "steps", "agent-tasks")
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		p := filepath.Join(base, e.Name(), id, "answer.md")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := f(e.Name(), string(b)); err != nil {
			return err
		}
	}
	return nil
}

// isEngineReport: the shape only this engine writes, not the first JSON with an
// "engine" key in it (implementation review, finding 1).
func isEngineReport(doc map[string]any) bool {
	eng, _ := doc["engine"].(map[string]any)
	if eng == nil || eng["name"] != "mcd" || eng["report_schema"] != "mcd-report/1" {
		return false
	}
	if fmt.Sprint(eng["version"]) == "" {
		return false
	}
	_, ok := doc["properties"].([]any)
	return ok
}

// reportIsOf says whether the report was written for this model text, by the
// hash the engine recorded for its input.
func reportIsOf(doc map[string]any, model string) bool {
	sum := sha256.Sum256([]byte(model))
	want := hex.EncodeToString(sum[:])
	inputs, _ := doc["inputs"].([]any)
	for _, i := range inputs {
		m, _ := i.(map[string]any)
		if m != nil && fmt.Sprint(m["sha256"]) == want {
			return true
		}
	}
	return false
}

// reportIsOfRunFile: the report names an input that is a file of this run — the
// route parse → edit the IR → check --ir produces a report of the IR, not of the
// Promela text, and that is still this run's work.
func reportIsOfRunFile(doc map[string]any, run string) bool {
	inputs, _ := doc["inputs"].([]any)
	for _, i := range inputs {
		m, _ := i.(map[string]any)
		if m == nil {
			continue
		}
		name := filepath.Base(fmt.Sprint(m["path"]))
		var found bool
		_ = filepath.Walk(run, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || found {
				return nil
			}
			if filepath.Base(p) != name {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) == fmt.Sprint(m["sha256"]) {
				found = true
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}
