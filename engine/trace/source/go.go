// Package source adapts source text into fail-closed trace-ledger facts.
//
// Adapters receive bytes from their caller and never read a source tree. They
// report syntactic observations only; unresolved references are represented as
// trace-ledger gaps rather than guessed relationships.
package source

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"modelcheck/trace"
)

const (
	goAdapterName    = "mcd-go-ast-adapter"
	goAdapterVersion = "1"
)

// Input is a caller-provided source artifact. Path is a logical, stable name
// for review anchors; AnalyzeGo never opens it. SourceRevision may identify a
// VCS revision, while Content is always pinned with its SHA-256 digest.
type Input struct {
	Path           string
	Content        []byte
	SourceRevision string
}

// Adapter converts one caller-provided source artifact into a trace ledger.
// Each implementation defines its own conservative resolution boundary.
type Adapter interface {
	Language() string
	Analyze(Input) (*trace.Document, error)
}

// GoAdapter produces syntax-only observations from Go source text.
type GoAdapter struct{}

// Language identifies the syntax accepted by GoAdapter.
func (GoAdapter) Language() string { return "go" }

// AnalyzeGo produces a syntactic trace ledger for one Go source artifact.
// It recognizes declared functions and direct calls to exactly one unshadowed,
// package-level function in that same artifact. Every other call expression is
// deliberately recorded as a gap: resolving selectors, imports, aliases, or
// flow-dependent bindings would exceed this adapter's syntactic contract.
func AnalyzeGo(input Input) (*trace.Document, error) {
	return GoAdapter{}.Analyze(input)
}

// Analyze produces a syntactic trace ledger for one Go source artifact.
// It recognizes declared functions and direct calls to exactly one unshadowed,
// package-level function in that same artifact. Every other call expression is
// deliberately recorded as a gap: resolving selectors, imports, aliases, or
// flow-dependent bindings would exceed this adapter's syntactic contract.
func (GoAdapter) Analyze(input Input) (*trace.Document, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, fmt.Errorf("Go source path is blank")
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, input.Content, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go source %q: %w", path, err)
	}
	hasDotImport := false
	for _, importSpec := range file.Imports {
		if importSpec.Name != nil && importSpec.Name.Name == "." {
			hasDotImport = true
			break
		}
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(input.Content))
	prefix := digest[:12]
	newProvenance := func(position token.Pos) trace.Provenance {
		line := fileSet.PositionFor(position, false).Line
		return trace.Provenance{
			SourceRevision: strings.TrimSpace(input.SourceRevision),
			ContentSHA256:  digest,
			Anchor:         fmt.Sprintf("%s:%d", path, line),
			Producer:       trace.Producer{Name: goAdapterName, Version: goAdapterVersion},
		}
	}

	document := &trace.Document{
		Schema:   trace.Schema,
		Nodes:    []trace.Node{},
		Edges:    []trace.Edge{},
		Evidence: []trace.Evidence{},
		Gaps:     []trace.Gap{},
	}
	type symbol struct {
		id   string
		decl *ast.FuncDecl
	}
	functions := make([]symbol, 0)
	byName := make(map[string][]symbol)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		line := fileSet.PositionFor(function.Pos(), false).Line
		id := fmt.Sprintf("go.symbol.%s.%s.%d", prefix, function.Name.Name, line)
		evidenceID := "evidence." + id
		document.Nodes = append(document.Nodes, trace.Node{
			ID:         id,
			Kind:       trace.SourceSymbol,
			Label:      function.Name.Name,
			Provenance: newProvenance(function.Pos()),
			Evidence:   []string{evidenceID},
		})
		document.Evidence = append(document.Evidence, trace.Evidence{
			ID:         evidenceID,
			Subject:    "node:" + id,
			Level:      trace.Syntactic,
			Provenance: newProvenance(function.Pos()),
		})
		entry := symbol{id: id, decl: function}
		functions = append(functions, entry)
		if function.Recv == nil {
			byName[function.Name.Name] = append(byName[function.Name.Name], entry)
		}
	}

	sequence := 0
	appendGap := func(call *ast.CallExpr) {
		sequence++
		position := call.Pos()
		gapID := fmt.Sprintf("go.gap.%s.%d", prefix, sequence)
		document.Gaps = append(document.Gaps, trace.Gap{
			ID:         gapID,
			Kind:       "source.unresolved_call",
			Reference:  newProvenance(position).Anchor,
			Reason:     fmt.Sprintf("call target %q is not unambiguously resolved by the Go AST adapter", callName(call.Fun)),
			Provenance: newProvenance(position),
		})
	}
	for _, caller := range functions {
		if caller.decl.Body == nil {
			continue
		}
		shadowed := localNames(caller.decl)
		ast.Inspect(caller.decl.Body, func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested {
				ast.Inspect(literal.Body, func(nestedNode ast.Node) bool {
					if nestedCall, ok := nestedNode.(*ast.CallExpr); ok {
						appendGap(nestedCall)
					}
					return true
				})
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			position := call.Pos()
			if target, ok := call.Fun.(*ast.Ident); ok && !hasDotImport && !shadowed[target.Name] {
				if candidates := byName[target.Name]; len(candidates) == 1 {
					sequence++
					edgeID := fmt.Sprintf("go.call.%s.%d", prefix, sequence)
					evidenceID := "evidence." + edgeID
					document.Edges = append(document.Edges, trace.Edge{
						ID:         edgeID,
						Kind:       trace.Calls,
						Source:     caller.id,
						Target:     candidates[0].id,
						Provenance: newProvenance(position),
						Evidence:   []string{evidenceID},
					})
					document.Evidence = append(document.Evidence, trace.Evidence{
						ID:         evidenceID,
						Subject:    "edge:" + edgeID,
						Level:      trace.Syntactic,
						Provenance: newProvenance(position),
					})
					return true
				}
			}
			appendGap(call)
			return true
		})
	}
	if err := trace.Validate(document); err != nil {
		return nil, fmt.Errorf("validate Go source trace ledger: %w", err)
	}
	return document, nil
}

func localNames(function *ast.FuncDecl) map[string]bool {
	names := make(map[string]bool)
	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				names[name.Name] = true
			}
		}
	}
	if function.Recv != nil {
		addFields(function.Recv)
	}
	addFields(function.Type.TypeParams)
	addFields(function.Type.Params)
	addFields(function.Type.Results)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch declaration := node.(type) {
		case *ast.AssignStmt:
			if declaration.Tok == token.DEFINE {
				for _, left := range declaration.Lhs {
					if name, ok := left.(*ast.Ident); ok {
						names[name.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if declaration.Tok == token.DEFINE {
				for _, expression := range []ast.Expr{declaration.Key, declaration.Value} {
					if name, ok := expression.(*ast.Ident); ok {
						names[name.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, name := range declaration.Names {
				names[name.Name] = true
			}
		case *ast.TypeSpec:
			names[declaration.Name.Name] = true
		}
		return true
	})
	return names
}

func callName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return callName(value.X) + "." + value.Sel.Name
	case *ast.IndexExpr:
		return callName(value.X)
	case *ast.IndexListExpr:
		return callName(value.X)
	default:
		return "<expression>"
	}
}
