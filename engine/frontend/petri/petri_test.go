package petri

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"modelcheck/ir"
)

func mustParse(t *testing.T, src string) *Net {
	t.Helper()
	n, err := Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSchemaFileIsDraft2020(t *testing.T) {
	b, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %v", s["$schema"])
	}
	// The hand-written validator must know every property the schema lists.
	defs := s["$defs"].(map[string]any)
	for name, want := range map[string][]string{
		"place":      {"name", "initial", "capacity", "line"},
		"arc":        {"place", "weight", "inhibitor"},
		"transition": {"name", "inputs", "outputs", "line"},
	} {
		props := defs[name].(map[string]any)["properties"].(map[string]any)
		if len(props) != len(want) {
			t.Fatalf("%s: schema lists %d properties, validator knows %d", name, len(props), len(want))
		}
		for _, p := range want {
			if _, ok := props[p]; !ok {
				t.Fatalf("%s: schema lacks %q", name, p)
			}
		}
	}
}

func TestParsePetrinet1Encoding(t *testing.T) {
	b, err := os.ReadFile("../../testdata/petri/petrinet1.json")
	if err != nil {
		t.Fatal(err)
	}
	n, err := Parse(b, "x")
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "petrinet1" || len(n.Places) != 6 || len(n.Transitions) != 6 {
		t.Fatalf("net %+v", n)
	}
	m := n.ToIR("petrinet1.json")
	if err := ir.Validate(m); err != nil {
		t.Fatal(err)
	}
	l, _ := ir.NewLayout(m)
	if l.Size != 8 { // 6 bytes + pc + excl, like the spike's 6 + headers
		t.Fatalf("state bytes %d", l.Size)
	}
	if len(m.Processes) != 1 || len(m.Processes[0].Edges) != 6 || len(m.Processes[0].Locations) != 1 {
		t.Fatal("expected one looping process with 6 edges")
	}
	e := m.Processes[0].Edges[1] // t2: inp2(p2,p4) -> out1(p3)
	if e.Guard.String() != "(p2 > 0) && (p4 > 0)" {
		t.Fatalf("t2 guard %s", e.Guard)
	}
	if len(e.Effect) != 3 || e.Effect[0].Var != "p2" || e.Effect[1].Var != "p4" || e.Effect[2].Var != "p3" {
		t.Fatalf("t2 effect order %+v (decrements must precede increments)", e.Effect)
	}
	if e.Atomic || e.Text != "t2" || e.Origin == nil || e.Origin.Name != "t2" || e.Origin.Line != 17 {
		t.Fatalf("t2 edge %+v origin %+v", e, e.Origin)
	}
	if len(m.Properties) != 2 || m.Properties[0].Kind != ir.KindDeadlock || m.Properties[1].ID != "safe" {
		t.Fatalf("properties %+v", m.Properties)
	}
	if m.Globals[0].Max != nil {
		t.Fatal("default capacity 255 must not narrow the byte domain")
	}
}

func TestWeightsAndCapacity(t *testing.T) {
	n := mustParse(t, `{"places":[{"name":"a","initial":3,"capacity":3},{"name":"b"}],
	  "transitions":[{"name":"t","inputs":[{"place":"a","weight":2}],"outputs":[{"place":"b","weight":2}]}]}`)
	m := n.ToIR("f")
	e := m.Processes[0].Edges[0]
	if e.Guard.String() != "a >= 2" || e.Effect[0].Value.String() != "a - 2" || e.Effect[1].Value.String() != "b + 2" {
		t.Fatalf("guard %s effect %s / %s", e.Guard, e.Effect[0].Value, e.Effect[1].Value)
	}
	if m.Globals[0].Max == nil || *m.Globals[0].Max != 3 {
		t.Fatal("capacity 3 must become Max = 3")
	}
	if len(e.Effect) != 2 {
		t.Fatalf("%d assignments", len(e.Effect))
	}
}

func TestRejections(t *testing.T) {
	cases := []struct {
		src        string
		kind, path string
		mention    string
	}{
		{`{"places":[{"name":"a"}],"transitions":[{"name":"t","inputs":[{"place":"a","inhibitor":true}]}]}`,
			"unsupported-input", "transitions", "Holzmann"},
		{`{"places":[{"name":"a"}],"transitions":[{"name":"t","outputs":[{"place":"a","inhibitor":true}]}]}`,
			"schema", "transitions[0].outputs[0].inhibitor", "output arc"},
		{`{"places":[{"name":"a"}],"transitions":[{"name":"t","inputs":[{"place":"a","weight":0}]}]}`,
			"schema", "transitions[0].inputs[0].weight", "outside"},
		{`{"places":[{"name":"a"}],"transitions":[{"name":"t","inputs":[{"place":"zz"}]}]}`,
			"schema", "transitions[0].inputs[0].place", "unknown place"},
		{`{"places":[{"name":"a","initial":2,"capacity":1}],"transitions":[{"name":"t"}]}`,
			"schema", "places[0].initial", "exceeds capacity"},
		{`{"places":[{"name":"a"},{"name":"a"}],"transitions":[{"name":"t"}]}`,
			"schema", "places[1].name", "duplicate"},
		{`{"places":[{"name":"a"}],"transitions":[{"name":"a"}]}`,
			"schema", "transitions[0].name", "already used"},
		{`{"places":[{"name":"1a"}],"transitions":[{"name":"t"}]}`,
			"schema", "places[0].name", "identifier"},
		{`{"places":[{"name":"a","colour":"red"}],"transitions":[{"name":"t"}]}`,
			"schema", "places[0]", "colour"},
		{`{"places":[],"transitions":[{"name":"t"}]}`, "schema", "$.places", "at least 1"},
		{`{"transitions":[{"name":"t"}]}`, "schema", "$", "places"},
		{`{"places":[{"name":"a","initial":"1"}],"transitions":[{"name":"t"}]}`, "schema", "places[0].initial", "integer"},
		{`{"places":[{"name":"a","initial":1.5}],"transitions":[{"name":"t"}]}`, "schema", "places[0].initial", "integer"},
		{`[]`, "schema", "$", "object"},
		{`{`, "schema", "", "JSON"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src), "t")
		pe, ok := err.(*Error)
		if !ok {
			t.Errorf("%s: expected *Error, got %v", c.src, err)
			continue
		}
		if pe.Kind != c.kind || pe.Path != c.path || !strings.Contains(pe.Message, c.mention) {
			t.Errorf("%s:\n got  %s / %s / %s\n want %s / %s / ...%s...", c.src, pe.Kind, pe.Path, pe.Message, c.kind, c.path, c.mention)
		}
	}
}

func TestDefaultsAndName(t *testing.T) {
	n := mustParse(t, `{"places":[{"name":"a"}],"transitions":[{"name":"t"}]}`)
	if n.Name != "test" || n.Places[0].Capacity != 255 || n.Places[0].Initial != 0 {
		t.Fatalf("%+v", n)
	}
	m := n.ToIR("f")
	if m.Processes[0].Edges[0].Guard.String() != "1" { // no inputs: always enabled
		t.Fatalf("guard %s", m.Processes[0].Edges[0].Guard)
	}
}
