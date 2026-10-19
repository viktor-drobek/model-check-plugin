package modelcheck_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestReleaseFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG6Steps,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features/g6-release.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("release workflow contract failed")
	}
}

func TestReleaseWorkflowPolicy(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	buildStart := strings.Index(workflow, "\n  build:\n")
	publishStart := strings.Index(workflow, "\n  publish:\n")
	if buildStart < 0 || publishStart <= buildStart {
		t.Fatal("expected separate build and publish jobs")
	}
	build := workflow[buildStart:publishStart]
	publish := workflow[publishStart:]
	for name, job := range map[string]string{"build": build, "publish": publish} {
		permission := "read"
		if name == "publish" {
			permission = "write"
		}
		header, _, ok := strings.Cut(job, "    steps:\n")
		if !ok || !strings.Contains(header, "    permissions:\n      contents: "+permission+"\n") {
			t.Errorf("%s job must declare contents: %s before steps", name, permission)
		}
	}
	if !strings.Contains(workflow[:buildStart], "\npermissions:\n  contents: read\n") || strings.Count(workflow, "contents: write") != 1 {
		t.Error("write permission must be limited to the publish job")
	}
	checkoutStart := strings.Index(build, "uses: actions/checkout@")
	if checkoutStart < 0 {
		t.Fatal("missing build checkout")
	}
	checkout, _, _ := strings.Cut(build[checkoutStart:], "\n      - name:")
	if !strings.Contains(checkout, "persist-credentials: false") {
		t.Error("checkout must not persist credentials")
	}
	if strings.Contains(publish, "actions/checkout@") || strings.Contains(workflow, "--clobber") {
		t.Error("publish must neither checkout executable source nor overwrite assets")
	}
}
