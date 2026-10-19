package modelcheck_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/protocol"
)

type g9ProtobufWorld struct {
	run    func() ([]byte, error)
	output []byte
	err    error
	doc    *trace.Document
}

func init() { stepRegistrars = append(stepRegistrars, registerG9ProtobufSteps) }

func TestG9ProtobufFeatures(t *testing.T) {
	suite := godog.TestSuite{ScenarioInitializer: func(sc *godog.ScenarioContext) {
		for _, register := range stepRegistrars {
			register(sc)
		}
	}, Options: &godog.Options{Format: "pretty", Paths: []string{"../features/g9-protobuf.feature"}, Strict: true, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 protobuf features")
	}
}

func registerG9ProtobufSteps(sc *godog.ScenarioContext) {
	var w g9ProtobufWorld
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w = g9ProtobufWorld{}
		return ctx, nil
	})
	sc.Step(`^the protobuf contract case "([^"]*)"$`, func(name string) error { var err error; w.run, err = g9ProtobufCase(name); return err })
	sc.Step(`^the protobuf contract adapter analyses the artifact$`, func() error {
		if w.run == nil {
			return fmt.Errorf("protobuf case was not configured")
		}
		w.output, w.err = w.run()
		return nil
	})
	sc.Step(`^the protobuf contract output is a deterministic ledger$`, func() error {
		if w.err != nil {
			return w.err
		}
		again, err := w.run()
		if err != nil {
			return err
		}
		if !bytes.Equal(w.output, again) {
			return fmt.Errorf("protobuf output is not deterministic")
		}
		w.doc, err = trace.UnmarshalJSON(w.output)
		return err
	})
	sc.Step(`^the protobuf contract output has (\d+) protocol messages$`, func(n int) error { return protobufNodeCount(w.doc, trace.ProtocolMessage, n) })
	sc.Step(`^the protobuf contract output has (\d+) binary fields$`, func(n int) error { return protobufNodeCount(w.doc, trace.BinaryField, n) })
	sc.Step(`^the protobuf contract output has (\d+) endpoint$`, func(n int) error { return protobufNodeCount(w.doc, trace.Endpoint, n) })
	sc.Step(`^the protobuf contract output has (\d+) gaps of kind "([^"]*)"$`, func(n int, kind string) error {
		count := 0
		for _, gap := range w.doc.Gaps {
			if gap.Kind == kind {
				count++
			}
		}
		if count != n {
			return fmt.Errorf("gaps=%d want=%d", count, n)
		}
		return nil
	})
	sc.Step(`^the protobuf contract output has a gap of kind "([^"]*)"$`, func(kind string) error {
		for _, gap := range w.doc.Gaps {
			if gap.Kind == kind {
				return nil
			}
		}
		return fmt.Errorf("gap %s missing", kind)
	})
	sc.Step(`^the protobuf contract output is absent with rule "([^"]*)"$`, func(code string) error {
		var rule *protocol.RuleError
		if w.output != nil || !errors.As(w.err, &rule) || rule.Code != code {
			return fmt.Errorf("output=%d err=%v want=%s", len(w.output), w.err, code)
		}
		return nil
	})
}

func protobufNodeCount(doc *trace.Document, kind trace.NodeKind, want int) error {
	count := 0
	for _, node := range doc.Nodes {
		if node.Kind == kind {
			count++
		}
	}
	if count != want {
		return fmt.Errorf("nodes=%d want=%d", count, want)
	}
	return nil
}

func g9ProtobufCase(name string) (func() ([]byte, error), error) {
	cases := map[string]struct {
		data string
		buf  bool
	}{
		"valid":                            {data: `syntax = "proto3"; message Request { string id = 1; repeated int32 values = 2; } message Reply { Request echo = 1; } service Echo { rpc Call (Request) returns (Reply); }`},
		"import":                           {data: `syntax = "proto3"; import "external.proto";`},
		"json-name-collision":              {data: `syntax = "proto3"; message A { string foo_bar = 1; string fooBar = 2; }`},
		"json-name-trailing-underscore":    {data: `syntax = "proto3"; message A { string foo_ = 1; string foo = 2; }`},
		"json-name-consecutive-underscore": {data: `syntax = "proto3"; message A { string foo__bar = 1; string fooBar = 2; }`},
		"scalar-message":                   {data: `syntax = "proto3"; message string { string value = 1; }`},
		"scalar-service":                   {data: `syntax = "proto3"; message A {} service string { rpc Call(A) returns(A); }`},
		"spaced-package":                   {data: `syntax = "proto3"; package demo . v1; message A { string value = 1; }`},
		"buf-valid":                        {data: `{"version":"v2","modules":[{"path":"api/v1"},{"path":"internal"}],"deps":[]}`, buf: true},
		"buf-dependency":                   {data: `{"version":"v2","modules":[{"path":"api"}],"deps":["buf.build/acme/api"]}`, buf: true},
	}
	c, ok := cases[name]
	if !ok {
		return nil, fmt.Errorf("unknown protobuf case %q", name)
	}
	data := []byte(c.data)
	sum := sha256.Sum256(data)
	in := protocol.ContractInput{Artifact: protocol.Input{Path: "fixtures/g9-contract", Content: data}, ContentSHA256: hex.EncodeToString(sum[:])}
	if c.buf {
		return func() ([]byte, error) { return protocol.AnalyzeBuf(in) }, nil
	}
	return func() ([]byte, error) { return protocol.AnalyzeProtobuf(in) }, nil
}
