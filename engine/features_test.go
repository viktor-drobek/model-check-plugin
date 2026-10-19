package modelcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures runs every Gherkin feature under ../features.
// Step definitions are registered by the steps_*_test.go files of this package.
func TestFeatures(t *testing.T) {
	if _, err := os.Stat("../features"); err != nil {
		t.Skip("no features directory")
	}
	paths, err := filepath.Glob("../features/*.feature")
	if err != nil {
		t.Fatal(err)
	}
	globalPaths := rootFeaturePaths(paths, os.Getenv("MCD_BDD_G9_ONLY") != "")
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, reg := range stepRegistrars {
				reg(sc)
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    globalPaths,
			Tags:     "~@pending && ~@runtime_internal",
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func rootFeaturePaths(paths []string, g9Only bool) []string {
	globalPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		name := filepath.Base(path)
		if g9Only && !strings.HasPrefix(name, "g9-") {
			continue
		}
		switch name {
		case "g9-impact.feature", "g9-impact-git-security.feature", "g9-impact-git-lifecycle.feature",
			"g9-runtime-spans.feature", "g9-contract-safety.feature":
			continue // These features run in their owning trace subpackages.
		}
		globalPaths = append(globalPaths, path)
	}
	return globalPaths
}

func TestRootFeatureOwnership(t *testing.T) {
	paths := []string{
		"../features/g0-engine.feature",
		"../features/g9-asyncapi.feature",
		"../features/g9-contract-safety.feature",
		"../features/g9-dossier.feature",
		"../features/g9-impact.feature",
		"../features/g9-impact-git-security.feature",
		"../features/g9-impact-git-lifecycle.feature",
		"../features/g9-openapi.feature",
		"../features/g9-pact.feature",
		"../features/g9-protobuf.feature",
		"../features/g9-runtime-spans.feature",
		"../features/g9-trace-corpus.feature",
	}
	for _, only := range []bool{false, true} {
		got := rootFeaturePaths(paths, only)
		want := []string{
			"../features/g9-asyncapi.feature",
			"../features/g9-dossier.feature",
			"../features/g9-openapi.feature",
			"../features/g9-pact.feature",
			"../features/g9-protobuf.feature",
			"../features/g9-trace-corpus.feature",
		}
		if !only {
			want = append([]string{paths[0]}, want...)
		}
		if !slices.Equal(got, want) {
			t.Errorf("G9 only=%t: got %v, want %v", only, got, want)
		}
	}
}

// stepRegistrars collects step definitions from steps_*_test.go files.
// Each such file appends its registrar in an init() function.
var stepRegistrars []func(*godog.ScenarioContext)
