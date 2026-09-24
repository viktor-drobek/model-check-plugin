package modelcheck_test

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures runs every Gherkin feature under ../features.
// Step definitions are registered by the steps_*_test.go files of this package.
func TestFeatures(t *testing.T) {
	if _, err := os.Stat("../features"); err != nil {
		t.Skip("no features directory")
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, reg := range stepRegistrars {
				reg(sc)
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features"},
			Tags:     "~@pending",
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// stepRegistrars collects step definitions from steps_*_test.go files.
// Each such file appends its registrar in an init() function.
var stepRegistrars []func(*godog.ScenarioContext)
