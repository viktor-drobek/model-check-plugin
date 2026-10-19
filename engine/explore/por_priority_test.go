package explore

import (
	"context"
	"strings"
	"testing"
)

func TestPORRefusesGroupedPriorities(t *testing.T) {
	m := porVisible()
	m.Processes[0].Edges[0].Priority = 1
	m.Processes[0].Edges[0].PriorityGroup = 1

	res, err := Run(context.Background(), m, Options{POR: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reduction == nil || res.Reduction.Applied {
		t.Fatalf("reduction = %+v, want grouped-priority refusal", res.Reduction)
	}
	if !strings.Contains(res.Reduction.Reason, "grouped transition priorities") {
		t.Fatalf("reason %q", res.Reduction.Reason)
	}
}
